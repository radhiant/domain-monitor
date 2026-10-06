// Package store persists check history, incidents and the latest probe records
// in an embedded SQLite database.
//
// The driver is modernc.org/sqlite, a pure-Go translation of SQLite: it keeps
// the agent buildable with CGO_ENABLED=0 and shippable as a single static
// binary, exactly like the server-monitor agent.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"domain-monitor/backend/internal/model"
)

// Store owns the database handle.
type Store struct {
	db *sql.DB
}

// Open creates the database file if needed and applies the schema.
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}

	// WAL keeps the minute-by-minute writes from blocking dashboard reads;
	// NORMAL sync is the right trade for telemetry that is regenerated on the
	// next tick anyway.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	// A single connection avoids SQLITE_BUSY entirely; the probe workers funnel
	// their results through one goroutine, so there is nothing to parallelise.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// SyncTargets writes the current target list and archives anything that has
// disappeared from list-domain.txt. Rows are never deleted: the history of a
// removed subdomain stays queryable.
func (s *Store) SyncTargets(targets []model.Target) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE targets SET archived = 1`); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
        INSERT INTO targets (id, apex, host, url, label, is_apex, ord, archived, first_seen)
        VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?)
        ON CONFLICT(id) DO UPDATE SET
            apex = excluded.apex,
            host = excluded.host,
            url = excluded.url,
            label = excluded.label,
            is_apex = excluded.is_apex,
            ord = excluded.ord,
            archived = 0`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().Unix()
	for _, t := range targets {
		if _, err := stmt.Exec(t.ID, t.Apex, t.Host, t.URL, t.Label, boolToInt(t.IsApex), t.Order, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// InsertCheck records one reachability sample.
func (s *Store) InsertCheck(targetID string, status model.Status, res *model.HTTPResult) error {
	if res == nil {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO checks (target_id, ts, up, status, status_code, latency_ms, error)
         VALUES (?, ?, ?, ?, ?, ?, ?)`,
		targetID, res.CheckedAt.Unix(), boolToInt(res.Up), string(status),
		res.StatusCode, res.LatencyMS, res.Error,
	)
	return err
}

// SaveCert stores the latest certificate record for a target.
func (s *Store) SaveCert(targetID string, info *model.CertInfo) error {
	return s.saveJSON("cert_state", "target_id", targetID, info.CheckedAt, info)
}

// SaveDNS stores the latest resolution record for a target.
func (s *Store) SaveDNS(targetID string, info *model.DNSInfo) error {
	return s.saveJSON("dns_state", "target_id", targetID, info.CheckedAt, info)
}

// SaveDomain stores the latest registration record for an apex domain.
func (s *Store) SaveDomain(info *model.DomainInfo) error {
	return s.saveJSON("domain_state", "apex", info.Apex, info.CheckedAt, info)
}

func (s *Store) saveJSON(table, keyCol, key string, checkedAt time.Time, payload any) error {
	blob, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// The table and key column are compile-time constants from this file, never
	// user input, so this interpolation cannot carry an injection.
	query := fmt.Sprintf(`
        INSERT INTO %s (%s, checked_at, payload) VALUES (?, ?, ?)
        ON CONFLICT(%s) DO UPDATE SET checked_at = excluded.checked_at, payload = excluded.payload`,
		table, keyCol, keyCol)

	_, err = s.db.Exec(query, key, checkedAt.Unix(), string(blob))
	return err
}

// LoadCerts returns every stored certificate record, keyed by target id.
func (s *Store) LoadCerts() (map[string]*model.CertInfo, error) {
	out := make(map[string]*model.CertInfo)
	err := s.loadJSON("cert_state", "target_id", func(key string, blob []byte) error {
		var info model.CertInfo
		if err := json.Unmarshal(blob, &info); err != nil {
			return err
		}
		// Days-left was computed when the row was written. Recompute it, so an
		// agent that was stopped for a week does not come back claiming the
		// headroom it had a week ago.
		if info.NotAfter != nil {
			days := int(time.Until(*info.NotAfter).Hours() / 24)
			info.DaysLeft = &days
		}
		out[key] = &info
		return nil
	})
	return out, err
}

// LoadDNS returns every stored resolution record, keyed by target id.
func (s *Store) LoadDNS() (map[string]*model.DNSInfo, error) {
	out := make(map[string]*model.DNSInfo)
	err := s.loadJSON("dns_state", "target_id", func(key string, blob []byte) error {
		var info model.DNSInfo
		if err := json.Unmarshal(blob, &info); err != nil {
			return err
		}
		out[key] = &info
		return nil
	})
	return out, err
}

// LoadDomains returns every stored registration record, keyed by apex.
func (s *Store) LoadDomains() (map[string]*model.DomainInfo, error) {
	out := make(map[string]*model.DomainInfo)
	err := s.loadJSON("domain_state", "apex", func(key string, blob []byte) error {
		var info model.DomainInfo
		if err := json.Unmarshal(blob, &info); err != nil {
			return err
		}
		if info.ExpiresAt != nil {
			days := int(time.Until(*info.ExpiresAt).Hours() / 24)
			info.DaysLeft = &days
		}
		out[key] = &info
		return nil
	})
	return out, err
}

func (s *Store) loadJSON(table, keyCol string, fn func(key string, blob []byte) error) error {
	rows, err := s.db.Query(fmt.Sprintf(`SELECT %s, payload FROM %s`, keyCol, table))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var key, payload string
		if err := rows.Scan(&key, &payload); err != nil {
			return err
		}
		// One unreadable row must not blank the entire warm start.
		_ = fn(key, []byte(payload))
	}
	return rows.Err()
}

// OpenIncident records the start of an outage and returns its row id.
func (s *Store) OpenIncident(targetID, reason, lastError string, at time.Time) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO incidents (target_id, started_at, reason, last_error) VALUES (?, ?, ?, ?)`,
		targetID, at.Unix(), reason, lastError,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// CloseIncident marks an outage as recovered.
func (s *Store) CloseIncident(id int64, at time.Time) error {
	_, err := s.db.Exec(`UPDATE incidents SET ended_at = ? WHERE id = ? AND ended_at IS NULL`, at.Unix(), id)
	return err
}

// InsertEvent records a status transition for the wall ticker.
func (s *Store) InsertEvent(ev model.Event) error {
	_, err := s.db.Exec(
		`INSERT INTO events (target_id, at, from_status, to_status, detail) VALUES (?, ?, ?, ?, ?)`,
		ev.TargetID, ev.At.Unix(), string(ev.From), string(ev.To), ev.Detail,
	)
	return err
}

// Prune enforces retention. Raw checks are the only table that grows without
// bound; incidents and events are kept far longer, because they are what an
// operator actually looks back at.
func (s *Store) Prune(retentionDays int) error {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	if _, err := s.db.Exec(`DELETE FROM checks WHERE ts < ?`, cutoff); err != nil {
		return err
	}

	eventCutoff := time.Now().AddDate(0, 0, -retentionDays*3).Unix()
	if _, err := s.db.Exec(`DELETE FROM events WHERE at < ?`, eventCutoff); err != nil {
		return err
	}

	incidentCutoff := time.Now().AddDate(0, 0, -180).Unix()
	_, err := s.db.Exec(`DELETE FROM incidents WHERE ended_at IS NOT NULL AND ended_at < ?`, incidentCutoff)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
