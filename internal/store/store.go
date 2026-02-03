package store

import (
	"context"
	"time"
)

type Message struct {
	ID         int64
	CreatedAt  time.Time
	RemoteAddr string
	Path       string
	Format     string
	Chan       string
	Title      string
	Content    string
}

type Delivery struct {
	MessageID   int64
	CreatedAt   time.Time
	ChannelName string
	ChannelType string
	Status      string
	Detail      string
}

type Store interface {
	Record(ctx context.Context, msg Message, deliveries []Delivery) error
	Close() error
}

type BanRecord struct {
	Kind        string
	IP          string
	FailCount   int
	BannedUntil time.Time
	LastSeen    time.Time
}

type CleanupResult struct {
	MessagesDeleted int64
	BansDeleted     int64
	Before          time.Time
}

type NotificationRecord struct {
	ID          int64     `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	RemoteAddr  string    `json:"remote_addr"`
	ChannelName string    `json:"channel_name"`
	ChannelType string    `json:"channel_type"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail"`
}

type NotificationPage struct {
	Items    []NotificationRecord `json:"items"`
	Page     int                  `json:"page"`
	PageSize int                  `json:"page_size"`
	Total    int                  `json:"total"`
}

type StoreSummary struct {
	NotificationTotal int64     `json:"notification_total"`
	LastSentAt        time.Time `json:"last_sent_at"`
	TodaySent         int64     `json:"today_sent"`
	TodayFailed       int64     `json:"today_failed"`
}

type TrendPoint struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type BanTrendStats struct {
	Last24h []TrendPoint `json:"last_24h"`
	Last7d  []TrendPoint `json:"last_7d"`
}

type BanStore interface {
	UpsertBan(ctx context.Context, rec BanRecord) error
	DeleteBan(ctx context.Context, kind, ip string) error
	CleanupBans(ctx context.Context, before time.Time) (int64, error)
}

type MaintenanceStore interface {
	Compact(ctx context.Context) error
	Cleanup(ctx context.Context, before time.Time) (CleanupResult, error)
}

type BanTrendStore interface {
	BanTrends(ctx context.Context) (BanTrendStats, error)
}

type NotificationStore interface {
	ListChannelMessages(ctx context.Context, page, pageSize int) (NotificationPage, error)
}

type SummaryStore interface {
	Summary(ctx context.Context) (StoreSummary, error)
}
