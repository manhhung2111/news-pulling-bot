package store

import (
	"database/sql"
	"errors"
	"time"
)

type DigestRecord struct {
	Date      string
	ChannelID string
	MessageID string
	ThreadID  string
	Provider  string
	PostedAt  time.Time
}

type ItemRecord struct {
	MessageID  string
	DigestDate string
	NormURL    string
	Title      string
	Source     string
}

// Feedback is one owner signal. Kind is "up", "down" or "comment".
type Feedback struct {
	Key           string
	DigestDate    string
	ItemMessageID string
	Kind          string
	Text          string
	CreatedAt     time.Time
}

// FeedbackWithItem is feedback joined with the item it was about (Title/Source empty for thread-level comments).
type FeedbackWithItem struct {
	Feedback
	Title  string
	Source string
}

func (s *Store) exists(query, arg string) (bool, error) {
	var one int
	err := s.db.QueryRow(query, arg).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) IsSent(normURL string) (bool, error) {
	return s.exists("SELECT 1 FROM sent WHERE norm_url = ?", normURL)
}

func (s *Store) IsSeen(normURL string) (bool, error) {
	return s.exists("SELECT 1 FROM seen WHERE norm_url = ?", normURL)
}

func (s *Store) MarkSeen(normURLs []string, at time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, u := range normURLs {
		if _, err := tx.Exec("INSERT OR IGNORE INTO seen (norm_url, first_seen_at) VALUES (?, ?)", u, at.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RecordDigest stores the posted digest and its items, and marks each item's URL as sent, in one transaction.
func (s *Store) RecordDigest(d DigestRecord, items []ItemRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR REPLACE INTO digests (date, channel_id, message_id, thread_id, provider, posted_at) VALUES (?, ?, ?, ?, ?, ?)`,
		d.Date, d.ChannelID, d.MessageID, d.ThreadID, d.Provider, d.PostedAt.Unix()); err != nil {
		return err
	}
	for _, it := range items {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO digest_items (message_id, digest_date, norm_url, title, source) VALUES (?, ?, ?, ?, ?)`,
			it.MessageID, it.DigestDate, it.NormURL, it.Title, it.Source); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO sent (norm_url, digest_date, sent_at) VALUES (?, ?, ?)`,
			it.NormURL, it.DigestDate, d.PostedAt.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) RecentDigests(since time.Time) ([]DigestRecord, error) {
	rows, err := s.db.Query(`SELECT date, channel_id, message_id, thread_id, provider, posted_at FROM digests WHERE posted_at >= ? ORDER BY posted_at`, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DigestRecord
	for rows.Next() {
		var d DigestRecord
		var cid, mid, tid, prov sql.NullString
		var posted int64
		if err := rows.Scan(&d.Date, &cid, &mid, &tid, &prov, &posted); err != nil {
			return nil, err
		}
		d.ChannelID, d.MessageID, d.ThreadID, d.Provider = cid.String, mid.String, tid.String, prov.String
		d.PostedAt = time.Unix(posted, 0)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ItemsForDigest(date string) ([]ItemRecord, error) {
	rows, err := s.db.Query(`SELECT message_id, digest_date, norm_url, title, source FROM digest_items WHERE digest_date = ?`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemRecord
	for rows.Next() {
		var it ItemRecord
		if err := rows.Scan(&it.MessageID, &it.DigestDate, &it.NormURL, &it.Title, &it.Source); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// UpsertFeedback inserts feedback, ignoring rows whose Key already exists.
func (s *Store) UpsertFeedback(fs []Feedback) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, f := range fs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO feedback (key, digest_date, item_message_id, kind, text, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
			f.Key, f.DigestDate, f.ItemMessageID, f.Kind, f.Text, f.CreatedAt.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const feedbackSelect = `SELECT f.key, f.digest_date, f.item_message_id, f.kind, f.text, f.created_at, COALESCE(i.title, ''), COALESCE(i.source, '')
FROM feedback f LEFT JOIN digest_items i ON i.message_id = f.item_message_id AND f.item_message_id != '' `

// FeedbackSince returns feedback created at or after since, newest first.
func (s *Store) FeedbackSince(since time.Time) ([]FeedbackWithItem, error) {
	return s.queryFeedback(feedbackSelect+`WHERE f.created_at >= ? ORDER BY f.created_at DESC`, since.Unix())
}

// FeedbackBefore returns feedback created before `before` and after `after`, newest first.
func (s *Store) FeedbackBefore(before, after time.Time) ([]FeedbackWithItem, error) {
	return s.queryFeedback(feedbackSelect+`WHERE f.created_at < ? AND f.created_at > ? ORDER BY f.created_at DESC`, before.Unix(), after.Unix())
}

func (s *Store) queryFeedback(q string, args ...any) ([]FeedbackWithItem, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FeedbackWithItem
	for rows.Next() {
		var f FeedbackWithItem
		var date, item, text sql.NullString
		var created int64
		if err := rows.Scan(&f.Key, &date, &item, &f.Kind, &text, &created, &f.Title, &f.Source); err != nil {
			return nil, err
		}
		f.DigestDate, f.ItemMessageID, f.Text = date.String, item.String, text.String
		f.CreatedAt = time.Unix(created, 0)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) GetKV(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow("SELECT value FROM kv WHERE key = ?", key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) SetKV(key, value string) error {
	_, err := s.db.Exec("INSERT INTO kv (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}
