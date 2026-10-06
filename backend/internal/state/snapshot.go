package state

import (
	"sort"
	"sync"
	"time"

	"domain-monitor/backend/internal/model"
)

// maxEvents is how much ticker history is kept in memory. The wall shows the
// most recent handful; the rest lives in SQLite.
const maxEvents = 40

// entry is the live record for one endpoint.
//
// The probe result pointers are never mutated after they are stored: each probe
// allocates a fresh struct, so readers can share them without copying.
type entry struct {
	target   model.Target
	status   model.Status
	reason   string
	http     *model.HTTPResult
	cert     *model.CertInfo
	dns      *model.DNSInfo
	uptime   model.Uptime
	spark    []float64
	incident *model.Incident
}

// Snapshot is the authoritative in-memory view of every target.
//
// The API and the WebSocket hub both read from here rather than from SQLite:
// a wall display re-reading the overview every few seconds must not turn into
// a stream of database scans.
type Snapshot struct {
	mu sync.RWMutex

	th       Thresholds
	order    []string
	entries  map[string]*entry
	apexes   []string
	domains  map[string]*model.DomainInfo
	events   []model.Event
	lastScan time.Time
}

// New returns an empty snapshot using the given thresholds.
func New(th Thresholds) *Snapshot {
	return &Snapshot{
		th:      th,
		entries: make(map[string]*entry),
		domains: make(map[string]*model.DomainInfo),
	}
}

// Thresholds exposes the configured boundaries so handlers can echo them to
// the frontend, keeping both sides on one definition of degraded.
func (s *Snapshot) Thresholds() Thresholds {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.th
}

// SetTargets replaces the target list, preserving the live state of everything
// that survived the edit and dropping whatever left the file.
func (s *Snapshot) SetTargets(targets []model.Target, apexes []string) (added, removed []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]*entry, len(targets))
	order := make([]string, 0, len(targets))

	for _, t := range targets {
		order = append(order, t.ID)
		if existing, ok := s.entries[t.ID]; ok {
			existing.target = t
			next[t.ID] = existing
			continue
		}
		next[t.ID] = &entry{target: t, status: model.StatusUnknown}
		added = append(added, t.ID)
	}

	for id := range s.entries {
		if _, ok := next[id]; !ok {
			removed = append(removed, id)
		}
	}

	s.entries = next
	s.order = order
	s.apexes = append([]string(nil), apexes...)

	return added, removed
}

// Targets returns the current target list in file order.
func (s *Snapshot) TargetList() []model.Target {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]model.Target, 0, len(s.order))
	for _, id := range s.order {
		if e, ok := s.entries[id]; ok {
			out = append(out, e.target)
		}
	}
	return out
}

// Apexes returns the apex domains being tracked.
func (s *Snapshot) Apexes() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.apexes...)
}

// ApplyHTTP stores a reachability result and reclassifies the target.
func (s *Snapshot) ApplyHTTP(id string, res *model.HTTPResult) (model.Status, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return model.StatusUnknown, ""
	}

	e.http = res
	s.lastScan = time.Now().UTC()

	// The tile sparkline updates on every check rather than waiting for the
	// next rollup pass, so the wall always reflects the newest sample.
	e.spark = appendSpark(e.spark, res.LatencyMS, 24)

	e.status, e.reason = Classify(s.th, res, e.cert, smoothLatency(e.spark))

	return e.status, e.reason
}

// ApplyCert stores a certificate record and reclassifies the target, since an
// expired certificate changes the status on its own.
func (s *Snapshot) ApplyCert(id string, cert *model.CertInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[id]
	if !ok {
		return
	}
	e.cert = cert
	if e.http != nil {
		e.status, e.reason = Classify(s.th, e.http, e.cert, smoothLatency(e.spark))
	}
}

// ApplyDNS stores a resolution record.
func (s *Snapshot) ApplyDNS(id string, dns *model.DNSInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.entries[id]; ok {
		e.dns = dns
	}
}

// ApplyDomain stores a registration record for an apex domain.
func (s *Snapshot) ApplyDomain(info *model.DomainInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.domains[info.Apex] = info
}

// SetUptimes replaces the rolled-up availability figures.
func (s *Snapshot) SetUptimes(ups map[string]model.Uptime) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, u := range ups {
		if e, ok := s.entries[id]; ok {
			e.uptime = u
		}
	}
}

// SetSparks replaces the tile sparklines from stored history, used once at
// startup so a restart does not blank every chart.
func (s *Snapshot) SetSparks(sparks map[string][]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, values := range sparks {
		if e, ok := s.entries[id]; ok {
			e.spark = values
		}
	}
}

// SetIncidents attaches the currently open incidents to their targets.
func (s *Snapshot) SetIncidents(incidents []*model.Incident) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range s.entries {
		e.incident = nil
	}
	for _, inc := range incidents {
		if e, ok := s.entries[inc.TargetID]; ok {
			e.incident = inc
		}
	}
}

// PushEvent records a status transition for the ticker.
func (s *Snapshot) PushEvent(ev model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append([]model.Event{ev}, s.events...)
	if len(s.events) > maxEvents {
		s.events = s.events[:maxEvents]
	}
}

// SetEvents seeds the ticker from stored history at startup.
func (s *Snapshot) SetEvents(events []model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(events) > maxEvents {
		events = events[:maxEvents]
	}
	s.events = append([]model.Event(nil), events...)
}

// State returns the full record for one target.
func (s *Snapshot) State(id string) (*model.TargetState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	e, ok := s.entries[id]
	if !ok {
		return nil, false
	}
	return e.toModel(), true
}

// States returns every target in file order.
func (s *Snapshot) States() []*model.TargetState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.statesLocked()
}

func (s *Snapshot) statesLocked() []*model.TargetState {
	out := make([]*model.TargetState, 0, len(s.order))
	for _, id := range s.order {
		if e, ok := s.entries[id]; ok {
			out = append(out, e.toModel())
		}
	}
	return out
}

// Groups returns the targets grouped by apex domain, which is the shape the
// wall display renders directly.
func (s *Snapshot) Groups() []model.DomainGroup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.groupsLocked()
}

func (s *Snapshot) groupsLocked() []model.DomainGroup {
	index := make(map[string]int)
	groups := make([]model.DomainGroup, 0, len(s.apexes))

	for _, apex := range s.apexes {
		index[apex] = len(groups)
		groups = append(groups, model.DomainGroup{
			Apex:   apex,
			Status: model.StatusUnknown,
			Domain: s.domains[apex],
		})
	}

	for _, id := range s.order {
		e, ok := s.entries[id]
		if !ok {
			continue
		}

		pos, ok := index[e.target.Apex]
		if !ok {
			// A target whose apex never got its own "domain:" line still needs
			// somewhere to live rather than vanishing from the wall.
			pos = len(groups)
			index[e.target.Apex] = pos
			groups = append(groups, model.DomainGroup{
				Apex:   e.target.Apex,
				Status: model.StatusUnknown,
				Domain: s.domains[e.target.Apex],
			})
		}

		g := &groups[pos]
		st := e.toModel()
		g.Endpoints = append(g.Endpoints, st)
		g.Total++
		if st.Status == model.StatusUp {
			g.UpCount++
		}
		// A group is only as healthy as its worst endpoint.
		g.Status = model.WorseOf(g.Status, st.Status)
	}

	return groups
}

// Expiry returns the combined SSL and domain watchlist, soonest first.
func (s *Snapshot) Expiry() []model.ExpiryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.expiryLocked()
}

func (s *Snapshot) expiryLocked() []model.ExpiryItem {
	items := make([]model.ExpiryItem, 0, len(s.entries)+len(s.domains))

	for _, id := range s.order {
		e, ok := s.entries[id]
		if !ok || e.cert == nil || e.cert.NotAfter == nil || e.cert.DaysLeft == nil {
			continue
		}
		items = append(items, model.ExpiryItem{
			Kind:      "ssl",
			TargetID:  e.target.ID,
			Label:     e.target.Host,
			Apex:      e.target.Apex,
			ExpiresAt: *e.cert.NotAfter,
			DaysLeft:  *e.cert.DaysLeft,
			Issuer:    e.cert.Issuer,
		})
	}

	for _, apex := range s.apexes {
		info, ok := s.domains[apex]
		if !ok || info.ExpiresAt == nil || info.DaysLeft == nil {
			continue
		}
		items = append(items, model.ExpiryItem{
			Kind:      "domain",
			TargetID:  apex,
			Label:     apex,
			Apex:      apex,
			ExpiresAt: *info.ExpiresAt,
			DaysLeft:  *info.DaysLeft,
			Registrar: info.Registrar,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].DaysLeft != items[j].DaysLeft {
			return items[i].DaysLeft < items[j].DaysLeft
		}
		return items[i].Label < items[j].Label
	})

	return items
}

// Events returns the recent status transitions, newest first.
func (s *Snapshot) Events() []model.Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.Event(nil), s.events...)
}

// Overview builds the KPI payload the wall header renders.
func (s *Snapshot) Overview() model.Overview {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ov := model.Overview{
		GeneratedAt:    time.Now().UTC(),
		TotalDomains:   len(s.apexes),
		TotalEndpoints: len(s.order),
		Events:         append([]model.Event(nil), s.events...),
	}

	var uptimeSum, latencySum float64
	var uptimeCount, latencyCount int

	for _, id := range s.order {
		e, ok := s.entries[id]
		if !ok {
			continue
		}

		switch e.status {
		case model.StatusUp:
			ov.Up++
		case model.StatusDegraded:
			ov.Degraded++
		case model.StatusDown:
			ov.Down++
		default:
			ov.Unknown++
		}

		if e.uptime.Samples > 0 {
			uptimeSum += e.uptime.Day
			uptimeCount++
		}

		if e.http != nil && e.http.Up {
			latencySum += e.http.LatencyMS
			latencyCount++
			if e.http.LatencyMS > ov.SlowestMS {
				ov.SlowestMS = e.http.LatencyMS
				ov.SlowestID = e.target.ID
			}
		}

		if e.incident != nil {
			ov.ActiveIncident++
			ov.Incidents = append(ov.Incidents, e.incident)
		}
	}

	if uptimeCount > 0 {
		ov.UptimeDay = uptimeSum / float64(uptimeCount)
	}
	if latencyCount > 0 {
		ov.AvgLatencyMS = latencySum / float64(latencyCount)
	}

	if !s.lastScan.IsZero() {
		scan := s.lastScan
		ov.LastScan = &scan
	}

	expiry := s.expiryLocked()
	ov.Expiry = expiry
	for i := range expiry {
		if expiry[i].Kind == "ssl" && ov.NextSSL == nil {
			item := expiry[i]
			ov.NextSSL = &item
		}
		if expiry[i].Kind == "domain" && ov.NextDomain == nil {
			item := expiry[i]
			ov.NextDomain = &item
		}
	}

	sort.Slice(ov.Incidents, func(i, j int) bool {
		return ov.Incidents[i].StartedAt.Before(ov.Incidents[j].StartedAt)
	})

	return ov
}

// Full is the payload a WebSocket client receives on connect.
type Full struct {
	Overview model.Overview      `json:"overview"`
	Groups   []model.DomainGroup `json:"groups"`
}

// Full returns overview and groups together, so a newly connected display
// renders in one frame instead of stitching several requests together.
func (s *Snapshot) Full() Full {
	return Full{
		Overview: s.Overview(),
		Groups:   s.Groups(),
	}
}

func (e *entry) toModel() *model.TargetState {
	return &model.TargetState{
		Target:         e.target,
		Status:         e.status,
		Reason:         e.reason,
		HTTP:           e.http,
		Cert:           e.cert,
		DNS:            e.dns,
		Uptime:         e.uptime,
		Spark:          append([]float64(nil), e.spark...),
		ActiveIncident: e.incident,
	}
}

// smoothLatencyWindow is how many recent samples the latency verdict averages.
// Three is enough to absorb a single congested probe without hiding a host
// that has genuinely become slow, which shows up within a few minutes.
const smoothLatencyWindow = 3

// smoothLatency averages the tail of the sparkline.
func smoothLatency(spark []float64) float64 {
	if len(spark) == 0 {
		return 0
	}
	window := spark
	if len(window) > smoothLatencyWindow {
		window = window[len(window)-smoothLatencyWindow:]
	}

	sum := 0.0
	for _, v := range window {
		sum += v
	}
	return sum / float64(len(window))
}

// appendSpark keeps a bounded, oldest-first series.
func appendSpark(current []float64, value float64, limit int) []float64 {
	out := append(current, value)
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}
