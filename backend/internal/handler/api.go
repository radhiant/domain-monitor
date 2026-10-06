package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"domain-monitor/backend/internal/config"
	"domain-monitor/backend/internal/state"
	"domain-monitor/backend/internal/store"
	"domain-monitor/backend/internal/websocket"
)

// API holds the dependencies every endpoint shares.
type API struct {
	cfg       *config.Config
	snap      *state.Snapshot
	store     *store.Store
	hub       *websocket.Hub
	rechecker Rechecker
	started   time.Time
}

// NewAPI constructs the handler set.
func NewAPI(cfg *config.Config, snap *state.Snapshot, st *store.Store, hub *websocket.Hub, rechecker Rechecker) *API {
	return &API{
		cfg:       cfg,
		snap:      snap,
		store:     st,
		hub:       hub,
		rechecker: rechecker,
		started:   time.Now(),
	}
}

// Health reports that the agent is alive.
func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"uptime":    int64(time.Since(a.started).Seconds()),
		"targets":   len(a.snap.TargetList()),
		"domains":   len(a.snap.Apexes()),
		"dashboard": a.hub.Clients(),
		"time":      time.Now().UTC(),
	})
}

// Config publishes the thresholds and intervals so the dashboard renders the
// same warning bands the backend classifies with, instead of a second copy
// that can drift.
func (a *API) Config(w http.ResponseWriter, r *http.Request) {
	th := a.snap.Thresholds()
	writeJSON(w, http.StatusOK, map[string]any{
		"latency_warn_ms":  th.LatencyWarnMS,
		"latency_crit_ms":  th.LatencyCritMS,
		"cert_warn_days":   th.CertWarnDays,
		"cert_crit_days":   th.CertCritDays,
		"domain_warn_days": th.DomainWarnDays,
		"domain_crit_days": th.DomainCritDays,
		"accept_status":    th.AcceptStatus,
		"http_interval_s":  int(a.cfg.HTTPInterval.Seconds()),
		"targets_file":     a.cfg.TargetsFile,
	})
}

// Overview serves the KPI band.
func (a *API) Overview(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.snap.Overview())
}

// Domains serves the endpoints grouped by apex domain.
func (a *API) Domains(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.snap.Groups())
}

// Targets serves every endpoint as a flat list.
func (a *API) Targets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.snap.States())
}

// Target serves one endpoint together with its outage history.
func (a *API) Target(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	st, ok := a.snap.State(id)
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Unknown target: "+id)
		return
	}

	incidents, err := a.store.IncidentsFor(id, 20)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"target":    st,
		"incidents": incidents,
	})
}

// History serves the time series behind the detail-view chart.
func (a *API) History(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := a.snap.State(id); !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Unknown target: "+id)
		return
	}

	rangeKey := r.URL.Query().Get("range")
	since, bucket, label := resolveRange(rangeKey)

	points, err := a.store.History(id, since, bucket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"target_id": id,
		"range":     label,
		"bucket_s":  int(bucket.Seconds()),
		"points":    points,
	})
}

// resolveRange maps a range key onto a window and bucket width.
//
// Bucket widths are chosen so every range renders roughly 150-300 points: wide
// enough to be readable across a room, cheap enough to send on every switch.
func resolveRange(key string) (since time.Time, bucket time.Duration, label string) {
	now := time.Now()
	switch key {
	case "7d":
		return now.AddDate(0, 0, -7), time.Hour, "7d"
	case "30d":
		return now.AddDate(0, 0, -30), 6 * time.Hour, "30d"
	default:
		return now.Add(-24 * time.Hour), 5 * time.Minute, "24h"
	}
}

// Incidents serves the outage list, open ones first.
func (a *API) Incidents(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	if r.URL.Query().Get("active") == "true" {
		incidents, err := a.store.ActiveIncidents()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, incidents)
		return
	}

	incidents, err := a.store.RecentIncidents(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "QUERY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, incidents)
}

// Expiry serves the combined SSL and domain watchlist, soonest first.
func (a *API) Expiry(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.snap.Expiry())
}

// Events serves the recent status transitions behind the ticker.
func (a *API) Events(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.snap.Events())
}

// Recheck forces an immediate sweep of every endpoint.
func (a *API) Recheck(w http.ResponseWriter, r *http.Request) {
	if a.rechecker == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Scheduler is not running")
		return
	}
	a.rechecker.RecheckAll()
	writeJSON(w, http.StatusAccepted, map[string]any{"status": "scheduled"})
}
