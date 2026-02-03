package store

import "context"

type noopStore struct{}

func NewNoop() Store { return noopStore{} }

func (noopStore) Record(_ context.Context, _ Message, _ []Delivery) error { return nil }

func (noopStore) Close() error { return nil }

