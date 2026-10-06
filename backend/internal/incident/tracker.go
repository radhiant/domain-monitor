// Package incident turns a stream of per-check verdicts into open and closed
// outage windows.
package incident

import (
	"sync"
	"time"

	"domain-monitor/backend/internal/model"
)

// ActionType is what the caller must persist as a result of one observation.
type ActionType int

const (
	// ActionNone means the observation confirmed the current state.
	ActionNone ActionType = iota
	// ActionOpen means an outage just crossed the failure threshold.
	ActionOpen
	// ActionClose means an open outage has recovered.
	ActionClose
)

// Action describes the store write an observation calls for.
type Action struct {
	Type       ActionType
	TargetID   string
	At         time.Time
	Reason     string
	LastError  string
	IncidentID int64 // set for ActionClose
}

// Observation is the result of feeding one check into the tracker.
type Observation struct {
	Action  Action
	From    model.Status
	To      model.Status
	Changed bool
}

type targetState struct {
	fails      int
	incidentID int64
	open       bool
	last       model.Status
	lastError  string
}

// Tracker debounces flapping targets.
//
// A single failed check is usually a blip: a dropped packet, a momentary
// upstream hiccup. Opening an incident for each one would bury the real
// outages, so an incident needs `threshold` consecutive failures to open. It
// closes on the first success, because recovery is never in doubt.
type Tracker struct {
	mu        sync.Mutex
	threshold int
	state     map[string]*targetState
}

// NewTracker returns a tracker requiring threshold consecutive failures.
func NewTracker(threshold int) *Tracker {
	if threshold < 1 {
		threshold = 1
	}
	return &Tracker{
		threshold: threshold,
		state:     make(map[string]*targetState),
	}
}

// Restore re-attaches an incident that was still open when the agent stopped,
// so a restart continues the outage instead of starting a duplicate one.
func (t *Tracker) Restore(targetID string, incidentID int64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	st := t.get(targetID)
	st.open = true
	st.incidentID = incidentID
	st.fails = t.threshold
	st.last = model.StatusDown
}

// Observe feeds one classified check result in and reports what to persist.
func (t *Tracker) Observe(targetID string, status model.Status, at time.Time, reason, errMsg string) Observation {
	t.mu.Lock()
	defer t.mu.Unlock()

	st := t.get(targetID)
	from := st.last
	obs := Observation{
		From:    from,
		To:      status,
		Changed: from != status,
	}
	st.last = status

	switch status {
	case model.StatusUnknown:
		// Not yet probed, or the probe itself could not run. That neither
		// confirms nor denies an outage, so the counters are left alone.
		return obs

	case model.StatusDown:
		st.fails++
		st.lastError = errMsg
		if !st.open && st.fails >= t.threshold {
			st.open = true
			obs.Action = Action{
				Type:      ActionOpen,
				TargetID:  targetID,
				At:        at,
				Reason:    reason,
				LastError: errMsg,
			}
		}
		return obs

	default: // UP or DEGRADED, meaning the endpoint answered.
		st.fails = 0
		if st.open {
			st.open = false
			obs.Action = Action{
				Type:       ActionClose,
				TargetID:   targetID,
				At:         at,
				IncidentID: st.incidentID,
				LastError:  st.lastError,
			}
			st.incidentID = 0
		}
		return obs
	}
}

// SetIncidentID records the row id the store assigned to a freshly opened
// incident, so the matching close updates the same row.
func (t *Tracker) SetIncidentID(targetID string, id int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.get(targetID).incidentID = id
}

// Forget drops all state for a target that has left list-domain.txt.
func (t *Tracker) Forget(targetID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.state, targetID)
}

// get returns the state for a target, creating it on first sight.
// The caller must hold the mutex.
func (t *Tracker) get(targetID string) *targetState {
	st, ok := t.state[targetID]
	if !ok {
		st = &targetState{last: model.StatusUnknown}
		t.state[targetID] = st
	}
	return st
}
