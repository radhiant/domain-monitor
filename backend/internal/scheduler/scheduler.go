// Package scheduler drives every probe, folds the results into the snapshot
// and persists them.
//
// One agent covers every domain and subdomain, so the work here is deciding
// what is due, spreading it out, and running it with a bounded amount of
// concurrency.
package scheduler

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"domain-monitor/backend/internal/config"
	"domain-monitor/backend/internal/incident"
	"domain-monitor/backend/internal/model"
	"domain-monitor/backend/internal/notify"
	"domain-monitor/backend/internal/probe"
	"domain-monitor/backend/internal/state"
	"domain-monitor/backend/internal/store"
	"domain-monitor/backend/internal/target"
	"domain-monitor/backend/internal/websocket"
)

// tick is how often the due table is scanned. It only has to be finer than the
// shortest probe interval, and a one-second sweep over a few dozen entries is
// free.
const tick = time.Second

// firstRunSpread is how long the very first pass is spread over. Long enough
// not to open 28 connections at once, short enough that the wall fills in
// while someone is still looking at it.
const firstRunSpread = 20 * time.Second

// kind identifies which probe a scheduled job runs.
type kind int

const (
	kindHTTP kind = iota
	kindTLS
	kindDNS
	kindRegistration
)

// job is one unit of work handed to a worker.
type job struct {
	kind   kind
	target model.Target
	apex   string
}

// result is what a worker produces, funnelled back to a single goroutine so
// all database writes and snapshot mutations happen in one place.
type result struct {
	job    job
	http   *model.HTTPResult
	cert   *model.CertInfo
	dns    *model.DNSInfo
	domain *model.DomainInfo
}

// schedule tracks when each probe is next due for one target.
type schedule struct {
	target   model.Target
	nextHTTP time.Time
	nextTLS  time.Time
	nextDNS  time.Time
}

// Scheduler owns the probe loop.
type Scheduler struct {
	cfg      *config.Config
	snap     *state.Snapshot
	store    *store.Store
	tracker  *incident.Tracker
	hub      *websocket.Hub
	loader   *target.Loader
	notifier notify.Notifier

	httpProbe *probe.HTTPProber
	tlsProbe  *probe.TLSProber
	dnsProbe  *probe.DNSProber
	regProbe  *probe.RegistrationProber

	mu       sync.Mutex
	due      map[string]*schedule
	apexDue  map[string]time.Time
	warnedAt map[string]time.Time

	jobs    chan job
	results chan result
	rng     *rand.Rand
}

// New wires a scheduler together.
func New(
	cfg *config.Config,
	snap *state.Snapshot,
	st *store.Store,
	tracker *incident.Tracker,
	hub *websocket.Hub,
	loader *target.Loader,
	notifier notify.Notifier,
) *Scheduler {
	return &Scheduler{
		cfg:      cfg,
		snap:     snap,
		store:    st,
		tracker:  tracker,
		hub:      hub,
		loader:   loader,
		notifier: notifier,

		httpProbe: probe.NewHTTPProber(cfg.HTTPTimeout, cfg.UserAgent),
		tlsProbe:  probe.NewTLSProber(cfg.TLSTimeout),
		dnsProbe:  probe.NewDNSProber(cfg.DNSTimeout),
		regProbe:  probe.NewRegistrationProber(cfg.RDAPTimeout),

		due:      make(map[string]*schedule),
		apexDue:  make(map[string]time.Time),
		warnedAt: make(map[string]time.Time),

		jobs:    make(chan job, 256),
		results: make(chan result, 256),
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// WarmStart repopulates the snapshot from SQLite so a restart shows history
// immediately instead of an empty wall.
func (s *Scheduler) WarmStart() {
	if certs, err := s.store.LoadCerts(); err == nil {
		for id, cert := range certs {
			s.snap.ApplyCert(id, cert)
		}
	} else {
		log.Warn().Err(err).Msg("could not restore certificate records")
	}

	if dns, err := s.store.LoadDNS(); err == nil {
		for id, rec := range dns {
			s.snap.ApplyDNS(id, rec)
		}
	}

	if domains, err := s.store.LoadDomains(); err == nil {
		for _, info := range domains {
			s.snap.ApplyDomain(info)
		}
	}

	if ups, err := s.store.Uptimes(); err == nil {
		s.snap.SetUptimes(ups)
	}

	if sparks, err := s.store.Sparks(s.cfg.SparkPoints); err == nil {
		s.snap.SetSparks(sparks)
	}

	if events, err := s.store.RecentEvents(40); err == nil {
		s.snap.SetEvents(events)
	}

	// Outages that were open at shutdown continue rather than being reopened
	// as duplicates the first time the endpoint fails again.
	if active, err := s.store.ActiveIncidents(); err == nil {
		for _, inc := range active {
			s.tracker.Restore(inc.TargetID, inc.ID)
		}
		s.snap.SetIncidents(active)
	}
}

// Run blocks until the context is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	var wg sync.WaitGroup

	for i := 0; i < s.cfg.ProbeConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.worker(ctx)
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.collect(ctx)
	}()

	sweep := time.NewTicker(tick)
	rollup := time.NewTicker(time.Minute)
	reload := time.NewTicker(s.cfg.ReloadInterval)
	prune := time.NewTicker(time.Hour)
	push := time.NewTicker(5 * time.Second)

	defer func() {
		sweep.Stop()
		rollup.Stop()
		reload.Stop()
		prune.Stop()
		push.Stop()
	}()

	for {
		select {
		case <-ctx.Done():
			close(s.jobs)
			wg.Wait()
			return

		case <-sweep.C:
			s.dispatchDue()

		case <-rollup.C:
			s.refreshRollups()

		case <-reload.C:
			if s.loader.Changed() {
				s.reloadTargets()
			}

		case <-prune.C:
			if err := s.store.Prune(s.cfg.RetentionDays); err != nil {
				log.Warn().Err(err).Msg("prune failed")
			}

		case <-push.C:
			// The KPI band carries derived figures (uptime average, incident
			// count) that no single probe result would update on its own.
			s.hub.Broadcast(model.WSMessage{
				Type:      "overview",
				Timestamp: time.Now().Unix(),
				Data:      s.snap.Overview(),
			})
		}
	}
}

// SetTargets installs a target list and schedules its first pass.
func (s *Scheduler) SetTargets(set *target.Set) {
	added, removed := s.snap.SetTargets(set.Targets, set.Apexes)

	if err := s.store.SyncTargets(set.Targets); err != nil {
		log.Error().Err(err).Msg("could not persist target list")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	next := make(map[string]*schedule, len(set.Targets))

	for _, t := range set.Targets {
		if existing, ok := s.due[t.ID]; ok {
			existing.target = t
			next[t.ID] = existing
			continue
		}
		// New targets are spread across the first-run window rather than all
		// firing on the same second.
		offset := time.Duration(s.rng.Int63n(int64(firstRunSpread)))
		next[t.ID] = &schedule{
			target:   t,
			nextHTTP: now.Add(offset),
			nextTLS:  now.Add(offset),
			nextDNS:  now.Add(offset + 2*time.Second),
		}
	}
	s.due = next

	apexDue := make(map[string]time.Time, len(set.Apexes))
	for _, apex := range set.Apexes {
		if at, ok := s.apexDue[apex]; ok {
			apexDue[apex] = at
			continue
		}
		// Registration data changes on the scale of months. If the database
		// already holds a recent lookup, honour it instead of hitting the
		// registry again on every restart.
		if last, ok := s.store.LastDomainCheck(apex); ok {
			apexDue[apex] = last.Add(s.cfg.RDAPInterval)
		} else {
			apexDue[apex] = now.Add(time.Duration(s.rng.Int63n(int64(30 * time.Second))))
		}
	}
	s.apexDue = apexDue

	for _, id := range removed {
		s.tracker.Forget(id)
	}

	log.Info().
		Int("targets", len(set.Targets)).
		Int("domains", len(set.Apexes)).
		Int("added", len(added)).
		Int("removed", len(removed)).
		Msg("target list installed")
}

// reloadTargets re-reads list-domain.txt after it changed on disk.
func (s *Scheduler) reloadTargets() {
	set, err := s.loader.Load()
	if err != nil {
		log.Error().Err(err).Str("file", s.loader.Path()).Msg("reload failed, keeping the previous list")
		return
	}

	log.Info().Str("file", s.loader.Path()).Msg("target file changed, reloading")
	s.SetTargets(set)

	s.hub.Broadcast(model.WSMessage{
		Type:      "snapshot",
		Timestamp: time.Now().Unix(),
		Data:      s.snap.Full(),
	})
}

// RecheckAll brings every HTTP check forward.
//
// The sweep is still spread over a few seconds: an operator pressing "recheck"
// should not make the agent open every connection at once and produce a
// latency spike that looks like a real problem.
func (s *Scheduler) RecheckAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, sched := range s.due {
		sched.nextHTTP = now.Add(time.Duration(s.rng.Int63n(int64(5 * time.Second))))
	}
	log.Info().Int("targets", len(s.due)).Msg("manual recheck requested")
}

// dispatchDue queues everything whose time has come.
func (s *Scheduler) dispatchDue() {
	now := time.Now()

	s.mu.Lock()
	var queued []job

	for _, sched := range s.due {
		if now.After(sched.nextHTTP) {
			sched.nextHTTP = now.Add(s.cfg.HTTPInterval)
			queued = append(queued, job{kind: kindHTTP, target: sched.target})
		}
		if now.After(sched.nextTLS) {
			sched.nextTLS = now.Add(s.cfg.TLSInterval)
			queued = append(queued, job{kind: kindTLS, target: sched.target})
		}
		if now.After(sched.nextDNS) {
			sched.nextDNS = now.Add(s.cfg.DNSInterval)
			queued = append(queued, job{kind: kindDNS, target: sched.target})
		}
	}

	for apex, at := range s.apexDue {
		if now.After(at) {
			s.apexDue[apex] = now.Add(s.cfg.RDAPInterval)
			queued = append(queued, job{kind: kindRegistration, apex: apex})
		}
	}
	s.mu.Unlock()

	for _, j := range queued {
		select {
		case s.jobs <- j:
		default:
			// The queue is saturated, which means probes are running slower
			// than they are scheduled. Dropping this round is correct: the
			// next sweep will pick the target up again.
			log.Warn().Msg("probe queue full, skipping a scheduled check")
		}
	}
}

// worker runs jobs until the queue closes.
func (s *Scheduler) worker(ctx context.Context) {
	for j := range s.jobs {
		if ctx.Err() != nil {
			return
		}
		s.run(ctx, j)
	}
}

func (s *Scheduler) run(ctx context.Context, j job) {
	res := result{job: j}

	switch j.kind {
	case kindHTTP:
		probeCtx, cancel := context.WithTimeout(ctx, s.cfg.HTTPTimeout)
		res.http = s.httpProbe.Probe(probeCtx, j.target)
		cancel()

	case kindTLS:
		probeCtx, cancel := context.WithTimeout(ctx, s.cfg.TLSTimeout)
		res.cert = s.tlsProbe.Probe(probeCtx, j.target)
		cancel()
		if res.cert == nil {
			return // plain HTTP target, nothing to record
		}

	case kindDNS:
		probeCtx, cancel := context.WithTimeout(ctx, s.cfg.DNSTimeout)
		res.dns = s.dnsProbe.Probe(probeCtx, j.target)
		cancel()

	case kindRegistration:
		probeCtx, cancel := context.WithTimeout(ctx, s.cfg.RDAPTimeout)
		res.domain = s.regProbe.Probe(probeCtx, j.apex)
		cancel()
	}

	select {
	case s.results <- res:
	case <-ctx.Done():
	}
}

// collect applies every result. Running as a single goroutine keeps the
// SQLite writes serialised and the incident bookkeeping race-free.
func (s *Scheduler) collect(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case res := <-s.results:
			s.apply(res)
		}
	}
}

func (s *Scheduler) apply(res result) {
	switch {
	case res.http != nil:
		s.applyHTTP(res.job.target, res.http)

	case res.cert != nil:
		s.snap.ApplyCert(res.job.target.ID, res.cert)
		if err := s.store.SaveCert(res.job.target.ID, res.cert); err != nil {
			log.Warn().Err(err).Str("target", res.job.target.ID).Msg("could not store certificate")
		}
		s.checkExpiryWarning(res.job.target, res.cert)
		s.pushTarget(res.job.target.ID)

	case res.dns != nil:
		s.snap.ApplyDNS(res.job.target.ID, res.dns)
		if err := s.store.SaveDNS(res.job.target.ID, res.dns); err != nil {
			log.Warn().Err(err).Str("target", res.job.target.ID).Msg("could not store dns record")
		}
		s.pushTarget(res.job.target.ID)

	case res.domain != nil:
		s.snap.ApplyDomain(res.domain)
		if err := s.store.SaveDomain(res.domain); err != nil {
			log.Warn().Err(err).Str("apex", res.domain.Apex).Msg("could not store domain record")
		}
		if res.domain.Error != "" {
			log.Warn().Str("apex", res.domain.Apex).Str("error", res.domain.Error).Msg("registration lookup failed")
		} else {
			log.Info().
				Str("apex", res.domain.Apex).
				Str("source", res.domain.Source).
				Str("registrar", res.domain.Registrar).
				Msg("registration record updated")
		}
	}
}

// applyHTTP is the hot path: classify, persist, run the incident state machine
// and push the change to every display.
func (s *Scheduler) applyHTTP(t model.Target, res *model.HTTPResult) {
	status, reason := s.snap.ApplyHTTP(t.ID, res)

	if err := s.store.InsertCheck(t.ID, status, res); err != nil {
		log.Warn().Err(err).Str("target", t.ID).Msg("could not store check")
	}

	obs := s.tracker.Observe(t.ID, status, res.CheckedAt, reason, res.Error)

	// A transition out of UNKNOWN is just the agent starting up; putting that
	// in the ticker would bury the real events behind 28 lines of noise.
	if obs.Changed && obs.From != model.StatusUnknown {
		ev := model.Event{
			At:       res.CheckedAt,
			TargetID: t.ID,
			From:     obs.From,
			To:       status,
			Detail:   reason,
		}
		s.snap.PushEvent(ev)
		if err := s.store.InsertEvent(ev); err != nil {
			log.Warn().Err(err).Msg("could not store event")
		}
		s.hub.Broadcast(model.WSMessage{
			Type:      "event",
			Timestamp: time.Now().Unix(),
			Data:      ev,
		})
	}

	switch obs.Action.Type {
	case incident.ActionOpen:
		id, err := s.store.OpenIncident(t.ID, obs.Action.Reason, obs.Action.LastError, obs.Action.At)
		if err != nil {
			log.Error().Err(err).Str("target", t.ID).Msg("could not open incident")
			break
		}
		s.tracker.SetIncidentID(t.ID, id)
		log.Warn().Str("target", t.ID).Str("reason", obs.Action.Reason).Msg("incident opened")
		s.refreshIncidents(t, id, true)

	case incident.ActionClose:
		if err := s.store.CloseIncident(obs.Action.IncidentID, obs.Action.At); err != nil {
			log.Error().Err(err).Str("target", t.ID).Msg("could not close incident")
		}
		log.Info().Str("target", t.ID).Msg("incident resolved")
		s.refreshIncidents(t, obs.Action.IncidentID, false)
	}

	s.pushTarget(t.ID)
}

// refreshIncidents re-reads the open incidents and notifies the hook.
func (s *Scheduler) refreshIncidents(t model.Target, incidentID int64, opened bool) {
	active, err := s.store.ActiveIncidents()
	if err != nil {
		log.Warn().Err(err).Msg("could not refresh active incidents")
		return
	}
	s.snap.SetIncidents(active)

	var subject *model.Incident
	for _, inc := range active {
		if inc.ID == incidentID {
			subject = inc
			break
		}
	}
	if subject == nil {
		subject = &model.Incident{ID: incidentID, TargetID: t.ID}
	}

	if opened {
		s.notifier.IncidentOpened(subject, t)
	} else {
		s.notifier.IncidentClosed(subject, t)
	}
}

// checkExpiryWarning fires the hook at most once a day per certificate, so a
// six-hourly probe does not turn one expiring cert into four alerts a day.
func (s *Scheduler) checkExpiryWarning(t model.Target, cert *model.CertInfo) {
	if cert.DaysLeft == nil || *cert.DaysLeft > s.cfg.CertWarnDays {
		return
	}

	s.mu.Lock()
	last, seen := s.warnedAt[t.ID]
	if seen && time.Since(last) < 24*time.Hour {
		s.mu.Unlock()
		return
	}
	s.warnedAt[t.ID] = time.Now()
	s.mu.Unlock()

	log.Warn().Str("target", t.ID).Int("days_left", *cert.DaysLeft).Msg("certificate expiring soon")

	if cert.NotAfter != nil {
		s.notifier.ExpiryWarning(model.ExpiryItem{
			Kind:      "ssl",
			TargetID:  t.ID,
			Label:     t.Host,
			Apex:      t.Apex,
			ExpiresAt: *cert.NotAfter,
			DaysLeft:  *cert.DaysLeft,
			Issuer:    cert.Issuer,
		})
	}
}

// refreshRollups recomputes the availability figures the KPI band shows.
func (s *Scheduler) refreshRollups() {
	if ups, err := s.store.Uptimes(); err == nil {
		s.snap.SetUptimes(ups)
	} else {
		log.Warn().Err(err).Msg("uptime rollup failed")
	}

	if active, err := s.store.ActiveIncidents(); err == nil {
		s.snap.SetIncidents(active)
	}
}

// pushTarget sends one changed endpoint to every display.
func (s *Scheduler) pushTarget(id string) {
	st, ok := s.snap.State(id)
	if !ok {
		return
	}
	s.hub.Broadcast(model.WSMessage{
		Type:      "update",
		Timestamp: time.Now().Unix(),
		Data:      st,
	})
}
