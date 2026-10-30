package rule

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver without cgo
)

// SQLiteStore keeps the rules in SQLite, one JSON document per rule.
type SQLiteStore struct {
	db *sql.DB
}

// OpenSQLite opens or creates the database at path.
func OpenSQLite(ctx context.Context, path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("sql.Open(): %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS rules (
		id TEXT PRIMARY KEY,
		channel_id TEXT NOT NULL,
		data TEXT NOT NULL
	)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS rules_channel_id ON rules (channel_id)`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create index: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) ListByChannel(ctx context.Context, channelID string) ([]Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT data FROM rules WHERE channel_id = ?`, channelID)
	if err != nil {
		return nil, fmt.Errorf("select rules: %w", err)
	}
	defer rows.Close()
	var rules []Rule
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		var r Rule
		if err := json.Unmarshal([]byte(data), &r); err != nil {
			return nil, fmt.Errorf("json.Unmarshal(): %w", err)
		}
		rules = append(rules, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rules: %w", err)
	}
	sortRules(rules)
	return rules, nil
}

func (s *SQLiteStore) Get(ctx context.Context, id string) (Rule, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM rules WHERE id = ?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Rule{}, ErrNotFound
	}
	if err != nil {
		return Rule{}, fmt.Errorf("select rule: %w", err)
	}
	var r Rule
	if err := json.Unmarshal([]byte(data), &r); err != nil {
		return Rule{}, fmt.Errorf("json.Unmarshal(): %w", err)
	}
	return r, nil
}

func (s *SQLiteStore) Put(ctx context.Context, r Rule) error {
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("json.Marshal(): %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO rules (id, channel_id, data) VALUES (?, ?, ?)
		 ON CONFLICT (id) DO UPDATE SET channel_id = excluded.channel_id, data = excluded.data`,
		r.ID, r.ChannelID, string(data)); err != nil {
		return fmt.Errorf("upsert rule: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete rule: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("sql.DB#Close(): %w", err)
	}
	return nil
}
