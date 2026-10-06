// Package state holds the live view of every target: the classifier that turns
// raw probe output into a status, and the snapshot the API serves from.
package state

import (
	"fmt"

	"domain-monitor/backend/internal/model"
)

// Thresholds are the boundaries between healthy, degraded and down.
type Thresholds struct {
	LatencyWarnMS  float64
	LatencyCritMS  float64
	CertWarnDays   int
	CertCritDays   int
	DomainWarnDays int
	DomainCritDays int

	// AcceptStatus are non-2xx codes that still count as healthy.
	AcceptStatus []int
}

// accepts reports whether a status code was explicitly declared healthy.
func (t Thresholds) accepts(code int) bool {
	for _, c := range t.AcceptStatus {
		if c == code {
			return true
		}
	}
	return false
}

// Classify decides the single status a wall tile is coloured by, and the short
// human reason shown next to it.
//
// Classification lives on the server on purpose: the wall, the list view and
// any future alerting must never disagree about what "degraded" means.
// judgeMS is the latency the verdict is based on. Callers pass a short rolling
// average rather than the last sample: a single slow response is usually one
// congested moment, and letting it flip a tile amber turns a wall display into
// a flicker that people learn to ignore. Pass 0 to judge the sample itself.
func Classify(th Thresholds, res *model.HTTPResult, cert *model.CertInfo, judgeMS float64) (model.Status, string) {
	if res == nil {
		return model.StatusUnknown, "belum diperiksa"
	}

	if judgeMS <= 0 {
		judgeMS = res.LatencyMS
	}

	if !res.Up {
		return model.StatusDown, downReason(res)
	}

	// An expired or misissued certificate means every browser refuses the site,
	// so it is an outage even though the probe itself got an answer.
	if cert != nil {
		if cert.DaysLeft != nil && *cert.DaysLeft <= 0 {
			return model.StatusDown, "sertifikat kedaluwarsa"
		}
		if cert.Error != "" {
			return model.StatusDown, "sertifikat tidak valid"
		}
	}

	if res.StatusCode >= 400 && !th.accepts(res.StatusCode) {
		return model.StatusDegraded, fmt.Sprintf("HTTP %d", res.StatusCode)
	}

	if cert != nil && cert.DaysLeft != nil && *cert.DaysLeft <= th.CertCritDays {
		return model.StatusDegraded, fmt.Sprintf("SSL %d hari lagi", *cert.DaysLeft)
	}

	if th.LatencyCritMS > 0 && judgeMS >= th.LatencyCritMS {
		return model.StatusDegraded, fmt.Sprintf("sangat lambat %.0f ms", judgeMS)
	}
	if th.LatencyWarnMS > 0 && judgeMS >= th.LatencyWarnMS {
		return model.StatusDegraded, fmt.Sprintf("lambat %.0f ms", judgeMS)
	}

	return model.StatusUp, ""
}

// downReason turns a transport failure into wording that is readable across a
// room, instead of a wrapped Go error.
func downReason(res *model.HTTPResult) string {
	switch res.ErrorKind {
	case model.ErrDNS:
		return "DNS gagal"
	case model.ErrConnect:
		return "koneksi ditolak"
	case model.ErrTLS:
		return "TLS gagal"
	case model.ErrTimeout:
		return "timeout"
	case model.ErrHTTP:
		if res.StatusCode > 0 {
			return fmt.Sprintf("HTTP %d", res.StatusCode)
		}
	}
	if res.Error != "" {
		return res.Error
	}
	return "tidak merespons"
}

// ExpiryStatus grades a remaining-days count for the expiry chips and the
// watchlist. Anything already past its date is critical, not merely warning.
func ExpiryStatus(days, warnDays, critDays int) model.Status {
	switch {
	case days <= 0:
		return model.StatusDown
	case days <= critDays:
		return model.StatusDown
	case days <= warnDays:
		return model.StatusDegraded
	default:
		return model.StatusUp
	}
}
