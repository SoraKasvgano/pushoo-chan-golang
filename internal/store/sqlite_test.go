package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLitePragmasAndRecordTransaction(t *testing.T) {
	store, closeStore, err := NewSQLite(filepath.Join(t.TempDir(), "store.db"), SQLiteOptions{RecordChannelMessages: true})
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()

	s := store.(*sqliteStore)
	if err := s.verifyPragmas(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_delivery BEFORE INSERT ON deliveries
WHEN NEW.channel_name = 'reject' BEGIN SELECT RAISE(ABORT, 'test failure'); END;`); err != nil {
		t.Fatal(err)
	}

	err = s.Record(context.Background(), Message{CreatedAt: time.Now(), Title: "atomic"}, []Delivery{
		{ChannelName: "accepted", Status: "ok"},
		{ChannelName: "reject", Status: "error"},
	})
	if err == nil {
		t.Fatal("expected the delivery trigger to abort the transaction")
	}
	for _, table := range []string{"messages", "deliveries", "channel_messages"} {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s contains %d rows after rollback", table, count)
		}
	}
}
