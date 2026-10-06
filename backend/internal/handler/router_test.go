package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"domain-monitor/backend/internal/config"
	"domain-monitor/backend/internal/model"
	"domain-monitor/backend/internal/state"
	"domain-monitor/backend/internal/store"
	"domain-monitor/backend/internal/websocket"
)

type fakeRechecker struct{ called bool }

func (f *fakeRechecker) RecheckAll() { f.called = true }

func newTestRouter(t *testing.T, token string) (http.Handler, *state.Snapshot, *fakeRechecker) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "router.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	snap := state.New(state.Thresholds{LatencyWarnMS: 800, LatencyCritMS: 2000, CertWarnDays: 30, CertCritDays: 14})
	snap.SetTargets([]model.Target{
		{ID: "a.test", Apex: "test", Host: "a.test", URL: "https://a.test", Label: "a"},
	}, []string{"test"})

	hub := websocket.NewHub(func() any { return snap.Full() })
	rechecker := &fakeRechecker{}

	cfg := &config.Config{APIToken: token, CorsOrigins: []string{"*"}}
	return NewRouter(cfg, snap, st, hub, rechecker), snap, rechecker
}

func get(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestHealthIsPublic(t *testing.T) {
	// A token must not lock out the load balancer probe.
	router, _, _ := newTestRouter(t, "secret")

	if rec := get(t, router, "/api/v1/health"); rec.Code != http.StatusOK {
		t.Fatalf("health = %d, want 200 without a token", rec.Code)
	}
}

func TestTokenIsEnforced(t *testing.T) {
	router, _, _ := newTestRouter(t, "secret")

	if rec := get(t, router, "/api/v1/overview"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated overview = %d, want 401", rec.Code)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/overview", nil)
	req.Header.Set("Authorization", "Bearer secret")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer overview = %d, want 200", rec.Code)
	}

	// A browser cannot set headers on a WebSocket handshake, so the query
	// parameter form has to work too.
	if rec := get(t, router, "/api/v1/overview?token=secret"); rec.Code != http.StatusOK {
		t.Fatalf("query-token overview = %d, want 200", rec.Code)
	}
}

func TestOpenAgentNeedsNoToken(t *testing.T) {
	router, _, _ := newTestRouter(t, "")

	if rec := get(t, router, "/api/v1/overview"); rec.Code != http.StatusOK {
		t.Fatalf("overview = %d, want 200 when no token is configured", rec.Code)
	}
}

func TestOverviewShape(t *testing.T) {
	router, _, _ := newTestRouter(t, "")

	rec := get(t, router, "/api/v1/overview")
	var ov model.Overview
	if err := json.Unmarshal(rec.Body.Bytes(), &ov); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if ov.TotalEndpoints != 1 || ov.TotalDomains != 1 {
		t.Fatalf("overview = %+v, want 1 endpoint in 1 domain", ov)
	}
	if ov.Unknown != 1 {
		t.Errorf("unprobed endpoint counted as %d unknown, want 1", ov.Unknown)
	}
}

func TestDomainsAreGrouped(t *testing.T) {
	router, _, _ := newTestRouter(t, "")

	rec := get(t, router, "/api/v1/domains")
	var groups []model.DomainGroup
	if err := json.Unmarshal(rec.Body.Bytes(), &groups); err != nil {
		t.Fatalf("decode domains: %v", err)
	}
	if len(groups) != 1 || groups[0].Apex != "test" || len(groups[0].Endpoints) != 1 {
		t.Fatalf("groups = %+v", groups)
	}
}

func TestUnknownTargetIs404(t *testing.T) {
	router, _, _ := newTestRouter(t, "")

	if rec := get(t, router, "/api/v1/targets/nope.test"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown target = %d, want 404", rec.Code)
	}
	if rec := get(t, router, "/api/v1/targets/nope.test/history"); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown history = %d, want 404", rec.Code)
	}
}

func TestHistoryDefaultsTo24h(t *testing.T) {
	router, _, _ := newTestRouter(t, "")

	rec := get(t, router, "/api/v1/targets/a.test/history?range=nonsense")
	var body struct {
		Range   string `json:"range"`
		BucketS int    `json:"bucket_s"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if body.Range != "24h" || body.BucketS != 300 {
		t.Fatalf("history = %+v, want a 24h range in 5m buckets", body)
	}
}

func TestRecheckReachesTheScheduler(t *testing.T) {
	router, _, rechecker := newTestRouter(t, "")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recheck", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("recheck = %d, want 202", rec.Code)
	}
	if !rechecker.called {
		t.Error("recheck did not reach the scheduler")
	}
}

func TestResponsesAreNotCached(t *testing.T) {
	// A cached overview would freeze the numbers on a wall display.
	router, _, _ := newTestRouter(t, "")

	rec := get(t, router, "/api/v1/overview")
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestPrometheusIsPublic(t *testing.T) {
	router, _, _ := newTestRouter(t, "secret-token")

	rec := get(t, router, "/metrics")
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d, want 200 without token", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "domain_overview_domains_total") {
		t.Fatalf("expected /metrics to contain domain_overview_domains_total, got %s", rec.Body.String())
	}
}

