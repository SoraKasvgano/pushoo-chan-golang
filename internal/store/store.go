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

