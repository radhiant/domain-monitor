package incident

import (
	"testing"
	"time"

	"domain-monitor/backend/internal/model"
)

func TestSingleFailureDoesNotOpenIncident(t *testing.T) {
	tr := NewTracker(2)
	now := time.Now()

	obs := tr.Observe("a.test", model.StatusDown, now, "timeout", "i/o timeout")
	if obs.Action.Type != ActionNone {
		t.Fatalf("one failure opened an incident: %+v", obs.Action)
	}

	// Recovery before the threshold must leave nothing to close.
	obs = tr.Observe("a.test", model.StatusUp, now.Add(time.Minute), "", "")
	if obs.Action.Type != ActionNone {
		t.Fatalf("recovery after a blip produced %+v", obs.Action)
	}
}

func TestTwoFailuresOpenAndSuccessCloses(t *testing.T) {
	tr := NewTracker(2)
	start := time.Now()

	tr.Observe("a.test", model.StatusDown, start, "timeout", "i/o timeout")
	obs := tr.Observe("a.test", model.StatusDown, start.Add(time.Minute), "timeout", "i/o timeout")
	if obs.Action.Type != ActionOpen {
		t.Fatalf("second failure did not open an incident: %+v", obs.Action)
	}
	if obs.Action.LastError != "i/o timeout" {
		t.Errorf("open action lost the error: %q", obs.Action.LastError)
	}

	tr.SetIncidentID("a.test", 42)

	// Still failing: the incident stays open and is not duplicated.
	obs = tr.Observe("a.test", model.StatusDown, start.Add(2*time.Minute), "timeout", "i/o timeout")
	if obs.Action.Type != ActionNone {
		t.Fatalf("continued failure produced %+v, want no action", obs.Action)
	}

	obs = tr.Observe("a.test", model.StatusUp, start.Add(3*time.Minute), "", "")
	if obs.Action.Type != ActionClose {
		t.Fatalf("recovery did not close the incident: %+v", obs.Action)
	}
	if obs.Action.IncidentID != 42 {
		t.Errorf("close targeted incident %d, want 42", obs.Action.IncidentID)
	}
}

func TestDegradedCountsAsReachable(t *testing.T) {
	tr := NewTracker(2)
	now := time.Now()

	tr.Observe("a.test", model.StatusDown, now, "timeout", "")
	tr.Observe("a.test", model.StatusDown, now, "timeout", "")

	// A slow but answering endpoint is not an outage, so it closes the window.
	obs := tr.Observe("a.test", model.StatusDegraded, now, "", "")
	if obs.Action.Type != ActionClose {
		t.Fatalf("degraded did not close the incident: %+v", obs.Action)
	}
}

func TestUnknownDoesNotAdvanceTheCounter(t *testing.T) {
	tr := NewTracker(2)
	now := time.Now()

	tr.Observe("a.test", model.StatusDown, now, "timeout", "")
	obs := tr.Observe("a.test", model.StatusUnknown, now, "", "")
	if obs.Action.Type != ActionNone {
		t.Fatalf("unknown produced %+v", obs.Action)
	}

	// The earlier failure still stands, so this one crosses the threshold.
	obs = tr.Observe("a.test", model.StatusDown, now, "timeout", "")
	if obs.Action.Type != ActionOpen {
		t.Fatalf("expected the incident to open here: %+v", obs.Action)
	}
}

func TestRestoreContinuesAnOpenIncident(t *testing.T) {
	tr := NewTracker(2)
	tr.Restore("a.test", 7)

	// The first check after a restart must not open a second incident.
	obs := tr.Observe("a.test", model.StatusDown, time.Now(), "timeout", "")
	if obs.Action.Type != ActionNone {
		t.Fatalf("restored incident was duplicated: %+v", obs.Action)
	}

	obs = tr.Observe("a.test", model.StatusUp, time.Now(), "", "")
	if obs.Action.Type != ActionClose || obs.Action.IncidentID != 7 {
		t.Fatalf("close = %+v, want incident 7", obs.Action)
	}
}

func TestTransitionIsReported(t *testing.T) {
	tr := NewTracker(2)
	now := time.Now()

	obs := tr.Observe("a.test", model.StatusUp, now, "", "")
	if !obs.Changed || obs.From != model.StatusUnknown || obs.To != model.StatusUp {
		t.Fatalf("first observation = %+v", obs)
	}

	obs = tr.Observe("a.test", model.StatusUp, now, "", "")
	if obs.Changed {
		t.Fatalf("repeat observation reported a change: %+v", obs)
	}
}
