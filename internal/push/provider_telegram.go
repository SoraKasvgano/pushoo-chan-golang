package push

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// Telegram provider (compatible with pushoo):
// - token format: "{botToken}#{chatID}"
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

	text := escapeTelegramMarkdown(req.Content)
	if req.Title != "" {
		text = req.Title + "\n\n" + text
	}

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	u := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	status, body, err := doJSONRequest(ctx, http.MethodPost, u, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status >= 200 && status < 300 {
		return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
	}
	return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
}
