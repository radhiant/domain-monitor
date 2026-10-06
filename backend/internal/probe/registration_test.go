package probe

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseWhoisDateLayouts(t *testing.T) {
	// One entry per registry format actually seen in the wild; a nil result
	// here would silently blank the expiry column on the wall.
	cases := map[string]string{
		"2026-08-28T00:00:00Z":       "2026-08-28",
		"2026-08-28T07:00:00+0700":   "2026-08-28",
		"2026-08-28 00:00:00":        "2026-08-28",
		"2026-08-28":                 "2026-08-28",
		"28-Aug-2026":                "2026-08-28",
		"2026.08.28":                 "2026-08-28",
		"28.08.2026":                 "2026-08-28",
		"2026-08-28T00:00:00Z (UTC)": "2026-08-28",
	}

	for raw, want := range cases {
		got := parseWhoisDate(raw)
		if got == nil {
			t.Errorf("parseWhoisDate(%q) = nil", raw)
			continue
		}
		if got.Format("2006-01-02") != want {
			t.Errorf("parseWhoisDate(%q) = %s, want %s", raw, got.Format("2006-01-02"), want)
		}
	}

	for _, raw := range []string{"", "not a date", "soon"} {
		if got := parseWhoisDate(raw); got != nil {
			t.Errorf("parseWhoisDate(%q) = %v, want nil", raw, got)
		}
	}
}

func TestParseWhois(t *testing.T) {
	body := `% This is a comment banner
Domain Name: EXAMPLE.ORG
Registrar: Example Registrar
Creation Date: 2015-03-01T00:00:00Z
Expiry Date: 2026-11-23T00:00:00Z
Domain Status: clientTransferProhibited https://icann.org/epp#clientTransferProhibited
Domain Status: clientTransferProhibited https://icann.org/epp#clientTransferProhibited
Name Server: ns1.example.com
`

	rec := parseWhois(body)

	if rec.Registrar != "Example Registrar" {
		t.Errorf("registrar = %q", rec.Registrar)
	}
	if rec.Expires == nil || rec.Expires.Format("2006-01-02") != "2026-11-23" {
		t.Errorf("expires = %v", rec.Expires)
	}
	if rec.Created == nil || rec.Created.Format("2006-01-02") != "2015-03-01" {
		t.Errorf("created = %v", rec.Created)
	}
	// The duplicated line and the trailing EPP URL must both be cleaned up.
	if len(rec.Statuses) != 1 || rec.Statuses[0] != "clientTransferProhibited" {
		t.Errorf("statuses = %v", rec.Statuses)
	}
}

func TestReferredServer(t *testing.T) {
	if got := referredServer("refer:        whois.verisign-grs.com\n"); got != "whois.verisign-grs.com" {
		t.Errorf("refer = %q", got)
	}
	if got := referredServer("Registrar WHOIS Server: whois.registrar.test\n"); got != "whois.registrar.test" {
		t.Errorf("registrar refer = %q", got)
	}
	if got := referredServer("Domain Name: example.com\n"); got != "" {
		t.Errorf("expected no referral, got %q", got)
	}
}

func TestParseRDAP(t *testing.T) {
	raw := `{
	  "objectClassName": "domain",
	  "ldhName": "EXAMPLE.COM",
	  "status": ["client transfer prohibited"],
	  "events": [
	    {"eventAction": "registration", "eventDate": "2016-02-11T09:00:00Z"},
	    {"eventAction": "expiration",   "eventDate": "2026-02-11T09:00:00Z"},
	    {"eventAction": "last changed", "eventDate": "2025-01-04T09:00:00Z"}
	  ],
	  "entities": [
	    {
	      "roles": ["registrant"],
	      "vcardArray": ["vcard", [["version", {}, "text", "4.0"], ["fn", {}, "text", "Redacted"]]]
	    },
	    {
	      "roles": ["registrar"],
	      "vcardArray": ["vcard", [["version", {}, "text", "4.0"], ["fn", {}, "text", "NiagaHoster"]]]
	    }
	  ]
	}`

	var doc rdapDomain
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	rec := parseRDAP(&doc)

	if rec.Registrar != "NiagaHoster" {
		t.Errorf("registrar = %q, want the registrar entity not the registrant", rec.Registrar)
	}
	if rec.Expires == nil || rec.Expires.Format("2006-01-02") != "2026-02-11" {
		t.Errorf("expires = %v", rec.Expires)
	}
	if rec.Created == nil || rec.Created.Format("2006-01-02") != "2016-02-11" {
		t.Errorf("created = %v", rec.Created)
	}
}

func TestDaysUntilRoundsDown(t *testing.T) {
	// 47 hours out is one full day, not two.
	if got := daysUntil(time.Now().Add(47 * time.Hour)); got != 1 {
		t.Errorf("daysUntil(47h) = %d, want 1", got)
	}
	if got := daysUntil(time.Now().Add(-2 * time.Hour)); got != 0 {
		t.Errorf("daysUntil(past) = %d, want 0 or negative", got)
	}
}
