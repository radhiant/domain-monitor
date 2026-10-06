package probe

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// ianaWhois is the root server used to discover which WHOIS server is
// authoritative for a TLD, so no per-TLD table has to be maintained here.
const ianaWhois = "whois.iana.org:43"

// whoisRecord is the subset of a WHOIS response the dashboard displays.
type whoisRecord struct {
	Registrar string
	Created   *time.Time
	Expires   *time.Time
	Statuses  []string
}

// queryWhois runs the two-step lookup: ask IANA which server owns the TLD,
// then ask that server about the domain.
func queryWhois(ctx context.Context, dialer *net.Dialer, domain string) (*whoisRecord, error) {
	tld := domain
	if i := strings.LastIndex(domain, "."); i >= 0 {
		tld = domain[i+1:]
	}

	root, err := whoisExchange(ctx, dialer, ianaWhois, tld)
	if err != nil {
		return nil, fmt.Errorf("iana lookup: %w", err)
	}

	server := referredServer(root)
	if server == "" {
		return nil, fmt.Errorf("no whois server published for .%s", tld)
	}

	body, err := whoisExchange(ctx, dialer, net.JoinHostPort(server, "43"), domain)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", server, err)
	}

	// Thin registries answer with a pointer to the registrar's own server,
	// which is the one holding the expiry date.
	if next := referredServer(body); next != "" && !strings.EqualFold(next, server) {
		if deeper, err := whoisExchange(ctx, dialer, net.JoinHostPort(next, "43"), domain); err == nil {
			if merged := parseWhois(deeper); merged.Expires != nil {
				return merged, nil
			}
		}
	}

	rec := parseWhois(body)
	if rec.Expires == nil && rec.Registrar == "" {
		return nil, fmt.Errorf("%s returned no usable record", server)
	}
	return rec, nil
}

// whoisExchange writes one query and reads the whole reply.
func whoisExchange(ctx context.Context, dialer *net.Dialer, addr, query string) (string, error) {
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if _, err := fmt.Fprintf(conn, "%s\r\n", query); err != nil {
		return "", err
	}

	var sb strings.Builder
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 32*1024), 512*1024)
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	if err := scanner.Err(); err != nil && sb.Len() == 0 {
		return "", err
	}
	return sb.String(), nil
}

// referredServer finds the "refer:" or "Registrar WHOIS Server:" pointer.
func referredServer(body string) string {
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch normalizeKey(key) {
		case "refer", "whois", "registrar whois server", "whois server":
			if v := strings.TrimSpace(value); v != "" {
				return strings.TrimSuffix(strings.ToLower(v), ".")
			}
		}
	}
	return ""
}

// parseWhois pulls registrar, dates and status out of a free-form response.
//
// WHOIS has no schema — every registry invents its own labels — so this matches
// on a set of known keys and ignores everything else.
func parseWhois(body string) *whoisRecord {
	rec := &whoisRecord{}
	seenStatus := make(map[string]bool)

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		switch normalizeKey(key) {
		case "registrar", "sponsoring registrar", "registrar name":
			if rec.Registrar == "" {
				rec.Registrar = value
			}

		case "registry expiry date", "expiry date", "expiration date",
			"registrar registration expiration date", "expires on", "expire date",
			"paid-till", "renewal date", "expires":
			if rec.Expires == nil {
				rec.Expires = parseWhoisDate(value)
			}

		case "creation date", "created", "created on", "registered on",
			"domain registration date", "registered":
			if rec.Created == nil {
				rec.Created = parseWhoisDate(value)
			}

		case "domain status", "status":
			status := value
			// Registries append the EPP documentation URL to each status.
			if i := strings.Index(status, " http"); i >= 0 {
				status = strings.TrimSpace(status[:i])
			}
			if status != "" && !seenStatus[status] && len(rec.Statuses) < 6 {
				seenStatus[status] = true
				rec.Statuses = append(rec.Statuses, status)
			}
		}
	}

	return rec
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// whoisDateLayouts covers the formats seen across gTLD and ccTLD registries.
var whoisDateLayouts = []string{
	time.RFC3339,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05 MST",
	"2006-01-02",
	"02-Jan-2006 15:04:05 MST",
	"02-Jan-2006",
	"2006.01.02 15:04:05",
	"2006.01.02",
	"02.01.2006",
	"01/02/2006",
	"Mon Jan 2 15:04:05 MST 2006",
}

// parseWhoisDate returns nil rather than a zero time when nothing matches, so
// callers can tell "unknown" apart from "expired in year 1".
func parseWhoisDate(raw string) *time.Time {
	value := strings.TrimSpace(raw)
	value = strings.TrimSuffix(value, " (UTC)")
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	if value == "" {
		return nil
	}

	for _, layout := range whoisDateLayouts {
		if t, err := time.Parse(layout, value); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}
