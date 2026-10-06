// Package config resolves runtime configuration from CLI flags, environment
// variables and defaults, in that order of precedence.
package config

import (
	"flag"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved agent configuration.
type Config struct {
	HTTPAddr    string
	TargetsFile string
	DBPath      string

	// Probe cadence. HTTP is the only one that runs often; certificates and
	// registration data move on the scale of months, so hammering them would
	// only annoy the registries.
	HTTPInterval   time.Duration
	TLSInterval    time.Duration
	DNSInterval    time.Duration
	RDAPInterval   time.Duration
	ReloadInterval time.Duration

	HTTPTimeout time.Duration
	TLSTimeout  time.Duration
	DNSTimeout  time.Duration
	RDAPTimeout time.Duration

	ProbeConcurrency int
	UserAgent        string

	// Thresholds. The backend classifies status server-side so every client
	// (wall, list view, future alerting) agrees on what "degraded" means.
	LatencyWarnMS  float64
	LatencyCritMS  float64
	CertWarnDays   int
	CertCritDays   int
	DomainWarnDays int
	DomainCritDays int

	// AcceptStatus lists HTTP status codes that count as healthy even though
	// they are not 2xx. An API or CDN host that answers 401 or 403 at its root
	// is working exactly as designed, and leaving those permanently amber
	// trains people to ignore the colour that matters.
	AcceptStatus []int

	FailThreshold int // consecutive failures before an incident opens
	RetentionDays int
	SparkPoints   int

	APIToken    string
	CorsOrigins []string
	LogLevel    string

	// Webhook / Alerting
	WebhookEnabled     bool
	WebhookURL         string
	WebhookChannel     string
	WebhookChatID      string
	WebhookWARecipient string
	CaptureURL         string
	WebhookBatchWait   time.Duration
}

// Load builds the configuration. Flags win over environment variables, which
// win over the defaults below.
func Load() *Config {
	envAddr := getEnv("HTTP_ADDR", ":9292")
	envTargets := getEnv("TARGETS_FILE", "../list-domain.txt")
	envDB := getEnv("DB_PATH", "./data/domain-monitor.db")
	envToken := getEnv("API_TOKEN", "")
	envOrigins := getEnv("CORS_ORIGINS", "*")
	envLogLevel := getEnv("LOG_LEVEL", "info")
	envUserAgent := getEnv("USER_AGENT", "domain-monitor/1.0 (+uptime probe)")

	fs := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flagAddr := fs.String("addr", envAddr, "HTTP and WebSocket listen address (e.g. :9292)")
	flagTargets := fs.String("targets", envTargets, "Path to list-domain.txt")
	flagDB := fs.String("db", envDB, "Path to the SQLite history database")
	flagToken := fs.String("token", envToken, "Optional API Bearer authentication token")
	flagOrigins := fs.String("cors", envOrigins, "Comma-separated allowed CORS origins")
	flagLogLevel := fs.String("log-level", envLogLevel, "Log verbosity level (debug, info, warn, error)")
	_ = fs.Parse(os.Args[1:])

	cfg := &Config{
		HTTPAddr:    *flagAddr,
		TargetsFile: *flagTargets,
		DBPath:      *flagDB,

		HTTPInterval:   getDuration("HTTP_INTERVAL", 60*time.Second, 10*time.Second),
		TLSInterval:    getDuration("TLS_INTERVAL", 6*time.Hour, time.Minute),
		DNSInterval:    getDuration("DNS_INTERVAL", 15*time.Minute, time.Minute),
		RDAPInterval:   getDuration("RDAP_INTERVAL", 12*time.Hour, time.Hour),
		ReloadInterval: getDuration("RELOAD_INTERVAL", 30*time.Second, 5*time.Second),

		HTTPTimeout: getDuration("HTTP_TIMEOUT", 10*time.Second, time.Second),
		TLSTimeout:  getDuration("TLS_TIMEOUT", 8*time.Second, time.Second),
		DNSTimeout:  getDuration("DNS_TIMEOUT", 5*time.Second, time.Second),
		RDAPTimeout: getDuration("RDAP_TIMEOUT", 15*time.Second, time.Second),

		ProbeConcurrency: getInt("PROBE_CONCURRENCY", 8, 1),
		UserAgent:        envUserAgent,

		LatencyWarnMS:  getFloat("LATENCY_WARN_MS", 800, 1),
		LatencyCritMS:  getFloat("LATENCY_CRIT_MS", 2000, 1),
		CertWarnDays:   getInt("CERT_WARN_DAYS", 30, 1),
		CertCritDays:   getInt("CERT_CRIT_DAYS", 14, 1),
		DomainWarnDays: getInt("DOMAIN_WARN_DAYS", 60, 1),
		DomainCritDays: getInt("DOMAIN_CRIT_DAYS", 30, 1),

		AcceptStatus: parseStatusList(getEnv("ACCEPT_STATUS", "401,403")),

		FailThreshold: getInt("FAIL_THRESHOLD", 2, 1),
		RetentionDays: getInt("RETENTION_DAYS", 30, 1),
		SparkPoints:   getInt("SPARK_POINTS", 24, 4),

		APIToken:    *flagToken,
		CorsOrigins: parseCommaList(*flagOrigins),
		LogLevel:    *flagLogLevel,

		WebhookEnabled:     getBool("WEBHOOK_ENABLED", false),
		WebhookURL:         getEnv("WEBHOOK_URL", ""),
		WebhookChannel:     getEnv("WEBHOOK_CHANNEL", "both"),
		WebhookChatID:      getEnv("WEBHOOK_CHAT_ID", ""),
		WebhookWARecipient: getEnv("WEBHOOK_WA_RECIPIENT", ""),
		CaptureURL:         getEnv("CAPTURE_URL", "http://localhost:8082/"),
		WebhookBatchWait:   getDuration("WEBHOOK_BATCH_WAIT", 10*time.Second, time.Second),
	}

	// A critical threshold below its warning threshold would make the warning
	// unreachable, so keep the pair ordered no matter what was configured.
	if cfg.LatencyCritMS < cfg.LatencyWarnMS {
		cfg.LatencyCritMS = cfg.LatencyWarnMS
	}
	if cfg.CertCritDays > cfg.CertWarnDays {
		cfg.CertCritDays = cfg.CertWarnDays
	}
	if cfg.DomainCritDays > cfg.DomainWarnDays {
		cfg.DomainCritDays = cfg.DomainWarnDays
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && strings.TrimSpace(val) != "" {
		return strings.TrimSpace(val)
	}
	return fallback
}

func getDuration(key string, fallback, min time.Duration) time.Duration {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < min {
		return fallback
	}
	return d
}

func getInt(key string, fallback, min int) int {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min {
		return fallback
	}
	return n
}

func getFloat(key string, fallback, min float64) float64 {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil || f < min {
		return fallback
	}
	return f
}

// parseStatusList reads "401,403" into status codes, ignoring anything that is
// not a plausible HTTP status.
func parseStatusList(raw string) []int {
	out := make([]int, 0, 4)
	for _, part := range strings.Split(raw, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 100 || n > 599 {
			continue
		}
		out = append(out, n)
	}
	return out
}

func parseCommaList(raw string) []string {
	if strings.TrimSpace(raw) == "" || raw == "*" {
		return []string{"*"}
	}
	parts := strings.Split(raw, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	if len(res) == 0 {
		return []string{"*"}
	}
	return res
}

func getBool(key string, fallback bool) bool {
	raw := getEnv(key, "")
	if raw == "" {
		return fallback
	}
	val, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return val
}
