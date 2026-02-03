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

// Telegram provider:
// - token format follows the existing config example: "{botToken}#{chatID}"
// - sendMessage API: POST https://api.telegram.org/bot{botToken}/sendMessage
type telegramProvider struct{}

func NewTelegramProvider() Provider { return telegramProvider{} }

func (telegramProvider) Type() string { return "telegram" }

func (telegramProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty telegram token"}, nil
	}
	botToken, chatID, ok := strings.Cut(token, "#")
	if !ok || strings.TrimSpace(botToken) == "" || strings.TrimSpace(chatID) == "" {
		return SendResult{Status: "error", Detail: "telegram token must be '{botToken}#{chatID}'"}, nil
	}

	text := req.Content
	if req.Title != "" {
		if text != "" {
			text = req.Title + "\n" + text
		} else {
			text = req.Title
		}
	}

	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	b, _ := json.Marshal(payload)
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
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

