package push

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Bark provider:
// - token is a base URL like "https://api.day.app/DEVICE_KEY/" (compatible with pushoo).
// - we send GET to {token}/{title}/{body}/ with title fallback to first line.
type barkProvider struct{}

func NewBarkProvider() Provider { return barkProvider{} }

func (barkProvider) Type() string { return "bark" }

func (barkProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	base := strings.TrimSpace(req.Token)
	if base == "" {
		return SendResult{Status: "error", Detail: "empty bark token"}, nil
	}
	lower := strings.ToLower(base)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		base = "https://api.day.app/" + base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	title := req.Title
	if title == "" {
		title = getTitle(req.Content)
	}
	content := markdownToText(req.Content)
	full := base + url.PathEscape(title) + "/" + url.PathEscape(content) + "/"

	status, body, err := doGetRequest(ctx, full, nil, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status >= 200 && status < 300 {
		return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
	}
	return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
}
