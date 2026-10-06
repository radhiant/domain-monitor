package state

import (
	"testing"
	"time"

	"domain-monitor/backend/internal/model"
)

func testThresholds() Thresholds {
	return Thresholds{
		LatencyWarnMS:  800,
		LatencyCritMS:  2000,
		CertWarnDays:   30,
		CertCritDays:   14,
		DomainWarnDays: 60,
		DomainCritDays: 30,
		AcceptStatus:   []int{401, 403},
	}
}

func certExpiringIn(days int) *model.CertInfo {
	notAfter := time.Now().AddDate(0, 0, days)
	return &model.CertInfo{NotAfter: &notAfter, DaysLeft: &days}
}

func TestClassifyHealthy(t *testing.T) {
	status, reason := Classify(testThresholds(),
		&model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 120},
		certExpiringIn(80), 0)

	if status != model.StatusUp || reason != "" {
		t.Fatalf("status = %s (%q), want UP with no reason", status, reason)
	}
}

func TestClassifyUnknownBeforeFirstProbe(t *testing.T) {
	if status, _ := Classify(testThresholds(), nil, nil, 0); status != model.StatusUnknown {
		t.Fatalf("status = %s, want UNKNOWN", status)
	}
}

func TestClassifyTransportFailureIsDown(t *testing.T) {
	cases := map[model.ErrorKind]string{
		model.ErrDNS:     "DNS gagal",
		model.ErrConnect: "koneksi ditolak",
		model.ErrTLS:     "TLS gagal",
		model.ErrTimeout: "timeout",
	}

	for kind, wantReason := range cases {
		status, reason := Classify(testThresholds(),
			&model.HTTPResult{Up: false, ErrorKind: kind}, nil, 0)
		if status != model.StatusDown {
			t.Errorf("%s: status = %s, want DOWN", kind, status)
		}
		if reason != wantReason {
			t.Errorf("%s: reason = %q, want %q", kind, reason, wantReason)
		}
	}
}

func TestExpiredCertificateIsDownEvenWhenReachable(t *testing.T) {
	// The probe would normally fail first, but a warm-started record can carry
	// an already-expired certificate. Browsers refuse the site either way.
	status, reason := Classify(testThresholds(),
		&model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 100},
		certExpiringIn(0), 0)

	if status != model.StatusDown {
		t.Fatalf("status = %s (%q), want DOWN", status, reason)
	}
}

func TestCertificateInsideCriticalWindowDegrades(t *testing.T) {
	status, reason := Classify(testThresholds(),
		&model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 100},
		certExpiringIn(10), 0)

	if status != model.StatusDegraded {
		t.Fatalf("status = %s, want DEGRADED", status)
	}
	if reason == "" {
		t.Error("a degraded certificate must explain itself on the tile")
	}
}

func TestLatencyBands(t *testing.T) {
	th := testThresholds()

	if status, _ := Classify(th, &model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 799}, nil, 0); status != model.StatusUp {
		t.Errorf("799ms = %s, want UP just under the warning band", status)
	}
	if status, _ := Classify(th, &model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 900}, nil, 0); status != model.StatusDegraded {
		t.Errorf("900ms = %s, want DEGRADED", status)
	}
	if status, _ := Classify(th, &model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 5000}, nil, 0); status != model.StatusDegraded {
		t.Errorf("5000ms = %s, want DEGRADED, not DOWN: a slow answer is still an answer", status)
	}
}

func TestAcceptedStatusCodesStayHealthy(t *testing.T) {
	th := testThresholds()

	// An API root that refuses anonymous callers is working as designed.
	for _, code := range []int{401, 403} {
		if status, _ := Classify(th, &model.HTTPResult{Up: true, StatusCode: code, LatencyMS: 50}, nil, 0); status != model.StatusUp {
			t.Errorf("HTTP %d = %s, want UP because it is in the accept list", code, status)
		}
	}

	// 404 is not accepted by default: a missing root usually means a bad deploy.
	if status, reason := Classify(th, &model.HTTPResult{Up: true, StatusCode: 404, LatencyMS: 50}, nil, 0); status != model.StatusDegraded {
		t.Errorf("HTTP 404 = %s (%q), want DEGRADED", status, reason)
	}
}

func TestServerErrorIsDown(t *testing.T) {
	status, _ := Classify(testThresholds(),
		&model.HTTPResult{Up: false, StatusCode: 502, ErrorKind: model.ErrHTTP}, nil, 0)
	if status != model.StatusDown {
		t.Fatalf("HTTP 502 = %s, want DOWN", status)
	}
}

func TestSmoothedLatencyOverridesTheSample(t *testing.T) {
	th := testThresholds()

	// One slow response inside an otherwise fast window must not flip the tile:
	// the smoothed figure is what the verdict is based on.
	status, _ := Classify(th, &model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 2400}, nil, 300)
	if status != model.StatusUp {
		t.Errorf("spike with a calm average = %s, want UP", status)
	}

	// Sustained slowness still degrades, even if this one sample looks fine.
	status, reason := Classify(th, &model.HTTPResult{Up: true, StatusCode: 200, LatencyMS: 120}, nil, 1400)
	if status != model.StatusDegraded {
		t.Errorf("sustained slowness = %s (%q), want DEGRADED", status, reason)
	}
}

func TestExpiryStatus(t *testing.T) {
	cases := []struct {
		days int
		want model.Status
	}{
		{-1, model.StatusDown},
		{0, model.StatusDown},
		{10, model.StatusDown},
		{20, model.StatusDegraded},
		{90, model.StatusUp},
	}

	for _, tc := range cases {
		if got := ExpiryStatus(tc.days, 30, 14); got != tc.want {
			t.Errorf("ExpiryStatus(%d) = %s, want %s", tc.days, got, tc.want)
		}
	}
}

func TestWorseOfPicksTheAlarmingOne(t *testing.T) {
	// A group must never average a dead endpoint away.
	if got := model.WorseOf(model.StatusUp, model.StatusDown); got != model.StatusDown {
		t.Errorf("WorseOf(UP, DOWN) = %s", got)
	}
	if got := model.WorseOf(model.StatusDegraded, model.StatusUp); got != model.StatusDegraded {
		t.Errorf("WorseOf(DEGRADED, UP) = %s", got)
	}
	if got := model.WorseOf(model.StatusUnknown, model.StatusUp); got != model.StatusUp {
		t.Errorf("WorseOf(UNKNOWN, UP) = %s, want a probed target to win over an unprobed one", got)
	}
}
