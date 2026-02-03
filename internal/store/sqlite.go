package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB
}

func NewSQLite(path string) (Store, func(), error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, nil, fmt.Errorf("failed to create database directory: %w", err)
		}
	}

	// Check if database file exists (for logging purposes)
	_, err := os.Stat(path)
	isNewDB := os.IsNotExist(err)

	// modernc sqlite supports "file:" DSN.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_busy_timeout=5000", filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, err
	}
	// Keep concurrency reasonable. SQLite allows multiple readers; writers are serialized.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	s := &sqliteStore{db: db}
	if err := s.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, nil, err
	}

	if isNewDB {
		fmt.Printf("[store] SQLite database created at %s\n", path)
	} else {
		fmt.Printf("[store] SQLite database opened at %s\n", path)
	}

	return s, func() { _ = db.Close() }, nil
}

func (s *sqliteStore) init(ctx context.Context) error {
	// Ensure foreign keys on every connection.
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at INTEGER NOT NULL,
  remote_addr TEXT,
  path TEXT,
  format TEXT,
  chan TEXT,
  title TEXT,
  content TEXT
);

CREATE TABLE IF NOT EXISTS deliveries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  channel_name TEXT,
  channel_type TEXT,
  status TEXT,
  detail TEXT,
  FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_deliveries_message_id ON deliveries(message_id);
`)
	return err
}

func (s *sqliteStore) Record(ctx context.Context, msg Message, deliveries []Delivery) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := msg.CreatedAt
	if now.IsZero() {
		now = time.Now()
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO messages(created_at, remote_addr, path, format, chan, title, content)
		 VALUES(?,?,?,?,?,?,?)`,
		now.Unix(),
		msg.RemoteAddr,
		msg.Path,
		msg.Format,
		msg.Chan,
		msg.Title,
		msg.Content,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}

	if len(deliveries) > 0 {
		stmt, err := tx.PrepareContext(ctx,
			`INSERT INTO deliveries(message_id, created_at, channel_name, channel_type, status, detail)
			 VALUES(?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, d := range deliveries {
			t := d.CreatedAt
			if t.IsZero() {
				t = time.Now()
			}
			if _, err := stmt.ExecContext(ctx, id, t.Unix(), d.ChannelName, d.ChannelType, d.Status, d.Detail); err != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *sqliteStore) Close() error { return s.db.Close() }

