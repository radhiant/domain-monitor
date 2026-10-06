// Package target turns the human-maintained list-domain.txt into the probe
// target set.
//
// The file stays the source of truth on purpose: the operator already edits it,
// and a second machine-readable copy would drift out of sync the first time
// someone adds a subdomain in a hurry.
package target

import (
	"bufio"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"domain-monitor/backend/internal/model"
)

// Set is one parsed generation of list-domain.txt.
type Set struct {
	Targets []model.Target
	Apexes  []string
}

// ByID indexes the set for lookups by target id.
func (s *Set) ByID() map[string]model.Target {
	out := make(map[string]model.Target, len(s.Targets))
	for _, t := range s.Targets {
		out[t.ID] = t
	}
	return out
}

// Parse reads the "domain:" / "subdomain:" format.
//
// A "domain:" line opens a group; the following "subdomain:" line fills it with
// a comma-separated list. Entries may or may not carry a scheme, and the apex
// itself is monitored as an endpoint in addition to being the WHOIS target.
func Parse(r io.Reader) (*Set, error) {
	set := &Set{}
	seen := make(map[string]bool)
	currentApex := ""

	add := func(raw string) {
		host, url := normalize(raw)
		if host == "" || seen[host] {
			return
		}
		seen[host] = true

		apex := currentApex
		if apex == "" {
			apex = host
		}

		set.Targets = append(set.Targets, model.Target{
			ID:     host,
			Apex:   apex,
			Host:   host,
			URL:    url,
			Label:  labelOf(host, apex),
			IsApex: host == apex,
			Order:  len(set.Targets),
		})
	}

	scanner := bufio.NewScanner(r)
	// Subdomain lines get long; the 64KB default token size is not guaranteed.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, ":")
		if !found {
			// A bare host on its own line is treated as a subdomain of the
			// group currently open, so a hand-edited file still parses.
			add(line)
			continue
		}

		switch strings.ToLower(strings.TrimSpace(key)) {
		case "domain":
			host, _ := normalize(value)
			if host == "" {
				continue
			}
			currentApex = host
			if !containsString(set.Apexes, host) {
				set.Apexes = append(set.Apexes, host)
			}
			add(value)

		case "subdomain", "subdomains":
			for _, part := range strings.Split(value, ",") {
				add(part)
			}

		default:
			// "https" from a bare URL line survives the Cut above; put it back.
			add(line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return set, nil
}

// LoadFile parses list-domain.txt from disk.
func LoadFile(path string) (*Set, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// normalize turns "https://mail.example.org/", " app.example.com " or
// "http://x.test:8080" into a bare hostname plus the URL to probe.
func normalize(raw string) (host, url string) {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, ",;")
	if s == "" {
		return "", ""
	}

	scheme := "https"
	if i := strings.Index(s, "://"); i >= 0 {
		scheme = strings.ToLower(s[:i])
		s = s[i+3:]
		if scheme != "http" && scheme != "https" {
			scheme = "https"
		}
	}

	// Drop path, query and credentials; keep an explicit port.
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	if s == "" || !strings.Contains(s, ".") {
		return "", ""
	}

	return s, scheme + "://" + s
}

// labelOf is the display name on a wall tile: the part in front of the apex.
func labelOf(host, apex string) string {
	if host == apex {
		return "@"
	}
	if suffix := "." + apex; strings.HasSuffix(host, suffix) {
		return strings.TrimSuffix(host, suffix)
	}
	return host
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Loader watches list-domain.txt and re-parses it when it changes, so adding a
// subdomain never requires restarting the agent.
type Loader struct {
	path    string
	modTime time.Time
	size    int64
}

// NewLoader returns a loader for the given file path.
func NewLoader(path string) *Loader {
	return &Loader{path: path}
}

// Path is the file being watched.
func (l *Loader) Path() string { return l.path }

// Load parses the file and records its stamp as current.
func (l *Loader) Load() (*Set, error) {
	set, err := LoadFile(l.path)
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Stat(l.path); statErr == nil {
		l.modTime = info.ModTime()
		l.size = info.Size()
	}
	return set, nil
}

// Changed reports whether the file has been modified since the last Load.
// Size is compared too: an edit that keeps the same second-resolution mtime is
// otherwise invisible.
func (l *Loader) Changed() bool {
	info, err := os.Stat(l.path)
	if err != nil {
		return false
	}
	return !info.ModTime().Equal(l.modTime) || info.Size() != l.size
}

// SortedApexes returns the apex list in stable alphabetical order.
func (s *Set) SortedApexes() []string {
	out := append([]string(nil), s.Apexes...)
	sort.Strings(out)
	return out
}
