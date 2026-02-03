package httpapi

import (
	"bytes"
	"context"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pushoo-chan-gover/internal/config"
	ienc "pushoo-chan-gover/internal/encoding"
	"pushoo-chan-gover/internal/push"
	"pushoo-chan-gover/internal/store"
)

type Options struct {
	Config       *config.Manager
	Push         *push.Service
	Store        store.Store
	EventHub     *EventHub
	FrontendDir  string
	EmbeddedFS   embed.FS
}

type API struct {
	opts Options

	frontend *FrontendHandler
}

func New(opts Options) *API {
	feDir := opts.FrontendDir
	if feDir == "" {
		feDir = "frontend"
	}
	opts.FrontendDir = feDir

	return &API{
		opts:     opts,
		frontend: NewFrontendHandler(feDir, opts.EmbeddedFS),
	}
}

func (a *API) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Public endpoints that don't require authentication
		isPublicEndpoint := false

		// Check if it's a public API endpoint
		switch {
		case r.URL.Path == "/api/health":
			isPublicEndpoint = true
		case strings.HasPrefix(r.URL.Path, "/send"):
			isPublicEndpoint = true
		case strings.HasPrefix(r.URL.Path, "/barkv2"):
			isPublicEndpoint = true
		case strings.HasPrefix(r.URL.Path, "/bark"):
			isPublicEndpoint = true
		}

		// Require authentication for web interface and config endpoints
		if !isPublicEndpoint {
			if err := a.verifyBasicAuth(w, r); err != nil {
				return
			}
		}

		// Route to handlers
		switch {
		case r.URL.Path == "/api/health":
			a.handleHealth(w, r)
		case r.URL.Path == "/api/events":
			a.opts.EventHub.ServeSSE(w, r)
		case strings.HasPrefix(r.URL.Path, "/send"):
			a.handleSend(w, r)
		case strings.HasPrefix(r.URL.Path, "/barkv2"):
			a.handleBarkV2(w, r)
		case strings.HasPrefix(r.URL.Path, "/bark"):
			a.handleBark(w, r)
		case strings.HasPrefix(r.URL.Path, "/config/"):
			a.handleConfig(w, r)
		default:
			a.serveFrontend(w, r)
		}
	})
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(map[string]any{"status": "ok"}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (a *API) serveFrontend(w http.ResponseWriter, r *http.Request) {
	a.frontend.ServeHTTP(w, r)
}

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	// Authentication is already handled by the global middleware
	switch r.URL.Path {
	case "/config/download":
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		raw := a.opts.Config.GetRaw()
		w.Header().Set("content-type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, raw)
	case "/config/upload":
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if err != nil {
			http.Error(w, "read body failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := a.opts.Config.SetRaw(r.Context(), string(body)); err != nil {
			http.Error(w, "write config failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(map[string]any{"status": "ok", "message": "Configuration updated successfully"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	default:
		http.NotFound(w, r)
	}
}

func (a *API) verifyBasicAuth(w http.ResponseWriter, r *http.Request) error {
	cfg := a.opts.Config.Get()
	user := strings.TrimSpace(cfg.Auth.User)
	pass := strings.TrimSpace(cfg.Auth.Pass)

	// If auth is not configured, deny access to web interface
	if user == "" || pass == "" {
		w.Header().Set("WWW-Authenticate", `Basic realm="pushoo-chan Web Interface", charset="UTF-8"`)
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.Error(w, "Authentication required. Please configure auth.user and auth.pass in config.yaml", http.StatusUnauthorized)
		log.Printf("[auth] Access denied: authentication not configured (from %s)", r.RemoteAddr)
		return errors.New("authentication not configured")
	}

	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Basic ") {
		w.Header().Set("WWW-Authenticate", `Basic realm="pushoo-chan Web Interface", charset="UTF-8"`)
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		log.Printf("[auth] Access denied: no credentials provided (from %s)", r.RemoteAddr)
		return errors.New("unauthorized")
	}

	dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="pushoo-chan Web Interface", charset="UTF-8"`)
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.Error(w, "Invalid authentication format", http.StatusUnauthorized)
		log.Printf("[auth] Access denied: invalid auth format (from %s)", r.RemoteAddr)
		return errors.New("unauthorized")
	}

	u, p, _ := strings.Cut(string(dec), ":")

	// Security: Use constant-time comparison to prevent timing attacks
	userMatch := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(p), []byte(pass)) == 1

	if !userMatch || !passMatch {
		w.Header().Set("WWW-Authenticate", `Basic realm="pushoo-chan Web Interface", charset="UTF-8"`)
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		log.Printf("[auth] Access denied: invalid credentials for user '%s' (from %s)", u, r.RemoteAddr)
		return errors.New("unauthorized")
	}

	log.Printf("[auth] Access granted: user '%s' (from %s to %s)", u, r.RemoteAddr, r.URL.Path)
	return nil
}

type reqContext struct {
	logs []string
}

func (rc *reqContext) log(msg string) {
	rc.logs = append(rc.logs, msg)
}

// Security: Whitelist of allowed charsets to prevent injection attacks
var allowedCharsets = map[string]bool{
	"utf-8":     true,
	"utf8":      true,
	"gbk":       true,
	"gb2312":    true,
	"gb18030":   true,
	"big5":      true,
	"iso-8859-1": true,
	"windows-1252": true,
}

func isValidCharset(charset string) bool {
	if charset == "" {
		return true // empty is valid (defaults to utf-8)
	}
	return allowedCharsets[strings.ToLower(charset)]
}

func (a *API) handleSend(w http.ResponseWriter, r *http.Request) {
	rc := &reqContext{}
	title, content, chanStr, charset, hasTitle, hasContent := parseCommonParams(rc, r)

	// Match TS behavior: if desp/content is absent, send title only as content.
	if !hasContent && hasTitle {
		content = title
		title = ""
	}

	chanList := splitChan(chanStr)
	if len(chanList) == 0 {
		rc.log("Warning: no channel specified, using default channel!")
	}

	results, err := a.opts.Push.Push(r.Context(), chanList, title, content, rc.log)
	if err != nil {
		rc.log("send failed: " + err.Error())
	}

	resp := map[string]any{}
	if len(results) > 0 {
		// Keep existing style field name.
		flat := make([]string, 0, len(results))
		for _, r := range results {
			flat = append(flat, r.Message)
		}
		resp["results"] = flat
	}
	if len(rc.logs) > 0 {
		resp["msg"] = rc.logs
	}

	status := http.StatusOK
	if len(results) == 0 {
		status = http.StatusInternalServerError
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.Header().Set("x-pushoo-charset", charset)
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	if err := enc.Encode(resp); err != nil {
		// Log error but don't try to write another response
		rc.log("Failed to encode response: " + err.Error())
	}

	// Store push history (optional).
	if a.opts.Store != nil && len(results) > 0 {
		msg := store.Message{
			CreatedAt:  time.Now(),
			RemoteAddr: r.RemoteAddr,
			Path:       r.URL.Path,
			Format:     "send",
			Chan:       chanStr,
			Title:      title,
			Content:    content,
		}
		dels := make([]store.Delivery, 0, len(results))
		for _, dr := range results {
			dels = append(dels, store.Delivery{
				CreatedAt:   time.Now(),
				ChannelName: dr.ChannelName,
				ChannelType: dr.ChannelType,
				Status:      dr.Status,
				Detail:      dr.Message,
			})
		}
		if err := a.opts.Store.Record(context.Background(), msg, dels); err != nil {
			rc.log("Failed to store push history: " + err.Error())
		}
	}
}

func (a *API) handleBark(w http.ResponseWriter, r *http.Request) {
	rc := &reqContext{}
	// Bark URL format: /bark/{chan}/{content} or /bark/{chan}/{title}/{content}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// parts[0] == "bark"
	chanName := ""
	title := ""
	content := ""
	if len(parts) >= 2 {
		chanName, _ = url.PathUnescape(parts[1])
	}
	if len(parts) == 3 {
		content, _ = url.PathUnescape(parts[2])
	}
	if len(parts) >= 4 {
		title, _ = url.PathUnescape(parts[2])
		content, _ = url.PathUnescape(parts[3])
	}

	// Bark POST body supports form/json: title/body.
	bodyMap, charset := parseBody(rc, r)
	if v := bodyMap["body"]; v != "" {
		content = v
	}
	if v := bodyMap["title"]; v != "" {
		title = v
	}

	results, err := a.opts.Push.Push(r.Context(), []string{chanName}, title, content, rc.log)
	if err != nil {
		rc.log("bark failed: " + err.Error())
	}

	out := map[string]any{
		"code":      200,
		"message":   "success",
		"timestamp": time.Now().Unix(),
	}
	if len(results) > 0 {
		flat := make([]string, 0, len(results))
		for _, r := range results {
			flat = append(flat, r.Message)
		}
		out["results"] = flat
	}
	if len(rc.logs) > 0 {
		out["msg"] = rc.logs
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.Header().Set("x-pushoo-charset", charset)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	if err := enc.Encode(out); err != nil {
		rc.log("Failed to encode response: " + err.Error())
	}
}

func (a *API) handleBarkV2(w http.ResponseWriter, r *http.Request) {
	rc := &reqContext{}
	bodyMap, charset := parseBody(rc, r)
	chanName := bodyMap["device_key"]
	title := bodyMap["title"]
	content := bodyMap["body"]

	results, err := a.opts.Push.Push(r.Context(), []string{chanName}, title, content, rc.log)
	if err != nil {
		rc.log("barkv2 failed: " + err.Error())
	}

	out := map[string]any{
		"code":      200,
		"message":   "success",
		"timestamp": time.Now().Unix(),
	}
	if len(results) > 0 {
		flat := make([]string, 0, len(results))
		for _, r := range results {
			flat = append(flat, r.Message)
		}
		out["results"] = flat
	}
	if len(rc.logs) > 0 {
		out["msg"] = rc.logs
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.Header().Set("x-pushoo-charset", charset)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	if err := enc.Encode(out); err != nil {
		rc.log("Failed to encode response: " + err.Error())
	}
}

func splitChan(chanStr string) []string {
	items := strings.Split(chanStr, ",")
	out := make([]string, 0, len(items))
	for _, s := range items {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseCommonParams(rc *reqContext, r *http.Request) (title, content, chanStr, charset string, hasTitle, hasContent bool) {
	qs := r.URL.Query()
	charset = qs.Get("charset")

	// Security: Validate charset against whitelist
	if charset != "" && !isValidCharset(charset) {
		rc.log("Warning: invalid charset '" + charset + "', using utf-8")
		charset = "utf-8"
	}

	qsOverride := url.Values{}
	if charset != "" {
		if v, err := ienc.ParseQueryWithCharset(r.URL.RawQuery, charset); err == nil {
			qsOverride = v
		} else {
			rc.log("parseQuery: failed to decode query using charset=" + charset + ": " + err.Error())
		}
	}
	bodyMap, bodyCharset := parseBody(rc, r)
	if charset == "" {
		charset = bodyCharset
	}

	// TS precedence: body -> query_ (override) -> query (default)
	get := func(k string) (string, bool) {
		if v, ok := bodyMap[k]; ok {
			return strings.TrimSpace(v), true
		}
		if qsOverride.Has(k) {
			return strings.TrimSpace(qsOverride.Get(k)), true
		}
		if qs.Has(k) {
			return strings.TrimSpace(qs.Get(k)), true
		}
		return "", false
	}
	title, hasTitle = get("text")
	content, hasContent = get("desp")
	chanStr, _ = get("chan")
	return title, content, chanStr, charset, hasTitle, hasContent
}

func parseBody(rc *reqContext, r *http.Request) (map[string]string, string) {
	out := map[string]string{}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return out, ""
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		rc.log("read body failed: " + err.Error())
		return out, ""
	}
	if len(b) == 0 {
		return out, ""
	}

	mediaType, params, _ := mime.ParseMediaType(r.Header.Get("content-type"))
	charset := params["charset"]
	if charset == "" {
		// allow query override later
		charset = ienc.ExtractCharsetFromBytes(b)
	}
	if charset == "" {
		charset = "utf-8"
	}

	// Security: Validate charset against whitelist
	if !isValidCharset(charset) {
		rc.log("Warning: invalid charset '" + charset + "', using utf-8")
		charset = "utf-8"
	}

	// Decode for JSON. For forms, we decode per-field to better handle percent-encoded bytes.
	trim := bytes.TrimSpace(b)
	if mediaType == "application/json" || (len(trim) > 0 && trim[0] == '{') {
		utf8b, err := ienc.DecodeBytes(b, charset)
		if err != nil {
			rc.log("decodeBody: charset " + charset + " not available; fallback to raw bytes")
			utf8b = b
		}
		var m map[string]any
		if err := json.Unmarshal(utf8b, &m); err != nil {
			rc.log("parseBody: json parse failed: " + err.Error())
			return out, charset
		}
		for k, v := range m {
			out[k] = fmt.Sprint(v)
		}
		return out, charset
	}

	// Default to x-www-form-urlencoded parser.
	values, err := ienc.ParseQueryWithCharset(string(b), charset)
	if err != nil {
		rc.log("parseBody: form parse failed: " + err.Error())
		return out, charset
	}
	for k := range values {
		out[k] = values.Get(k)
	}
	return out, charset
}
