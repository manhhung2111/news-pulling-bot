// Package store keeps the digest's history, posted digests and owner feedback in SQLite.
package store

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS sent (norm_url TEXT PRIMARY KEY, digest_date TEXT, sent_at INTEGER);
CREATE TABLE IF NOT EXISTS seen (norm_url TEXT PRIMARY KEY, first_seen_at INTEGER);
CREATE TABLE IF NOT EXISTS digests (date TEXT PRIMARY KEY, channel_id TEXT, message_id TEXT, thread_id TEXT, provider TEXT, posted_at INTEGER);
CREATE TABLE IF NOT EXISTS digest_items (message_id TEXT PRIMARY KEY, digest_date TEXT, norm_url TEXT, title TEXT, source TEXT);
CREATE TABLE IF NOT EXISTS feedback (key TEXT PRIMARY KEY, digest_date TEXT, item_message_id TEXT, kind TEXT, text TEXT, created_at INTEGER);
CREATE TABLE IF NOT EXISTS kv (key TEXT PRIMARY KEY, value TEXT);
`

// Open opens (or creates) the database at path and runs migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }
