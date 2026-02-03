package store

import (
	"context"
	"time"
)

type noopStore struct{}

func NewNoop() Store { return noopStore{} }

func (noopStore) Record(_ context.Context, _ Message, _ []Delivery) error { return nil }

func (noopStore) Close() error { return nil }

func (noopStore) UpsertBan(_ context.Context, _ BanRecord) error { return nil }

func (noopStore) DeleteBan(_ context.Context, _, _ string) error { return nil }

func (noopStore) CleanupBans(_ context.Context, _ time.Time) (int64, error) { return 0, nil }

func (noopStore) Compact(_ context.Context) error { return nil }

func (noopStore) Cleanup(_ context.Context, before time.Time) (CleanupResult, error) {
	return CleanupResult{Before: before}, nil
}

func (noopStore) BanTrends(_ context.Context) (BanTrendStats, error) {
	return BanTrendStats{}, nil
}

func (noopStore) ListChannelMessages(_ context.Context, page, pageSize int) (NotificationPage, error) {
	return NotificationPage{
		Page:     page,
		PageSize: pageSize,
		Total:    0,
		Items:    []NotificationRecord{},
	}, nil
}
