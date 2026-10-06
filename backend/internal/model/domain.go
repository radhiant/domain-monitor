// Package model holds the wire types shared by the probes, the store and the
// HTTP/WebSocket handlers. Every struct here is JSON-encoded straight to the
// dashboard, so field names are snake_case to match the server-monitor project.
package model

import "time"

// Status is the single health verdict the wall display colours a tile by.
type Status string

const (
	StatusUp       Status = "UP"
	StatusDegraded Status = "DEGRADED"
	StatusDown     Status = "DOWN"
	StatusUnknown  Status = "UNKNOWN"
)

// rank orders statuses from best to worst so a group can roll up to its worst
// member. Averaging would hide exactly the case that matters: one dead
// subdomain inside an otherwise healthy domain.
var rank = map[Status]int{
	StatusUnknown:  0,
	StatusUp:       1,
	StatusDegraded: 2,
	StatusDown:     3,
}

// WorseOf returns whichever status is more alarming.
func WorseOf(a, b Status) Status {
	if rank[b] > rank[a] {
		return b
	}
	return a
}

// ErrorKind classifies a failed probe so the UI can say *why* something is
// down instead of showing a raw Go error string on a TV across the room.
type ErrorKind string

const (
	ErrNone    ErrorKind = ""
	ErrDNS     ErrorKind = "dns"
	ErrConnect ErrorKind = "connect"
	ErrTLS     ErrorKind = "tls"
	ErrTimeout ErrorKind = "timeout"
	ErrHTTP    ErrorKind = "http"
)

// Target is one monitored endpoint parsed out of list-domain.txt.
type Target struct {
	ID     string `json:"id"`    // hostname; stable primary key and detail-view slug
	Apex   string `json:"apex"`  // registrable domain this endpoint belongs to
	Host   string `json:"host"`  // hostname without scheme
	URL    string `json:"url"`   // full probe URL
	Label  string `json:"label"` // leading label ("app"), or "@" for the apex itself
	IsApex bool   `json:"is_apex"`
	Order  int    `json:"order"` // position in list-domain.txt, preserved for display
}

// HTTPResult is one reachability sample.
type HTTPResult struct {
	CheckedAt  time.Time `json:"checked_at"`
	Up         bool      `json:"up"`
	StatusCode int       `json:"status_code"`
	LatencyMS  float64   `json:"latency_ms"`
	TTFBMS     float64   `json:"ttfb_ms"`
	BodyBytes  int64     `json:"body_bytes"`
	FinalURL   string    `json:"final_url"`
	Redirects  []string  `json:"redirects,omitempty"`
	Error      string    `json:"error,omitempty"`
	ErrorKind  ErrorKind `json:"error_kind,omitempty"`
}

// CertInfo is the TLS leaf certificate as of the last handshake.
type CertInfo struct {
	CheckedAt  time.Time  `json:"checked_at"`
	NotBefore  *time.Time `json:"not_before,omitempty"`
	NotAfter   *time.Time `json:"not_after,omitempty"`
	DaysLeft   *int       `json:"days_left,omitempty"`
	Issuer     string     `json:"issuer,omitempty"`
	Subject    string     `json:"subject,omitempty"`
	SANs       []string   `json:"sans,omitempty"`
	TLSVersion string     `json:"tls_version,omitempty"`
	Cipher     string     `json:"cipher,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// DNSInfo is the last resolution result for a host.
type DNSInfo struct {
	CheckedAt time.Time `json:"checked_at"`
	Addrs     []string  `json:"addrs,omitempty"`
	CNAME     string    `json:"cname,omitempty"`
	NS        []string  `json:"ns,omitempty"`
	ResolveMS float64   `json:"resolve_ms"`
	Error     string    `json:"error,omitempty"`
}

// DomainInfo is registration data for an apex domain, from RDAP or WHOIS.
type DomainInfo struct {
	Apex      string     `json:"apex"`
	CheckedAt time.Time  `json:"checked_at"`
	Registrar string     `json:"registrar,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	DaysLeft  *int       `json:"days_left,omitempty"`
	Statuses  []string   `json:"statuses,omitempty"`
	Source    string     `json:"source,omitempty"` // "rdap" or "whois"
	Error     string     `json:"error,omitempty"`
}

// Uptime is the rolled-up availability record for one endpoint.
type Uptime struct {
	Day     float64 `json:"day"`   // last 24h, percent
	Week    float64 `json:"week"`  // last 7d, percent
	Month   float64 `json:"month"` // last 30d, percent
	AvgMS   float64 `json:"avg_ms"`
	P95MS   float64 `json:"p95_ms"`
	Samples int     `json:"samples"`
}

// Incident is one continuous outage window.
type Incident struct {
	ID          int64      `json:"id"`
	TargetID    string     `json:"target_id"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	DurationSec int64      `json:"duration_sec"`
	Reason      string     `json:"reason"`
	LastError   string     `json:"last_error,omitempty"`
	Active      bool       `json:"active"`
}

// Event is a status transition, rendered in the wall ticker.
type Event struct {
	At       time.Time `json:"at"`
	TargetID string    `json:"target_id"`
	From     Status    `json:"from"`
	To       Status    `json:"to"`
	Detail   string    `json:"detail,omitempty"`
}

// TargetState is everything the dashboard needs about one endpoint in a single
// object, so the wall view never has to join data client-side.
type TargetState struct {
	Target
	Status         Status      `json:"status"`
	Reason         string      `json:"reason,omitempty"`
	HTTP           *HTTPResult `json:"http,omitempty"`
	Cert           *CertInfo   `json:"cert,omitempty"`
	DNS            *DNSInfo    `json:"dns,omitempty"`
	Uptime         Uptime      `json:"uptime"`
	Spark          []float64   `json:"spark"` // recent latencies, oldest first
	ActiveIncident *Incident   `json:"active_incident,omitempty"`
}

// DomainGroup is one apex domain with its endpoints and registration data.
type DomainGroup struct {
	Apex      string         `json:"apex"`
	Status    Status         `json:"status"`
	Domain    *DomainInfo    `json:"domain,omitempty"`
	Endpoints []*TargetState `json:"endpoints"`
	UpCount   int            `json:"up_count"`
	Total     int            `json:"total"`
}

// ExpiryItem is one row of the combined SSL + domain expiry watchlist.
type ExpiryItem struct {
	Kind      string    `json:"kind"` // "ssl" or "domain"
	TargetID  string    `json:"target_id"`
	Label     string    `json:"label"`
	Apex      string    `json:"apex"`
	ExpiresAt time.Time `json:"expires_at"`
	DaysLeft  int       `json:"days_left"`
	Issuer    string    `json:"issuer,omitempty"`
	Registrar string    `json:"registrar,omitempty"`
}

// Overview drives the KPI band at the top of the wall display.
type Overview struct {
	GeneratedAt    time.Time    `json:"generated_at"`
	TotalDomains   int          `json:"total_domains"`
	TotalEndpoints int          `json:"total_endpoints"`
	Up             int          `json:"up"`
	Degraded       int          `json:"degraded"`
	Down           int          `json:"down"`
	Unknown        int          `json:"unknown"`
	UptimeDay      float64      `json:"uptime_day"`
	AvgLatencyMS   float64      `json:"avg_latency_ms"`
	SlowestID      string       `json:"slowest_id,omitempty"`
	SlowestMS      float64      `json:"slowest_ms"`
	ActiveIncident int          `json:"active_incidents"`
	NextSSL        *ExpiryItem  `json:"next_ssl,omitempty"`
	NextDomain     *ExpiryItem  `json:"next_domain,omitempty"`
	LastScan       *time.Time   `json:"last_scan,omitempty"`
	Events         []Event      `json:"events"`
	Incidents      []*Incident  `json:"incidents"`
	Expiry         []ExpiryItem `json:"expiry"`
}

// WSMessage is the envelope every websocket frame uses.
//
// Types: "snapshot" on connect, "update" for one changed endpoint, "overview"
// for the KPI band, "event" for a status transition, and "pong".
type WSMessage struct {
	Type      string `json:"type"`
	Timestamp int64  `json:"timestamp"`
	Data      any    `json:"data,omitempty"`
}

// ErrorResponse mirrors the server-monitor error envelope.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail is the machine-readable part of an error response.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
