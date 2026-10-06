package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"domain-monitor/backend/internal/model"
)

// rdapBootstrap redirects to whichever registry actually serves a TLD, so the
// agent does not need to know that .com lives at Verisign.
const rdapBootstrap = "https://rdap.org/domain/"

// RegistrationProber reports when an apex domain's registration expires.
//
// RDAP is tried first because it is structured and unambiguous. Plenty of
// ccTLDs still have no RDAP endpoint, so WHOIS on port 43 remains the fallback
// rather than an afterthought.
type RegistrationProber struct {
	client *http.Client
	dialer *net.Dialer
}

// NewRegistrationProber builds a prober with the given per-lookup timeout.
func NewRegistrationProber(timeout time.Duration) *RegistrationProber {
	return &RegistrationProber{
		client: &http.Client{Timeout: timeout},
		dialer: &net.Dialer{Timeout: timeout},
	}
}

// Probe looks up one apex domain.
func (p *RegistrationProber) Probe(ctx context.Context, apex string) *model.DomainInfo {
	info := &model.DomainInfo{Apex: apex, CheckedAt: time.Now().UTC()}

	rec, err := p.queryRDAP(ctx, apex)
	source := "rdap"
	if err != nil {
		whoisRec, whoisErr := queryWhois(ctx, p.dialer, apex)
		if whoisErr != nil {
			info.Error = fmt.Sprintf("rdap: %v; whois: %v", err, whoisErr)
			return info
		}
		rec, source = whoisRec, "whois"
	}

	info.Source = source
	info.Registrar = rec.Registrar
	info.CreatedAt = rec.Created
	info.ExpiresAt = rec.Expires
	info.Statuses = rec.Statuses

	if rec.Expires != nil {
		days := daysUntil(*rec.Expires)
		info.DaysLeft = &days
	}

	return info
}

// rdapDomain is the slice of the RDAP domain object we care about.
type rdapDomain struct {
	LdhName string   `json:"ldhName"`
	Status  []string `json:"status"`
	Events  []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	Entities []rdapEntity `json:"entities"`
}

type rdapEntity struct {
	Roles      []string          `json:"roles"`
	VCardArray []json.RawMessage `json:"vcardArray"`
	Entities   []rdapEntity      `json:"entities"`
}

func (p *RegistrationProber) queryRDAP(ctx context.Context, apex string) (*whoisRecord, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rdapBootstrap+apex, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rdap+json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var doc rdapDomain
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}

	rec := parseRDAP(&doc)
	if rec.Expires == nil && rec.Registrar == "" {
		return nil, fmt.Errorf("response carried no expiry or registrar")
	}
	return rec, nil
}

// parseRDAP maps the RDAP document onto the same shape the WHOIS parser
// produces, so the rest of the agent does not care which source answered.
func parseRDAP(doc *rdapDomain) *whoisRecord {
	rec := &whoisRecord{}

	for _, ev := range doc.Events {
		date := parseWhoisDate(ev.Date)
		if date == nil {
			continue
		}
		switch strings.ToLower(ev.Action) {
		case "expiration":
			rec.Expires = date
		case "registration":
			rec.Created = date
		}
	}

	rec.Registrar = findRegistrar(doc.Entities)

	for _, s := range doc.Status {
		if len(rec.Statuses) < 6 {
			rec.Statuses = append(rec.Statuses, s)
		}
	}

	return rec
}

// findRegistrar walks the entity tree for the one holding the registrar role.
func findRegistrar(entities []rdapEntity) string {
	for _, ent := range entities {
		for _, role := range ent.Roles {
			if strings.EqualFold(role, "registrar") {
				if name := vcardName(ent.VCardArray); name != "" {
					return name
				}
			}
		}
		if name := findRegistrar(ent.Entities); name != "" {
			return name
		}
	}
	return ""
}

// vcardName digs the "fn" (formatted name) property out of a jCard, which is
// encoded as ["vcard", [["fn", {}, "text", "Some Registrar"], ...]].
func vcardName(vcard []json.RawMessage) string {
	if len(vcard) < 2 {
		return ""
	}

	var props [][]json.RawMessage
	if err := json.Unmarshal(vcard[1], &props); err != nil {
		return ""
	}

	for _, prop := range props {
		if len(prop) < 4 {
			continue
		}
		var name string
		if err := json.Unmarshal(prop[0], &name); err != nil || !strings.EqualFold(name, "fn") {
			continue
		}
		var value string
		if err := json.Unmarshal(prop[3], &value); err == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
