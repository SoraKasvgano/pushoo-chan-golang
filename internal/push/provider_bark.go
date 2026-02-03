package push

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Bark provider:
// - token is a base URL like "https://api.day.app/DEVICE_KEY/" (the original config example).
// - we send GET to {token}/{title}/{body} or {token}/{body}.
type barkProvider struct{}

func NewBarkProvider() Provider { return barkProvider{} }

func (barkProvider) Type() string { return "bark" }

func (barkProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	base := strings.TrimSpace(req.Token)
	if base == "" {
		return SendResult{Status: "error", Detail: "empty bark token"}, nil
	}
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		return SendResult{Status: "error", Detail: "bark token must be a URL"}, nil
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	var full string
	if req.Title != "" {
		full = base + url.PathEscape(req.Title) + "/" + url.PathEscape(req.Content)
	} else {
		full = base + url.PathEscape(req.Content)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
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

