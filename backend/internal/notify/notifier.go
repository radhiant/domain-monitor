// Package notify defines the outbound alerting hook.
//
// Nothing is wired up yet — the dashboard is the deliverable, and a monitoring
// system that starts paging before its thresholds have been observed in the
// real world only teaches people to ignore it. The interface exists so adding
// a webhook or a Telegram sender later is one new file implementing Notifier,
// with no change to the scheduler.
package notify

import (
	"domain-monitor/backend/internal/model"
)

// Notifier receives the events an operator would want pushed to them.
type Notifier interface {
	// IncidentOpened fires once, when an outage crosses the failure threshold.
	IncidentOpened(inc *model.Incident, target model.Target)
	// IncidentClosed fires when the endpoint answers again.
	IncidentClosed(inc *model.Incident, target model.Target)
	// ExpiryWarning fires when a certificate or domain enters its warning window.
	ExpiryWarning(item model.ExpiryItem)
}

// Nop discards everything. It is the notifier the agent ships with.
type Nop struct{}

// IncidentOpened does nothing.
func (Nop) IncidentOpened(*model.Incident, model.Target) {}

// IncidentClosed does nothing.
func (Nop) IncidentClosed(*model.Incident, model.Target) {}

// ExpiryWarning does nothing.
func (Nop) ExpiryWarning(model.ExpiryItem) {}
