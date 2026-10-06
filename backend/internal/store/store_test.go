package store

import (
	"path/filepath"
	"testing"
	"time"

	"domain-monitor/backend/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	if err := s.SyncTargets([]model.Target{
		{ID: "a.test", Apex: "test", Host: "a.test", URL: "https://a.test", Label: "a"},
		{ID: "b.test", Apex: "test", Host: "b.test", URL: "https://b.test", Label: "b"},
	}); err != nil {
		t.Fatalf("sync targets: %v", err)
	}

	return s
}

func insert(t *testing.T, s *Store, id string, at time.Time, up bool, latency float64) {
	t.Helper()

	status := model.StatusUp
	if !up {
		status = model.StatusDown
	}
	err := s.InsertCheck(id, status, &model.HTTPResult{
		CheckedAt: at,
		Up:        up,
		LatencyMS: latency,
	})
	if err != nil {
		t.Fatalf("insert check: %v", err)
	}
}

func TestUptimeRollup(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	// 8 successes and 2 failures inside the last hour: 80% over 24h.
	for i := range 10 {
		insert(t, s, "a.test", now.Add(-time.Duration(i)*time.Minute), i >= 2, float64(100+i*10))
	}
	// A sample from 10 days ago must land in the 30-day window only.
	insert(t, s, "a.test", now.AddDate(0, 0, -10), true, 100)

	ups, err := s.Uptimes()
	if err != nil {
		t.Fatalf("uptimes: %v", err)
	}

	got := ups["a.test"]
	if got.Day != 80 {
		t.Errorf("24h uptime = %.2f, want 80", got.Day)
	}
	// 9 of 11 succeeded once the older sample joins the window.
	if want := 100.0 * 9 / 11; got.Month < want-0.01 || got.Month > want+0.01 {
		t.Errorf("30d uptime = %.2f, want %.2f", got.Month, want)
	}
	if got.AvgMS <= 0 {
		t.Errorf("avg latency = %.2f, want a positive average of the successful checks", got.AvgMS)
	}
	if got.P95MS <= 0 {
		t.Errorf("p95 latency = %.2f, want the window function to return a value", got.P95MS)
	}
}

func TestUptimeIsPerTarget(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	insert(t, s, "a.test", now, true, 100)
	insert(t, s, "b.test", now, false, 0)

	ups, err := s.Uptimes()
	if err != nil {
		t.Fatalf("uptimes: %v", err)
	}

	if ups["a.test"].Day != 100 || ups["b.test"].Day != 0 {
		t.Fatalf("a = %.0f, b = %.0f; a healthy target must not absorb a dead one",
			ups["a.test"].Day, ups["b.test"].Day)
	}
}

func TestSparksAreOldestFirst(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	// Written newest-last so the expected order is 100, 200, 300.
	insert(t, s, "a.test", now.Add(-3*time.Minute), true, 100)
	insert(t, s, "a.test", now.Add(-2*time.Minute), true, 200)
	insert(t, s, "a.test", now.Add(-time.Minute), true, 300)

	sparks, err := s.Sparks(2)
	if err != nil {
		t.Fatalf("sparks: %v", err)
	}

	got := sparks["a.test"]
	if len(got) != 2 {
		t.Fatalf("spark = %v, want the 2 most recent", got)
	}
	if got[0] != 200 || got[1] != 300 {
		t.Errorf("spark = %v, want [200 300] oldest first", got)
	}
}

func TestHistoryBuckets(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().Truncate(time.Hour)

	// Two samples in one hour, one of them failing.
	insert(t, s, "a.test", base.Add(-90*time.Minute), true, 200)
	insert(t, s, "a.test", base.Add(-80*time.Minute), false, 0)
	insert(t, s, "a.test", base.Add(-20*time.Minute), true, 400)

	points, err := s.History("a.test", base.Add(-3*time.Hour), time.Hour)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("got %d buckets, want 2", len(points))
	}
	if points[0].Total != 2 || points[0].Percent != 50 {
		t.Errorf("first bucket = %+v, want 2 samples at 50%%", points[0])
	}
	if points[1].Total != 1 || points[1].Percent != 100 {
		t.Errorf("second bucket = %+v, want 1 sample at 100%%", points[1])
	}
}

func TestIncidentLifecycle(t *testing.T) {
	s := newTestStore(t)
	start := time.Now().Add(-10 * time.Minute)

	id, err := s.OpenIncident("a.test", "timeout", "i/o timeout", start)
	if err != nil {
		t.Fatalf("open incident: %v", err)
	}

	active, err := s.ActiveIncidents()
	if err != nil {
		t.Fatalf("active: %v", err)
	}
	if len(active) != 1 || !active[0].Active {
		t.Fatalf("active incidents = %+v", active)
	}
	if active[0].DurationSec < 590 {
		t.Errorf("open incident duration = %ds, want it measured against now", active[0].DurationSec)
	}

	if err := s.CloseIncident(id, start.Add(5*time.Minute)); err != nil {
		t.Fatalf("close: %v", err)
	}

	active, err = s.ActiveIncidents()
	if err != nil {
		t.Fatalf("active after close: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("incident still open after close: %+v", active)
	}

	recent, err := s.RecentIncidents(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 1 || recent[0].DurationSec != 300 {
		t.Fatalf("closed incident = %+v, want a 300s duration", recent[0])
	}
}

func TestArchiveKeepsHistory(t *testing.T) {
	s := newTestStore(t)
	insert(t, s, "b.test", time.Now(), true, 120)

	// b.test is dropped from list-domain.txt.
	if err := s.SyncTargets([]model.Target{
		{ID: "a.test", Apex: "test", Host: "a.test", URL: "https://a.test", Label: "a"},
	}); err != nil {
		t.Fatalf("resync: %v", err)
	}

	ups, err := s.Uptimes()
	if err != nil {
		t.Fatalf("uptimes: %v", err)
	}
	if _, ok := ups["b.test"]; !ok {
		t.Fatal("history for an archived target was discarded")
	}
}

func TestPruneDropsOldChecksOnly(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()

	insert(t, s, "a.test", now.AddDate(0, 0, -40), true, 100)
	insert(t, s, "a.test", now, true, 100)

	oldIncident, err := s.OpenIncident("a.test", "timeout", "", now.AddDate(0, 0, -40))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.CloseIncident(oldIncident, now.AddDate(0, 0, -40).Add(time.Minute)); err != nil {
		t.Fatalf("close: %v", err)
	}

	if err := s.Prune(30); err != nil {
		t.Fatalf("prune: %v", err)
	}

	ups, err := s.Uptimes()
	if err != nil {
		t.Fatalf("uptimes: %v", err)
	}
	if ups["a.test"].Samples != 1 {
		t.Errorf("samples after prune = %d, want only the recent check", ups["a.test"].Samples)
	}

	// A 40-day-old incident is well inside the 180-day incident retention.
	recent, err := s.RecentIncidents(10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(recent) != 1 {
		t.Errorf("incidents after prune = %d, want the 40-day-old one kept", len(recent))
	}
}

func TestCertRoundTripRecomputesDaysLeft(t *testing.T) {
	s := newTestStore(t)

	notAfter := time.Now().Add(45 * 24 * time.Hour)
	stale := 999
	if err := s.SaveCert("a.test", &model.CertInfo{
		CheckedAt: time.Now(),
		NotAfter:  &notAfter,
		DaysLeft:  &stale,
		Issuer:    "Test CA",
	}); err != nil {
		t.Fatalf("save cert: %v", err)
	}

	certs, err := s.LoadCerts()
	if err != nil {
		t.Fatalf("load certs: %v", err)
	}

	got := certs["a.test"]
	if got == nil || got.DaysLeft == nil {
		t.Fatal("certificate did not survive the round trip")
	}
	if *got.DaysLeft != 44 && *got.DaysLeft != 45 {
		t.Errorf("days left = %d, want it recomputed to ~45 rather than the stored 999", *got.DaysLeft)
	}
}
