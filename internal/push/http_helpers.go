package push

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseBody = 4096

func doJSONRequest(ctx context.Context, method, rawURL string, payload any, headers map[string]string) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

func doFormRequest(ctx context.Context, rawURL string, values url.Values, headers map[string]string) (int, []byte, error) {
	encoded := values.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(encoded))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

func doGetRequest(ctx context.Context, rawURL string, params url.Values, headers map[string]string) (int, []byte, error) {
	full, err := addQueryParams(rawURL, params)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return 0, nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

func doRequest(req *http.Request) (int, []byte, error) {
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	return resp.StatusCode, body, nil
}

func addQueryParams(rawURL string, params url.Values) (string, error) {
	if len(params) == 0 {
		return rawURL, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL, err
	}
	q := u.Query()
	for k, vals := range params {
		for _, v := range vals {
			q.Add(k, v)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
