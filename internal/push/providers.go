package push

import (
	"context"
	"strings"
)

type Provider interface {
	// Type returns config "type" value (e.g. "telegram", "bark").
	Type() string
	Send(ctx context.Context, req SendRequest) (SendResult, error)
}

type SendRequest struct {
	ChannelName string
	ChannelType string
	Token       string
	Title       string
	Content     string
}

type SendResult struct {
	Status string // "success" | "error"
	Detail string
}

type ProviderRegistry struct {
	m map[string]Provider
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{m: map[string]Provider{}}
}

func (r *ProviderRegistry) Register(p Provider) {
	r.m[strings.ToLower(p.Type())] = p
}

func (r *ProviderRegistry) Get(typ string) (Provider, bool) {
	p, ok := r.m[strings.ToLower(typ)]
	return p, ok
}
