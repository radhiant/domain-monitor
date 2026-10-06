package target

import (
	"strings"
	"testing"
)

// The exact file the operator maintains. If this ever stops parsing, the whole
// dashboard silently monitors nothing, so it is pinned as a test fixture.
const sample = `domain: example.com
subdomain: app.example.com, https://zabbix.example.com, https://ops.example.com

domain: example.org
subdomain: https://mail.example.org, https://cdn.example.org
`

func TestParseSample(t *testing.T) {
	set, err := Parse(strings.NewReader(sample))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if len(set.Apexes) != 2 {
		t.Fatalf("apexes = %v, want 2", set.Apexes)
	}
	// The apex is monitored as an endpoint too: 2 apexes + 5 subdomains.
	if len(set.Targets) != 7 {
		t.Fatalf("targets = %d, want 7", len(set.Targets))
	}

	byID := set.ByID()

	apex, ok := byID["example.com"]
	if !ok {
		t.Fatal("apex example.com is not a target")
	}
	if !apex.IsApex || apex.Label != "@" || apex.URL != "https://example.com" {
		t.Fatalf("apex target = %+v", apex)
	}

	// A bare host must get https:// even though the file omits the scheme.
	app, ok := byID["app.example.com"]
	if !ok {
		t.Fatal("app.example.com missing")
	}
	if app.URL != "https://app.example.com" {
		t.Fatalf("app URL = %q", app.URL)
	}
	if app.Label != "app" || app.Apex != "example.com" || app.IsApex {
		t.Fatalf("app target = %+v", app)
	}

	// Grouping must follow the most recent "domain:" line.
	if got := byID["mail.example.org"].Apex; got != "example.org" {
		t.Fatalf("mail.example.org apex = %q, want example.org", got)
	}

	// Order is preserved so the wall renders the file's own layout.
	for i, tgt := range set.Targets {
		if tgt.Order != i {
			t.Fatalf("target %d has order %d", i, tgt.Order)
		}
	}
}

func TestParseSkipsNoiseAndDuplicates(t *testing.T) {
	in := `# a comment
domain: example.com
subdomain: https://a.example.com, a.example.com, , https://a.example.com/some/path?q=1
subdomain: HTTPS://B.EXAMPLE.COM.
`
	set, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// example.com + a.example.com + b.example.com; the repeats collapse.
	if len(set.Targets) != 3 {
		var got []string
		for _, tgt := range set.Targets {
			got = append(got, tgt.Host)
		}
		t.Fatalf("targets = %v, want 3 unique", got)
	}

	byID := set.ByID()
	if _, ok := byID["b.example.com"]; !ok {
		t.Fatal("uppercase host with trailing dot was not normalized")
	}
	if got := byID["a.example.com"].URL; got != "https://a.example.com" {
		t.Fatalf("path was not stripped: %q", got)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string
		wantURL  string
	}{
		{"app.example.com", "app.example.com", "https://app.example.com"},
		{" https://mail.example.org ", "mail.example.org", "https://mail.example.org"},
		{"http://legacy.example.com", "legacy.example.com", "http://legacy.example.com"},
		{"https://api.example.com:8443/v1", "api.example.com:8443", "https://api.example.com:8443"},
		{"ftp://odd.example.com", "odd.example.com", "https://odd.example.com"},
		{"localhost", "", ""},
		{"", "", ""},
	}

	for _, tc := range cases {
		host, url := normalize(tc.in)
		if host != tc.wantHost || url != tc.wantURL {
			t.Errorf("normalize(%q) = (%q, %q), want (%q, %q)", tc.in, host, url, tc.wantHost, tc.wantURL)
		}
	}
}

func TestLabelOfUnrelatedHost(t *testing.T) {
	// A host that does not sit under the open apex keeps its full name rather
	// than rendering as a confusing empty tile.
	if got := labelOf("other.test", "example.com"); got != "other.test" {
		t.Fatalf("labelOf = %q", got)
	}
}
