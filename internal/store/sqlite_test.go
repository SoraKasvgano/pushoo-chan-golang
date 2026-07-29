package store

import (
	"context"
	"path/filepath"
	"sync"
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

	err = s.recordSync(context.Background(), Message{CreatedAt: time.Now(), Title: "atomic"}, []Delivery{
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

func TestSQLiteConcurrentRecordQueueDrainsWithoutOrphans(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	store, closeStore, err := NewSQLite(path, SQLiteOptions{RecordChannelMessages: true})
	if err != nil {
		t.Fatal(err)
	}

	const messages = 200
	var wg sync.WaitGroup
	errCh := make(chan error, messages)
	for i := 0; i < messages; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- store.Record(context.Background(), Message{CreatedAt: time.Now(), Title: "load"}, []Delivery{
				{ChannelName: "one", Status: "success"},
				{ChannelName: "two", Status: "success"},
			})
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}
	}
	closeStore()

	reopened, closeReopened, err := NewSQLite(path, SQLiteOptions{RecordChannelMessages: true})
	if err != nil {
		t.Fatal(err)
	}
	defer closeReopened()
	s := reopened.(*sqliteStore)
	for table, want := range map[string]int{"messages": messages, "deliveries": messages * 2, "channel_messages": messages * 2} {
		var got int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("%s contains %d rows, want %d", table, got, want)
		}
	}
	if err := s.verifyPragmas(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteRejectsIncompleteHistory(t *testing.T) {
	store, closeStore, err := NewSQLite(filepath.Join(t.TempDir(), "validation.db"), SQLiteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	if err := store.Record(context.Background(), Message{}, nil); err == nil {
		t.Fatal("history without deliveries was accepted")
	}
	if err := store.Record(context.Background(), Message{}, []Delivery{{Status: "success"}}); err == nil {
		t.Fatal("delivery without a channel name was accepted")
	}
}
