package push

import (
	"context"
	"encoding/json"
)

type stubProvider struct{}

func NewStubProvider() Provider { return stubProvider{} }

func (stubProvider) Type() string { return "stub" }

func (stubProvider) Send(_ context.Context, req SendRequest) (SendResult, error) {
	b, _ := json.Marshal(map[string]any{
		"type":    req.ChannelType,
		"channel": req.ChannelName,
		"title":   req.Title,
		"content": req.Content,
	})
	return SendResult{Status: "success", Detail: string(b)}, nil
}

