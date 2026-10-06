// Package handler exposes the REST and WebSocket surface the dashboard reads.
package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"domain-monitor/backend/internal/config"
	"domain-monitor/backend/internal/model"
	"domain-monitor/backend/internal/state"
	"domain-monitor/backend/internal/store"
	"domain-monitor/backend/internal/websocket"
)

// Rechecker lets the API ask the scheduler for an immediate sweep without the
// handler package depending on the scheduler.
type Rechecker interface {
	RecheckAll()
}

// NewRouter builds the HTTP and WebSocket router.
func NewRouter(
	cfg *config.Config,
	snap *state.Snapshot,
	st *store.Store,
	hub *websocket.Hub,
	rechecker Rechecker,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CorsOrigins,
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	api := NewAPI(cfg, snap, st, hub, rechecker)
	promHandler := NewPrometheusHandler(snap)

	// Health and Prometheus stay public so a load balancer or Prometheus scraper can reach it
	// without holding the API token.
	r.Get("/api/v1/health", api.Health)
	r.Get("/metrics", promHandler.ServeHTTP)

	r.Group(func(g chi.Router) {
		g.Use(authMiddleware(cfg.APIToken))

		g.Get("/api/v1/config", api.Config)
		g.Get("/api/v1/overview", api.Overview)
		g.Get("/api/v1/domains", api.Domains)
		g.Get("/api/v1/targets", api.Targets)
		g.Get("/api/v1/targets/{id}", api.Target)
		g.Get("/api/v1/targets/{id}/history", api.History)
		g.Get("/api/v1/incidents", api.Incidents)
		g.Get("/api/v1/expiry", api.Expiry)
		g.Get("/api/v1/events", api.Events)
		g.Post("/api/v1/recheck", api.Recheck)

		g.Get("/ws/v1", func(w http.ResponseWriter, r *http.Request) {
			websocket.ServeWs(hub, w, r)
		})
	})

	return r
}

// authMiddleware enforces a Bearer token when one is configured. The token may
// also arrive as a query parameter, because a browser cannot set headers on a
// WebSocket handshake.
func authMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			provided := ""
			if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
				provided = strings.TrimPrefix(authHeader, "Bearer ")
			} else if q := r.URL.Query().Get("token"); q != "" {
				provided = q
			}

			if provided != token {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or missing API token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	// Wall displays poll through Nginx; caching an overview would freeze the
	// numbers on screen while everything behind them changed.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, model.ErrorResponse{
		Error: model.ErrorDetail{Code: code, Message: message},
	})
}
