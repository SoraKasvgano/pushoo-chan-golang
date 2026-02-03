package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// webhook provider (extension):
// - type: "webhook"
// - token: full URL
// - sends POST JSON {title, content, channel}
type webhookProvider struct{}

func NewWebhookProvider() Provider { return webhookProvider{} }

func (webhookProvider) Type() string { return "webhook" }

func (webhookProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	u := strings.TrimSpace(req.Token)
	if u == "" {
		return SendResult{Status: "error", Detail: "empty webhook url"}, nil
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return SendResult{Status: "error", Detail: "webhook token must be a URL"}, nil
	}
	payload := map[string]any{
		"title":   req.Title,
		"content": req.Content,
		"channel": req.ChannelName,
	}
	b, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	httpReq.Header.Set("content-type", "application/json; charset=utf-8")
	resp, err := sharedHTTPClient.Do(httpReq)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return SendResult{Status: "success", Detail: fmt.Sprintf("%s %s", resp.Status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}, nil
}

