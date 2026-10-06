package store

import (
	"database/sql"
	"time"

	"domain-monitor/backend/internal/model"
)

// HistoryPoint is one aggregated bucket on the detail-view chart.
type HistoryPoint struct {
	TS      int64   `json:"ts"`
	AvgMS   float64 `json:"avg_ms"`
	MaxMS   float64 `json:"max_ms"`
	Up      int     `json:"up"`
	Total   int     `json:"total"`
	Percent float64 `json:"percent"`
}

// Uptimes rolls availability up over 24 hours, 7 days and 30 days for every
// target in three queries.
//
// This is recomputed on a timer into an in-memory cache rather than per HTTP
// request: the wall display re-reads the overview constantly, and a full-table
// scan per viewer would be pure waste.
func (s *Store) Uptimes() (map[string]model.Uptime, error) {
	out := make(map[string]model.Uptime)
	now := time.Now()

	windows := []struct {
		since  time.Time
		assign func(u *model.Uptime, pct float64)
	}{
		{now.Add(-24 * time.Hour), func(u *model.Uptime, pct float64) { u.Day = pct }},
		{now.AddDate(0, 0, -7), func(u *model.Uptime, pct float64) { u.Week = pct }},
		{now.AddDate(0, 0, -30), func(u *model.Uptime, pct float64) { u.Month = pct }},
	}

	for _, w := range windows {
		rows, err := s.db.Query(`
            SELECT target_id,
                   100.0 * SUM(CASE WHEN up = 1 THEN 1 ELSE 0 END) / COUNT(*) AS pct,
                   COUNT(*) AS samples
            FROM checks
            WHERE ts >= ?
            GROUP BY target_id`, w.since.Unix())
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var id string
			var pct float64
			var samples int
			if err := rows.Scan(&id, &pct, &samples); err != nil {
				rows.Close()
				return nil, err
			}
			u := out[id]
			w.assign(&u, pct)
			// Sample count reflects the widest window scanned so far.
			if samples > u.Samples {
				u.Samples = samples
			}
			out[id] = u
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}

	if err := s.fillLatency(out, now.Add(-24*time.Hour)); err != nil {
		return nil, err
	}

	return out, nil
}

// fillLatency adds the mean and 95th percentile response time over a window.
//
// The percentile is computed in SQL with window functions so the raw samples
// never leave the database; pulling 24 hours of rows into Go once a minute
// would cost far more than the query does.
func (s *Store) fillLatency(out map[string]model.Uptime, since time.Time) error {
	rows, err := s.db.Query(`
        SELECT target_id, AVG(latency_ms)
        FROM checks
        WHERE ts >= ? AND up = 1
        GROUP BY target_id`, since.Unix())
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var avg float64
		if err := rows.Scan(&id, &avg); err != nil {
			rows.Close()
			return err
		}
		u := out[id]
		u.AvgMS = avg
		out[id] = u
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	p95, err := s.db.Query(`
        SELECT target_id, latency_ms FROM (
            SELECT target_id,
                   latency_ms,
                   ROW_NUMBER() OVER (PARTITION BY target_id ORDER BY latency_ms) AS rn,
                   COUNT(*)     OVER (PARTITION BY target_id)                     AS cnt
            FROM checks
            WHERE ts >= ? AND up = 1
        )
        WHERE rn = MAX(1, CAST(cnt * 0.95 AS INTEGER))`, since.Unix())
	if err != nil {
		return err
	}
	defer p95.Close()

	for p95.Next() {
		var id string
		var value float64
		if err := p95.Scan(&id, &value); err != nil {
			return err
		}
		u := out[id]
		u.P95MS = value
		out[id] = u
	}
	return p95.Err()
}

// Sparks returns the last n latencies per target, oldest first, for the inline
// sparkline on each wall tile.
func (s *Store) Sparks(n int) (map[string][]float64, error) {
	rows, err := s.db.Query(`
        SELECT target_id, latency_ms FROM (
            SELECT target_id,
                   latency_ms,
                   ROW_NUMBER() OVER (PARTITION BY target_id ORDER BY ts DESC) AS rn
            FROM checks
        )
        WHERE rn <= ?
        ORDER BY target_id, rn DESC`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]float64)
	for rows.Next() {
		var id string
		var value float64
		if err := rows.Scan(&id, &value); err != nil {
			return nil, err
		}
		out[id] = append(out[id], value)
	}
	return out, rows.Err()
}

// History buckets one target for the detail view. The bucket width is chosen
// by the caller so a day, a week and a month all render about the same number
// of points.
func (s *Store) History(targetID string, since time.Time, bucket time.Duration) ([]HistoryPoint, error) {
	width := int64(bucket.Seconds())
	if width < 1 {
		width = 60
	}

	rows, err := s.db.Query(`
        SELECT (ts / ?) * ? AS slot,
               AVG(latency_ms),
               MAX(latency_ms),
               SUM(CASE WHEN up = 1 THEN 1 ELSE 0 END),
               COUNT(*)
        FROM checks
        WHERE target_id = ? AND ts >= ?
        GROUP BY slot
        ORDER BY slot`, width, width, targetID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	points := make([]HistoryPoint, 0, 256)
	for rows.Next() {
		var p HistoryPoint
		if err := rows.Scan(&p.TS, &p.AvgMS, &p.MaxMS, &p.Up, &p.Total); err != nil {
			return nil, err
		}
		if p.Total > 0 {
			p.Percent = 100 * float64(p.Up) / float64(p.Total)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// ActiveIncidents returns every outage that has not recovered yet.
func (s *Store) ActiveIncidents() ([]*model.Incident, error) {
	return s.queryIncidents(`
        SELECT id, target_id, started_at, ended_at, reason, last_error
        FROM incidents
        WHERE ended_at IS NULL
        ORDER BY started_at ASC`)
}

// RecentIncidents returns the newest incidents, open ones first.
func (s *Store) RecentIncidents(limit int) ([]*model.Incident, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return s.queryIncidents(`
        SELECT id, target_id, started_at, ended_at, reason, last_error
        FROM incidents
        ORDER BY (ended_at IS NULL) DESC, started_at DESC
        LIMIT ?`, limit)
}

// IncidentsFor returns the outage history of one target.
func (s *Store) IncidentsFor(targetID string, limit int) ([]*model.Incident, error) {
	if limit <= 0 || limit > 500 {
		limit = 20
	}
	return s.queryIncidents(`
        SELECT id, target_id, started_at, ended_at, reason, last_error
        FROM incidents
        WHERE target_id = ?
        ORDER BY started_at DESC
        LIMIT ?`, targetID, limit)
}

func (s *Store) queryIncidents(query string, args ...any) ([]*model.Incident, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*model.Incident, 0, 16)
	for rows.Next() {
		var (
			inc     model.Incident
			started int64
			ended   sql.NullInt64
		)
		if err := rows.Scan(&inc.ID, &inc.TargetID, &started, &ended, &inc.Reason, &inc.LastError); err != nil {
			return nil, err
		}

		inc.StartedAt = time.Unix(started, 0).UTC()
		if ended.Valid {
			end := time.Unix(ended.Int64, 0).UTC()
			inc.EndedAt = &end
			inc.DurationSec = ended.Int64 - started
		} else {
			inc.Active = true
			// An open incident is still growing, so its duration is measured
			// against now rather than a stored end time.
			inc.DurationSec = time.Now().Unix() - started
		}
		out = append(out, &inc)
	}
	return out, rows.Err()
}

// RecentEvents returns the newest status transitions for the wall ticker.
func (s *Store) RecentEvents(limit int) ([]model.Event, error) {
	if limit <= 0 || limit > 200 {
		limit = 25
	}

	rows, err := s.db.Query(`
        SELECT target_id, at, from_status, to_status, detail
        FROM events
        ORDER BY at DESC
        LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]model.Event, 0, limit)
	for rows.Next() {
		var (
			ev   model.Event
			at   int64
			from string
			to   string
		)
		if err := rows.Scan(&ev.TargetID, &at, &from, &to, &ev.Detail); err != nil {
			return nil, err
		}
		ev.At = time.Unix(at, 0).UTC()
		ev.From = model.Status(from)
		ev.To = model.Status(to)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// LastDomainCheck reports when an apex was last looked up, so a restart does
// not re-query every registry immediately.
func (s *Store) LastDomainCheck(apex string) (time.Time, bool) {
	var at int64
	err := s.db.QueryRow(`SELECT checked_at FROM domain_state WHERE apex = ?`, apex).Scan(&at)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(at, 0).UTC(), true
}

// LastCertCheck reports when a target certificate was last inspected.
func (s *Store) LastCertCheck(targetID string) (time.Time, bool) {
	var at int64
	err := s.db.QueryRow(`SELECT checked_at FROM cert_state WHERE target_id = ?`, targetID).Scan(&at)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(at, 0).UTC(), true
}
