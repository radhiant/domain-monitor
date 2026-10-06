package handler

import (
	"fmt"
	"net/http"
	"strings"

	"domain-monitor/backend/internal/model"
	"domain-monitor/backend/internal/state"
)

// PrometheusHandler exposes domain monitoring metrics in standard Prometheus exposition format.
type PrometheusHandler struct {
	snap *state.Snapshot
}

// NewPrometheusHandler constructs a new PrometheusHandler.
func NewPrometheusHandler(snap *state.Snapshot) *PrometheusHandler {
	return &PrometheusHandler{snap: snap}
}

// escapeLabel escapes Prometheus label values.
func escapeLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

// ServeHTTP writes the Prometheus exposition format output.
func (h *PrometheusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ov := h.snap.Overview()
	states := h.snap.States()
	groups := h.snap.Groups()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	var b strings.Builder

	// Overview KPIs
	b.WriteString("# HELP domain_overview_domains_total Total tracked apex domains\n")
	b.WriteString("# TYPE domain_overview_domains_total gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_domains_total %d\n", ov.TotalDomains))

	b.WriteString("# HELP domain_overview_targets_total Total tracked endpoints/targets\n")
	b.WriteString("# TYPE domain_overview_targets_total gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_targets_total %d\n", ov.TotalEndpoints))

	b.WriteString("# HELP domain_overview_up_total Targets currently in UP state\n")
	b.WriteString("# TYPE domain_overview_up_total gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_up_total %d\n", ov.Up))

	b.WriteString("# HELP domain_overview_degraded_total Targets currently in DEGRADED state\n")
	b.WriteString("# TYPE domain_overview_degraded_total gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_degraded_total %d\n", ov.Degraded))

	b.WriteString("# HELP domain_overview_down_total Targets currently in DOWN state\n")
	b.WriteString("# TYPE domain_overview_down_total gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_down_total %d\n", ov.Down))

	b.WriteString("# HELP domain_overview_unknown_total Targets currently in UNKNOWN state\n")
	b.WriteString("# TYPE domain_overview_unknown_total gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_unknown_total %d\n", ov.Unknown))

	b.WriteString("# HELP domain_overview_uptime_day_percent Average 24-hour fleet uptime percentage\n")
	b.WriteString("# TYPE domain_overview_uptime_day_percent gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_uptime_day_percent %.2f\n", ov.UptimeDay))

	b.WriteString("# HELP domain_overview_latency_avg_seconds Average response latency across all targets in seconds\n")
	b.WriteString("# TYPE domain_overview_latency_avg_seconds gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_latency_avg_seconds %.4f\n", ov.AvgLatencyMS/1000.0))

	b.WriteString("# HELP domain_overview_active_incidents Currently active incident outage count\n")
	b.WriteString("# TYPE domain_overview_active_incidents gauge\n")
	b.WriteString(fmt.Sprintf("domain_overview_active_incidents %d\n", ov.ActiveIncident))

	// Per-Target Metrics
	if len(states) > 0 {
		b.WriteString("\n# HELP domain_probe_success Target probe success (1 if UP, 0 otherwise)\n")
		b.WriteString("# TYPE domain_probe_success gauge\n")
		for _, st := range states {
			val := 0
			if st.Status == model.StatusUp {
				val = 1
			}
			b.WriteString(fmt.Sprintf("domain_probe_success{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %d\n",
				escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), val))
		}

		b.WriteString("\n# HELP domain_status Current categorical status of target (1 for active state)\n")
		b.WriteString("# TYPE domain_status gauge\n")
		for _, st := range states {
			for _, s := range []model.Status{model.StatusUp, model.StatusDegraded, model.StatusDown, model.StatusUnknown} {
				val := 0
				if st.Status == s {
					val = 1
				}
				b.WriteString(fmt.Sprintf("domain_status{target_id=\"%s\",host=\"%s\",apex=\"%s\",status=\"%s\"} %d\n",
					escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), s, val))
			}
		}

		b.WriteString("\n# HELP domain_http_latency_seconds HTTP probe roundtrip latency in seconds\n")
		b.WriteString("# TYPE domain_http_latency_seconds gauge\n")
		for _, st := range states {
			if st.HTTP != nil {
				b.WriteString(fmt.Sprintf("domain_http_latency_seconds{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %.4f\n",
					escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), st.HTTP.LatencyMS/1000.0))
			}
		}

		b.WriteString("\n# HELP domain_http_ttfb_seconds HTTP time to first byte in seconds\n")
		b.WriteString("# TYPE domain_http_ttfb_seconds gauge\n")
		for _, st := range states {
			if st.HTTP != nil {
				b.WriteString(fmt.Sprintf("domain_http_ttfb_seconds{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %.4f\n",
					escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), st.HTTP.TTFBMS/1000.0))
			}
		}

		b.WriteString("\n# HELP domain_http_status_code HTTP response status code\n")
		b.WriteString("# TYPE domain_http_status_code gauge\n")
		for _, st := range states {
			if st.HTTP != nil {
				b.WriteString(fmt.Sprintf("domain_http_status_code{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %d\n",
					escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), st.HTTP.StatusCode))
			}
		}

		b.WriteString("\n# HELP domain_ssl_expiry_days Remaining days before TLS certificate expires\n")
		b.WriteString("# TYPE domain_ssl_expiry_days gauge\n")
		for _, st := range states {
			if st.Cert != nil && st.Cert.DaysLeft != nil {
				b.WriteString(fmt.Sprintf("domain_ssl_expiry_days{target_id=\"%s\",host=\"%s\",apex=\"%s\",issuer=\"%s\"} %d\n",
					escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), escapeLabel(st.Cert.Issuer), *st.Cert.DaysLeft))
			}
		}

		b.WriteString("\n# HELP domain_dns_resolve_seconds DNS resolution latency in seconds\n")
		b.WriteString("# TYPE domain_dns_resolve_seconds gauge\n")
		for _, st := range states {
			if st.DNS != nil {
				b.WriteString(fmt.Sprintf("domain_dns_resolve_seconds{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %.4f\n",
					escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), st.DNS.ResolveMS/1000.0))
			}
		}

		b.WriteString("\n# HELP domain_uptime_day_percent 24-hour uptime percentage\n")
		b.WriteString("# TYPE domain_uptime_day_percent gauge\n")
		for _, st := range states {
			b.WriteString(fmt.Sprintf("domain_uptime_day_percent{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %.2f\n",
				escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), st.Uptime.Day))
		}

		b.WriteString("\n# HELP domain_uptime_week_percent 7-day uptime percentage\n")
		b.WriteString("# TYPE domain_uptime_week_percent gauge\n")
		for _, st := range states {
			b.WriteString(fmt.Sprintf("domain_uptime_week_percent{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %.2f\n",
				escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), st.Uptime.Week))
		}

		b.WriteString("\n# HELP domain_uptime_month_percent 30-day uptime percentage\n")
		b.WriteString("# TYPE domain_uptime_month_percent gauge\n")
		for _, st := range states {
			b.WriteString(fmt.Sprintf("domain_uptime_month_percent{target_id=\"%s\",host=\"%s\",apex=\"%s\"} %.2f\n",
				escapeLabel(st.ID), escapeLabel(st.Host), escapeLabel(st.Apex), st.Uptime.Month))
		}
	}

	// Registration Expiry per Apex Domain
	if len(groups) > 0 {
		b.WriteString("\n# HELP domain_registration_expiry_days Remaining days before apex domain registration expires\n")
		b.WriteString("# TYPE domain_registration_expiry_days gauge\n")
		for _, g := range groups {
			if g.Domain != nil && g.Domain.DaysLeft != nil {
				b.WriteString(fmt.Sprintf("domain_registration_expiry_days{apex=\"%s\",registrar=\"%s\"} %d\n",
					escapeLabel(g.Apex), escapeLabel(g.Domain.Registrar), *g.Domain.DaysLeft))
			}
		}
	}

	_, _ = w.Write([]byte(b.String()))
}
