// Package store persists the Slack root-message state for each Alertmanager
// alert group, in Postgres or SQLite.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// AlertGroup tracks the Slack root message for one Alertmanager group.
type AlertGroup struct {
	Receiver   string
	GroupKey   string
	Channel    string
	MessageTS  string
	Status     string
	ResolvedAt *time.Time
}

type Store struct {
	db     *sql.DB
	sqlite bool
}

// New opens the store. dsn is a Postgres connection string, or a
// "sqlite://path/to/file.db" (or "sqlite::memory:") URL for SQLite.
func New(dsn string) (*Store, error) {
	driver := "postgres"
	sqlite := false
	if rest, ok := strings.CutPrefix(dsn, "sqlite://"); ok {
		driver, dsn, sqlite = "sqlite", rest, true
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if sqlite {
		// modernc.org/sqlite gives each pooled connection its own private
		// database for in-memory DSNs, so a second connection can't see
		// tables migrated on the first. One connection avoids that.
		db.SetMaxOpenConns(1)
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	s := &Store{db: db, sqlite: sqlite}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	timestampType := "TIMESTAMPTZ"
	if s.sqlite {
		timestampType = "TIMESTAMP"
	}
	_, err := s.db.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS alert_groups (
	receiver    TEXT NOT NULL,
	group_key   TEXT NOT NULL,
	channel     TEXT NOT NULL,
	message_ts  TEXT NOT NULL,
	status      TEXT NOT NULL,
	resolved_at %[1]s,
	updated_at  %[1]s NOT NULL DEFAULT (%s),
	PRIMARY KEY (receiver, group_key)
)`, timestampType, s.now()))
	if err != nil {
		return err
	}
	if _, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS group_scopes (
	receiver  TEXT NOT NULL,
	group_key TEXT NOT NULL,
	labels    TEXT NOT NULL,
	PRIMARY KEY (receiver, group_key)
)`); err != nil {
		return err
	}
	_, err = s.db.Exec(`
CREATE TABLE IF NOT EXISTS inhibited_posted (
	receiver    TEXT NOT NULL,
	group_key   TEXT NOT NULL,
	fingerprint TEXT NOT NULL,
	PRIMARY KEY (receiver, group_key, fingerprint)
)`)
	return err
}

// FiringGroup is a firing group plus the label scope used to look up its
// inhibited alerts, for the background poller.
type FiringGroup struct {
	AlertGroup
	Labels map[string]string
}

// SaveScope remembers a group's common labels so the poller can query Alertmanager for it later.
func (s *Store) SaveScope(receiver, groupKey string, labels map[string]string) error {
	b, err := json.Marshal(labels)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO group_scopes (receiver, group_key, labels) VALUES ($1, $2, $3)
		 ON CONFLICT (receiver, group_key) DO UPDATE SET labels = EXCLUDED.labels`,
		receiver, groupKey, string(b))
	return err
}

// ListFiring returns every firing group that has a saved scope.
func (s *Store) ListFiring() ([]FiringGroup, error) {
	rows, err := s.db.Query(
		`SELECT g.receiver, g.group_key, g.channel, g.message_ts, c.labels
		 FROM alert_groups g JOIN group_scopes c ON c.receiver = g.receiver AND c.group_key = g.group_key
		 WHERE g.status = 'firing'`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []FiringGroup
	for rows.Next() {
		var fg FiringGroup
		var labels string
		if err := rows.Scan(&fg.Receiver, &fg.GroupKey, &fg.Channel, &fg.MessageTS, &labels); err != nil {
			return nil, err
		}
		fg.Status = "firing"
		if err := json.Unmarshal([]byte(labels), &fg.Labels); err != nil {
			return nil, err
		}
		out = append(out, fg)
	}
	return out, rows.Err()
}

// MarkInhibitedPosted records fingerprints already posted to the group's
// thread and returns only the ones that were new, so each inhibited alert
// gets exactly one thread comment.
func (s *Store) MarkInhibitedPosted(receiver, groupKey string, fingerprints []string) ([]string, error) {
	var fresh []string
	for _, fp := range fingerprints {
		res, err := s.db.Exec(
			`INSERT INTO inhibited_posted (receiver, group_key, fingerprint) VALUES ($1, $2, $3)
			 ON CONFLICT DO NOTHING`, receiver, groupKey, fp)
		if err != nil {
			return fresh, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			fresh = append(fresh, fp)
		}
	}
	return fresh, nil
}

// now returns the SQL expression for the current timestamp in the active dialect.
func (s *Store) now() string {
	if s.sqlite {
		return "CURRENT_TIMESTAMP"
	}
	return "now()"
}

// Get returns the tracked group, or nil if none exists yet.
func (s *Store) Get(receiver, groupKey string) (*AlertGroup, error) {
	var ag AlertGroup
	err := s.db.QueryRow(
		`SELECT receiver, group_key, channel, message_ts, status, resolved_at
		 FROM alert_groups WHERE receiver = $1 AND group_key = $2`,
		receiver, groupKey,
	).Scan(&ag.Receiver, &ag.GroupKey, &ag.Channel, &ag.MessageTS, &ag.Status, &ag.ResolvedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ag, nil
}

// Create records a group's new root Slack message, overwriting any prior
// (resolved) row for the same group so a fresh occurrence starts a new thread.
func (s *Store) Create(ag AlertGroup) error {
	if _, err := s.db.Exec(`DELETE FROM inhibited_posted WHERE receiver = $1 AND group_key = $2`, ag.Receiver, ag.GroupKey); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`INSERT INTO alert_groups (receiver, group_key, channel, message_ts, status)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (receiver, group_key) DO UPDATE
		 SET channel = EXCLUDED.channel,
		     message_ts = EXCLUDED.message_ts,
		     status = EXCLUDED.status,
		     resolved_at = NULL,
		     updated_at = `+s.now(),
		ag.Receiver, ag.GroupKey, ag.Channel, ag.MessageTS, ag.Status,
	)
	return err
}

// MarkResolved flips a group to resolved so the cleanup ticker can reap it later.
func (s *Store) MarkResolved(receiver, groupKey string) error {
	_, err := s.db.Exec(
		`UPDATE alert_groups SET status = 'resolved', resolved_at = `+s.now()+`, updated_at = `+s.now()+`
		 WHERE receiver = $1 AND group_key = $2`,
		receiver, groupKey,
	)
	return err
}

// Touch bumps updated_at when a firing group gets a new thread reply.
func (s *Store) Touch(receiver, groupKey string) error {
	_, err := s.db.Exec(
		`UPDATE alert_groups SET updated_at = `+s.now()+` WHERE receiver = $1 AND group_key = $2`,
		receiver, groupKey,
	)
	return err
}

// DeleteResolvedOlderThan reaps resolved groups past the retention window.
func (s *Store) DeleteResolvedOlderThan(age time.Duration) (int64, error) {
	seconds := int64(age.Seconds())
	cutoff := "now() - $1::interval"
	threshold := fmt.Sprintf("%d seconds", seconds)
	if s.sqlite {
		cutoff = "datetime('now', $1)"
		threshold = fmt.Sprintf("-%d seconds", seconds)
	}
	for _, table := range [...]string{"inhibited_posted", "group_scopes"} {
		if _, err := s.db.Exec(
			`DELETE FROM `+table+` WHERE NOT EXISTS (
				SELECT 1 FROM alert_groups g WHERE g.receiver = `+table+`.receiver AND g.group_key = `+table+`.group_key
				AND NOT (g.status = 'resolved' AND g.resolved_at < `+cutoff+`))`,
			threshold,
		); err != nil {
			return 0, err
		}
	}
	res, err := s.db.Exec(
		`DELETE FROM alert_groups WHERE status = 'resolved' AND resolved_at < `+cutoff,
		threshold,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
