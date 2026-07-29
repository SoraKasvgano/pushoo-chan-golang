package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type sqliteStore struct {
	db *sql.DB

	recordCh     chan recordOp
	recordStopCh chan struct{}
	recordWG     sync.WaitGroup
	recordMu     sync.RWMutex
	recordClosed bool
	closeOnce    sync.Once
	closeErr     error

	banCh     chan banOp
	banStopCh chan struct{}
	banWG     sync.WaitGroup

	recordChannelMessages bool
}

type SQLiteOptions struct {
	RecordChannelMessages bool
}

type banOpType int

const (
	banOpUpsert banOpType = iota
	banOpDelete
)

type banOp struct {
	kind        string
	ip          string
	failCount   int
	bannedUntil int64
	lastSeen    int64
	op          banOpType
}

type recordOp struct {
	msg        Message
	deliveries []Delivery
}

const (
	banQueueSize     = 4096
	recordQueueSize  = 4096
	banMaxBatch      = 500
	banFlushInterval = 1 * time.Second
)

func NewSQLite(path string, opts SQLiteOptions) (Store, func(), error) {
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
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)&_pragma=read_uncommitted(0)&_pragma=wal_autocheckpoint(1000)", filepath.ToSlash(path))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, err
	}
	// Keep concurrency reasonable. SQLite allows multiple readers; writers are serialized.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	s := &sqliteStore{
		db:                    db,
		recordChannelMessages: opts.RecordChannelMessages,
	}
	if err := s.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	if err := s.verifyPragmas(context.Background()); err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	s.startRecordWorker()
	s.startBanWorker()

	if isNewDB {
		fmt.Printf("[store] SQLite database created at %s\n", path)
	} else {
		fmt.Printf("[store] SQLite database opened at %s\n", path)
	}

	return s, func() { _ = s.Close() }, nil
}

func (s *sqliteStore) verifyPragmas(ctx context.Context) error {
	var journalMode string
	if err := s.db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		return fmt.Errorf("read SQLite journal mode: %w", err)
	}
	if journalMode != "wal" {
		return fmt.Errorf("SQLite WAL mode is not active (journal_mode=%s)", journalMode)
	}
	var busyTimeout, foreignKeys, readUncommitted int
	if err := s.db.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("read SQLite busy timeout: %w", err)
	}
	if busyTimeout < 5000 {
		return fmt.Errorf("SQLite busy timeout is too low (%dms)", busyTimeout)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("read SQLite foreign keys setting: %w", err)
	}
	if foreignKeys != 1 {
		return fmt.Errorf("SQLite foreign key enforcement is not active")
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA read_uncommitted;").Scan(&readUncommitted); err != nil {
		return fmt.Errorf("read SQLite isolation setting: %w", err)
	}
	if readUncommitted != 0 {
		return fmt.Errorf("SQLite dirty reads are enabled")
	}
	rows, err := s.db.QueryContext(ctx, "PRAGMA foreign_key_check;")
	if err != nil {
		return fmt.Errorf("check SQLite foreign keys: %w", err)
	}
	if rows.Next() {
		_ = rows.Close()
		return fmt.Errorf("SQLite contains orphaned rows; PRAGMA foreign_key_check failed")
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("scan SQLite foreign key check: %w", err)
	}
	_ = rows.Close()
	var emptyMessages int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages m
WHERE NOT EXISTS (SELECT 1 FROM deliveries d WHERE d.message_id = m.id)`).Scan(&emptyMessages); err != nil {
		return fmt.Errorf("check SQLite message consistency: %w", err)
	}
	if emptyMessages != 0 {
		return fmt.Errorf("SQLite contains %d messages without delivery records", emptyMessages)
	}
	return nil
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

CREATE TABLE IF NOT EXISTS ip_bans (
  kind TEXT NOT NULL,
  ip TEXT NOT NULL,
  fail_count INTEGER NOT NULL,
  banned_until INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY(kind, ip)
);

CREATE TABLE IF NOT EXISTS channel_messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  remote_addr TEXT,
  channel_name TEXT,
  channel_type TEXT,
  title TEXT,
  content TEXT,
  status TEXT,
  detail TEXT,
  FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_deliveries_message_id ON deliveries(message_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
CREATE INDEX IF NOT EXISTS idx_ip_bans_last_seen ON ip_bans(last_seen);
CREATE INDEX IF NOT EXISTS idx_channel_messages_message_id ON channel_messages(message_id);
CREATE INDEX IF NOT EXISTS idx_channel_messages_created_at ON channel_messages(created_at);
`)
	return err
}

func (s *sqliteStore) Record(ctx context.Context, msg Message, deliveries []Delivery) error {
	if err := validateRecord(deliveries); err != nil {
		return err
	}
	s.recordMu.RLock()
	defer s.recordMu.RUnlock()
	if s.recordClosed {
		return fmt.Errorf("SQLite store is closed")
	}
	op := recordOp{msg: msg, deliveries: append([]Delivery(nil), deliveries...)}
	select {
	case s.recordCh <- op:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *sqliteStore) recordSync(ctx context.Context, msg Message, deliveries []Delivery) error {
	if err := validateRecord(deliveries); err != nil {
		return err
	}
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
		var msgStmt *sql.Stmt
		if s.recordChannelMessages {
			msgStmt, err = tx.PrepareContext(ctx,
				`INSERT INTO channel_messages(message_id, created_at, remote_addr, channel_name, channel_type, title, content, status, detail)
				 VALUES(?,?,?,?,?,?,?,?,?)`)
			if err != nil {
				return err
			}
			defer msgStmt.Close()
		}
		for _, d := range deliveries {
			t := d.CreatedAt
			if t.IsZero() {
				t = time.Now()
			}
			if _, err := stmt.ExecContext(ctx, id, t.Unix(), d.ChannelName, d.ChannelType, d.Status, d.Detail); err != nil {
				return err
			}
			if s.recordChannelMessages && msgStmt != nil {
				if _, err := msgStmt.ExecContext(ctx, id, t.Unix(), msg.RemoteAddr, d.ChannelName, d.ChannelType, msg.Title, msg.Content, d.Status, d.Detail); err != nil {
					return err
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func validateRecord(deliveries []Delivery) error {
	if len(deliveries) == 0 {
		return fmt.Errorf("push history requires at least one delivery")
	}
	for i, delivery := range deliveries {
		if delivery.ChannelName == "" || delivery.Status == "" {
			return fmt.Errorf("delivery %d requires channel name and status", i+1)
		}
	}
	return nil
}

func (s *sqliteStore) startRecordWorker() {
	s.recordCh = make(chan recordOp, recordQueueSize)
	s.recordStopCh = make(chan struct{})
	s.recordWG.Add(1)
	go func() {
		defer s.recordWG.Done()
		write := func(op recordOp) {
			if err := s.recordSync(context.Background(), op.msg, op.deliveries); err != nil {
				log.Printf("[store] failed to record push history: %v", err)
			}
		}
		for {
			select {
			case op := <-s.recordCh:
				write(op)
			case <-s.recordStopCh:
				for {
					select {
					case op := <-s.recordCh:
						write(op)
					default:
						return
					}
				}
			}
		}
	}()
}

func (s *sqliteStore) UpsertBan(ctx context.Context, rec BanRecord) error {
	return s.enqueueBanOp(ctx, banOp{
		kind:        rec.Kind,
		ip:          rec.IP,
		failCount:   rec.FailCount,
		bannedUntil: rec.BannedUntil.Unix(),
		lastSeen:    rec.LastSeen.Unix(),
		op:          banOpUpsert,
	})
}

func (s *sqliteStore) DeleteBan(ctx context.Context, kind, ip string) error {
	return s.enqueueBanOp(ctx, banOp{
		kind: kind,
		ip:   ip,
		op:   banOpDelete,
	})
}

func (s *sqliteStore) CleanupBans(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ip_bans WHERE last_seen < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	return rows, nil
}

func (s *sqliteStore) Compact(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA optimize;"); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "VACUUM;")
	return err
}

func (s *sqliteStore) Cleanup(ctx context.Context, before time.Time) (CleanupResult, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return CleanupResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	msgRes, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE created_at < ?`, before.Unix())
	if err != nil {
		return CleanupResult{}, err
	}
	banRes, err := tx.ExecContext(ctx, `DELETE FROM ip_bans WHERE last_seen < ?`, before.Unix())
	if err != nil {
		return CleanupResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CleanupResult{}, err
	}

	msgDeleted, _ := msgRes.RowsAffected()
	banDeleted, _ := banRes.RowsAffected()
	return CleanupResult{
		MessagesDeleted: msgDeleted,
		BansDeleted:     banDeleted,
		Before:          before,
	}, nil
}

func (s *sqliteStore) BanTrends(ctx context.Context) (BanTrendStats, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return BanTrendStats{}, err
	}
	defer tx.Rollback()
	now := time.Now()
	hourStart := now.Truncate(time.Hour)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	last24Start := hourStart.Add(-23 * time.Hour)
	last7Start := dayStart.AddDate(0, 0, -6)

	hourPoints := make([]TrendPoint, 0, 24)
	for i := 0; i < 24; i++ {
		t := last24Start.Add(time.Duration(i) * time.Hour)
		label := t.Format("01-02 15:00")
		hourPoints = append(hourPoints, TrendPoint{Label: label, Count: 0})
	}
	dayPoints := make([]TrendPoint, 0, 7)
	for i := 0; i < 7; i++ {
		t := last7Start.AddDate(0, 0, i)
		label := t.Format("01-02")
		dayPoints = append(dayPoints, TrendPoint{Label: label, Count: 0})
	}

	rows, err := tx.QueryContext(ctx, `
SELECT strftime('%Y-%m-%d %H:00', updated_at, 'unixepoch', 'localtime') AS bucket, COUNT(*)
FROM ip_bans
WHERE updated_at >= ?
GROUP BY bucket
`, last24Start.Unix())
	if err != nil {
		return BanTrendStats{}, err
	}
	hourMap := map[string]int{}
	for rows.Next() {
		var bucket string
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			_ = rows.Close()
			return BanTrendStats{}, err
		}
		t, err := time.ParseInLocation("2006-01-02 15:04", bucket, now.Location())
		if err == nil {
			hourMap[t.Format("01-02 15:00")] = count
		}
	}
	_ = rows.Close()
	for i := range hourPoints {
		if v, ok := hourMap[hourPoints[i].Label]; ok {
			hourPoints[i].Count = v
		}
	}

	rows, err = tx.QueryContext(ctx, `
SELECT strftime('%Y-%m-%d', updated_at, 'unixepoch', 'localtime') AS bucket, COUNT(*)
FROM ip_bans
WHERE updated_at >= ?
GROUP BY bucket
`, last7Start.Unix())
	if err != nil {
		return BanTrendStats{}, err
	}
	dayMap := map[string]int{}
	for rows.Next() {
		var bucket string
		var count int
		if err := rows.Scan(&bucket, &count); err != nil {
			_ = rows.Close()
			return BanTrendStats{}, err
		}
		t, err := time.ParseInLocation("2006-01-02", bucket, now.Location())
		if err == nil {
			dayMap[t.Format("01-02")] = count
		}
	}
	_ = rows.Close()
	for i := range dayPoints {
		if v, ok := dayMap[dayPoints[i].Label]; ok {
			dayPoints[i].Count = v
		}
	}
	if err := tx.Commit(); err != nil {
		return BanTrendStats{}, err
	}

	return BanTrendStats{
		Last24h: hourPoints,
		Last7d:  dayPoints,
	}, nil
}

func (s *sqliteStore) ListChannelMessages(ctx context.Context, page, pageSize int) (NotificationPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return NotificationPage{}, err
	}
	defer tx.Rollback()

	var total int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_messages`).Scan(&total); err != nil {
		return NotificationPage{}, err
	}

	rows, err := tx.QueryContext(ctx, `
SELECT id, created_at, remote_addr, channel_name, channel_type, title, content, status, detail
FROM channel_messages
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?`, pageSize, offset)
	if err != nil {
		return NotificationPage{}, err
	}
	defer rows.Close()

	items := make([]NotificationRecord, 0, pageSize)
	for rows.Next() {
		var rec NotificationRecord
		var created int64
		if err := rows.Scan(&rec.ID, &created, &rec.RemoteAddr, &rec.ChannelName, &rec.ChannelType, &rec.Title, &rec.Content, &rec.Status, &rec.Detail); err != nil {
			return NotificationPage{}, err
		}
		rec.CreatedAt = time.Unix(created, 0)
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return NotificationPage{}, err
	}
	if err := tx.Commit(); err != nil {
		return NotificationPage{}, err
	}

	return NotificationPage{
		Items:    items,
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}, nil
}

func (s *sqliteStore) Summary(ctx context.Context) (StoreSummary, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return StoreSummary{}, err
	}
	defer tx.Rollback()
	var total int64
	var lastCreated sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM channel_messages`).Scan(&total); err != nil {
		return StoreSummary{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT MAX(created_at) FROM channel_messages`).Scan(&lastCreated); err != nil {
		return StoreSummary{}, err
	}

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	var todaySent int64
	var todayFailed int64
	if err := tx.QueryRowContext(ctx, `
SELECT
  COALESCE(SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END), 0) AS sent,
  COALESCE(SUM(CASE WHEN status != 'success' THEN 1 ELSE 0 END), 0) AS failed
FROM channel_messages
WHERE created_at >= ?`, dayStart).Scan(&todaySent, &todayFailed); err != nil {
		return StoreSummary{}, err
	}

	var last time.Time
	if lastCreated.Valid && lastCreated.Int64 > 0 {
		last = time.Unix(lastCreated.Int64, 0)
	}
	if err := tx.Commit(); err != nil {
		return StoreSummary{}, err
	}
	return StoreSummary{
		NotificationTotal: total,
		LastSentAt:        last,
		TodaySent:         todaySent,
		TodayFailed:       todayFailed,
	}, nil
}

func (s *sqliteStore) startBanWorker() {
	s.banCh = make(chan banOp, banQueueSize)
	s.banStopCh = make(chan struct{})
	s.banWG.Add(1)
	go func() {
		defer s.banWG.Done()
		ticker := time.NewTicker(banFlushInterval)
		defer ticker.Stop()

		batch := make([]banOp, 0, banMaxBatch)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			_ = s.flushBanOps(context.Background(), batch)
			batch = batch[:0]
		}

		for {
			select {
			case op := <-s.banCh:
				batch = append(batch, op)
				if len(batch) >= banMaxBatch {
					flush()
				}
			case <-ticker.C:
				flush()
			case <-s.banStopCh:
				for {
					select {
					case op := <-s.banCh:
						batch = append(batch, op)
						if len(batch) >= banMaxBatch {
							flush()
						}
					default:
						flush()
						return
					}
				}
			}
		}
	}()
}

func (s *sqliteStore) enqueueBanOp(ctx context.Context, op banOp) error {
	select {
	case s.banCh <- op:
		return nil
	default:
		return s.flushBanOps(ctx, []banOp{op})
	}
}

func (s *sqliteStore) flushBanOps(ctx context.Context, ops []banOp) error {
	if len(ops) == 0 {
		return nil
	}
	type key struct {
		kind string
		ip   string
	}
	merged := make(map[key]banOp, len(ops))
	for _, op := range ops {
		merged[key{kind: op.kind, ip: op.ip}] = op
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	upsertStmt, err := tx.PrepareContext(ctx, `
INSERT INTO ip_bans(kind, ip, fail_count, banned_until, last_seen, updated_at)
VALUES(?,?,?,?,?,?)
ON CONFLICT(kind, ip) DO UPDATE SET
  fail_count=excluded.fail_count,
  banned_until=excluded.banned_until,
  last_seen=excluded.last_seen,
  updated_at=excluded.updated_at`)
	if err != nil {
		return err
	}
	defer upsertStmt.Close()

	deleteStmt, err := tx.PrepareContext(ctx, `DELETE FROM ip_bans WHERE kind=? AND ip=?`)
	if err != nil {
		return err
	}
	defer deleteStmt.Close()

	now := time.Now().Unix()
	for _, op := range merged {
		switch op.op {
		case banOpDelete:
			if _, err := deleteStmt.ExecContext(ctx, op.kind, op.ip); err != nil {
				return err
			}
		case banOpUpsert:
			if _, err := upsertStmt.ExecContext(ctx, op.kind, op.ip, op.failCount, op.bannedUntil, op.lastSeen, now); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

func (s *sqliteStore) Close() error {
	s.closeOnce.Do(func() {
		s.recordMu.Lock()
		s.recordClosed = true
		close(s.recordStopCh)
		s.recordMu.Unlock()
		s.recordWG.Wait()
		if s.banStopCh != nil {
			close(s.banStopCh)
			s.banWG.Wait()
		}
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}
