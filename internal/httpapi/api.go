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
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	"pushoo-chan-gover/internal/config"
	ienc "pushoo-chan-gover/internal/encoding"
	"pushoo-chan-gover/internal/push"
	"pushoo-chan-gover/internal/store"
)

type Options struct {
	Config      *config.Manager
	Push        *push.Service
	Store       store.Store
	EventHub    *EventHub
	FrontendDir string
	EmbeddedFS  embed.FS
}

type API struct {
	opts Options

	frontend  *FrontendHandler
	authBans  *IPBanStore
	tokenBans *IPBanStore
}

type banStoreAdapter struct {
	store store.BanStore
}

func (a banStoreAdapter) UpsertBan(ctx context.Context, kind, ip string, failCount int, bannedUntil, lastSeen time.Time) error {
	return a.store.UpsertBan(ctx, store.BanRecord{
		Kind:        kind,
		IP:          ip,
		FailCount:   failCount,
		BannedUntil: bannedUntil,
		LastSeen:    lastSeen,
	})
}

func (a banStoreAdapter) DeleteBan(ctx context.Context, kind, ip string) error {
	return a.store.DeleteBan(ctx, kind, ip)
}

func New(opts Options) *API {
	feDir := opts.FrontendDir
	if feDir == "" {
		feDir = "frontend"
	}
	opts.FrontendDir = feDir
	cfg := opts.Config.Get()
	var banPersistor BanPersistor
	if bs, ok := opts.Store.(store.BanStore); ok {
		banPersistor = banStoreAdapter{store: bs}
	}
	banOpts := IPBanOptions{
		MaxEntries:      cfg.Security.IPBanMaxEntries,
		CleanupInterval: time.Duration(cfg.Security.IPBanCleanupSeconds) * time.Second,
		IdleTTL:         time.Duration(cfg.Security.IPBanIdleMinutes) * time.Minute,
		Persistor:       banPersistor,
	}

	return &API{
		opts:      opts,
		frontend:  NewFrontendHandler(feDir, opts.EmbeddedFS),
		authBans:  NewIPBanStore(mergeBanOpts(banOpts, "auth")),
		tokenBans: NewIPBanStore(mergeBanOpts(banOpts, "token")),
	}
}

func mergeBanOpts(opts IPBanOptions, kind string) IPBanOptions {
	opts.Kind = kind
	return opts
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
		case r.URL.Path == "/webhook/tawk" || strings.HasPrefix(r.URL.Path, "/webhook/tawk/"):
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
		case r.URL.Path == "/api/security/ban_stats":
			a.handleBanStats(w, r)
		case r.URL.Path == "/api/security/ban_trends":
			a.handleBanTrends(w, r)
		case r.URL.Path == "/api/store/compact":
			a.handleStoreCompact(w, r)
		case r.URL.Path == "/api/store/cleanup":
			a.handleStoreCleanup(w, r)
		case r.URL.Path == "/api/store/notifications":
			a.handleStoreNotifications(w, r)
		case r.URL.Path == "/api/store/summary":
			a.handleStoreSummary(w, r)
		case r.URL.Path == "/api/events":
			a.opts.EventHub.ServeSSE(w, r)
		case strings.HasPrefix(r.URL.Path, "/send"):
			a.handleSend(w, r)
		case strings.HasPrefix(r.URL.Path, "/barkv2"):
			a.handleBarkV2(w, r)
		case strings.HasPrefix(r.URL.Path, "/bark"):
			a.handleBark(w, r)
		case r.URL.Path == "/webhook/tawk" || strings.HasPrefix(r.URL.Path, "/webhook/tawk/"):
			a.handleTawkWebhook(w, r)
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

func (a *API) handleBanStats(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	stats := map[string]any{
		"auth":  a.authBans.Stats(limit),
		"token": a.tokenBans.Stats(limit),
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (a *API) handleStoreCompact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	ms, ok := a.opts.Store.(store.MaintenanceStore)
	if !ok {
		http.Error(w, "SQLite store not enabled", http.StatusBadRequest)
		return
	}
	if err := ms.Compact(r.Context()); err != nil {
		http.Error(w, "compact failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (a *API) handleStoreCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	ms, ok := a.opts.Store.(store.MaintenanceStore)
	if !ok {
		http.Error(w, "SQLite store not enabled", http.StatusBadRequest)
		return
	}
	keepDays := 30
	if v := r.URL.Query().Get("keep_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			keepDays = n
		}
	}
	before := time.Now().Add(-time.Duration(keepDays) * 24 * time.Hour)
	res, err := ms.Cleanup(r.Context(), before)
	if err != nil {
		http.Error(w, "cleanup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"result": res,
	})
}

func (a *API) handleBanTrends(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	ts, ok := a.opts.Store.(store.BanTrendStore)
	if !ok {
		http.Error(w, "SQLite store not enabled", http.StatusBadRequest)
		return
	}
	stats, err := ts.BanTrends(r.Context())
	if err != nil {
		http.Error(w, "trend query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(stats)
}

func (a *API) handleStoreNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	ns, ok := a.opts.Store.(store.NotificationStore)
	if !ok {
		http.Error(w, "SQLite store not enabled", http.StatusBadRequest)
		return
	}
	page := 1
	pageSize := 10
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			pageSize = n
		}
	}
	data, err := ns.ListChannelMessages(r.Context(), page, pageSize)
	if err != nil {
		http.Error(w, "list failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(data)
}

func (a *API) handleStoreSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	ss, ok := a.opts.Store.(store.SummaryStore)
	if !ok {
		http.Error(w, "SQLite store not enabled", http.StatusBadRequest)
		return
	}
	data, err := ss.Summary(r.Context())
	if err != nil {
		http.Error(w, "summary failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("content-type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(data)
}

func (a *API) serveFrontend(w http.ResponseWriter, r *http.Request) {
	a.frontend.ServeHTTP(w, r)
}

func (a *API) handleConfig(w http.ResponseWriter, r *http.Request) {
	// Authentication is already handled by the global middleware
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/config/download":
		if r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		raw := a.opts.Config.GetRaw()
		w.Header().Set("X-Config-Revision", a.opts.Config.Revision())
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
		if err := a.opts.Config.SetRawIfRevision(r.Context(), string(body), r.Header.Get("If-Match")); err != nil {
			if errors.Is(err, config.ErrConfigConflict) {
				http.Error(w, "configuration changed; reload before saving", http.StatusConflict)
				return
			}
			http.Error(w, "invalid configuration: "+err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("X-Config-Revision", a.opts.Config.Revision())
		w.Header().Set("content-type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(map[string]any{"status": "ok", "message": "Configuration updated successfully"}); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	case "/config/data":
		if r.Method == http.MethodGet {
			w.Header().Set("X-Config-Revision", a.opts.Config.Revision())
			w.Header().Set("content-type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(a.opts.Config.Get())
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		var cfg config.Config
		dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			http.Error(w, "invalid configuration: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := config.Validate(cfg); err != nil {
			http.Error(w, "invalid configuration: "+err.Error(), http.StatusBadRequest)
			return
		}
		raw, err := yaml.Marshal(&cfg)
		if err != nil {
			http.Error(w, "encode configuration failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := a.opts.Config.SetRawIfRevision(r.Context(), string(raw), r.Header.Get("If-Match")); err != nil {
			if errors.Is(err, config.ErrConfigConflict) {
				http.Error(w, "configuration changed; reload before saving", http.StatusConflict)
				return
			}
			http.Error(w, "write config failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Config-Revision", a.opts.Config.Revision())
		w.Header().Set("content-type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	default:
		http.NotFound(w, r)
	}
}

func (a *API) verifyBasicAuth(w http.ResponseWriter, r *http.Request) error {
	cfg := a.opts.Config.Get()
	user := strings.TrimSpace(cfg.Auth.User)
	pass := strings.TrimSpace(cfg.Auth.Pass)
	ip := getClientIP(r)

	// Brute-force protection: check IP ban status
	if banned, until := a.authBans.IsBanned(ip); banned {
		retryAfter := time.Until(until)
		if retryAfter < 0 {
			retryAfter = 0
		}
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
		http.Error(w, "Too many failed attempts. Please try again later.", http.StatusTooManyRequests)
		log.Printf("[auth] IP banned: %s until %s", ip, until.Format(time.RFC3339))
		return errors.New("ip banned")
	}

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
		log.Printf("[auth] Access denied: no credentials provided (from %s)", ip)
		return errors.New("unauthorized")
	}

	dec, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Basic realm="pushoo-chan Web Interface", charset="UTF-8"`)
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.Error(w, "Invalid authentication format", http.StatusUnauthorized)
		log.Printf("[auth] Access denied: invalid auth format (from %s)", ip)
		return errors.New("unauthorized")
	}

	u, p, _ := strings.Cut(string(dec), ":")

	// Security: Use constant-time comparison to prevent timing attacks
	userMatch := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(p), []byte(pass)) == 1

	if !userMatch || !passMatch {
		// Record failure and possibly ban
		banned, until, failCount := a.authBans.RecordFailure(ip, cfg.Security.AuthFailLimit, cfg.Security.AuthBanMinutes)
		if banned {
			retryAfter := time.Until(until)
			if retryAfter < 0 {
				retryAfter = 0
			}
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
			http.Error(w, "Too many failed attempts. Please try again later.", http.StatusTooManyRequests)
			log.Printf("[auth] IP banned after failures: %s until %s", ip, until.Format(time.RFC3339))
			return errors.New("ip banned")
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="pushoo-chan Web Interface", charset="UTF-8"`)
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		log.Printf("[auth] Access denied: invalid credentials for user '%s' (from %s, fail=%d)", u, ip, failCount)
		return errors.New("unauthorized")
	}

	// Success: clear any previous failures for this IP
	a.authBans.Reset(ip)
	log.Printf("[auth] Access granted: user '%s' (from %s to %s)", u, ip, r.URL.Path)
	return nil
}

// verifyPushToken verifies the push token if push_token.enabled is true
func (a *API) verifyPushToken(r *http.Request) error {
	cfg := a.opts.Config.Get()
	ip := getClientIP(r)

	// Brute-force protection: check IP ban status for token failures
	if banned, until := a.tokenBans.IsBanned(ip); banned {
		retryAfter := time.Until(until)
		if retryAfter < 0 {
			retryAfter = 0
		}
		return &BanError{
			Message:    "Too many failed token attempts. Please try again later.",
			RetryAfter: retryAfter,
		}
	}

	// If push token is not enabled, allow access
	if !cfg.PushToken.Enabled {
		return nil
	}

	// Get token from query parameter or form data
	token := r.URL.Query().Get("token")
	if token == "" && r.Method == http.MethodPost {
		// Try to get from form data
		if err := r.ParseForm(); err == nil {
			token = r.Form.Get("token")
		}
	}
	if token == "" && r.Method == http.MethodPost {
		// Try to get from JSON body (without consuming it)
		if bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 2<<20)); err == nil {
			// Restore body for downstream handlers
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))

			if len(bodyBytes) > 0 {
				mediaType, params, _ := mime.ParseMediaType(r.Header.Get("content-type"))
				charset := params["charset"]
				if charset == "" {
					charset = ienc.ExtractCharsetFromBytes(bodyBytes)
				}
				if charset == "" {
					charset = "utf-8"
				}

				if !isValidCharset(charset) {
					charset = "utf-8"
				}

				trim := bytes.TrimSpace(bodyBytes)
				if mediaType == "application/json" || (len(trim) > 0 && trim[0] == '{') {
					utf8b, err := ienc.DecodeBytes(bodyBytes, charset)
					if err != nil {
						utf8b = bodyBytes
					}
					var m map[string]any
					if err := json.Unmarshal(utf8b, &m); err == nil {
						if v, ok := m["token"]; ok {
							token = strings.TrimSpace(fmt.Sprint(v))
						}
					}
				}
			}
		}
	}

	// Check if token is provided
	if token == "" {
		banned, until, failCount := a.tokenBans.RecordFailure(ip, cfg.Security.TokenFailLimit, cfg.Security.TokenBanMinutes)
		if banned {
			return &BanError{
				Message:    "Too many failed token attempts. Please try again later.",
				RetryAfter: time.Until(until),
			}
		}
		log.Printf("[push] Missing token (from %s, fail=%d)", ip, failCount)
		return fmt.Errorf("push token is required but not provided")
	}

	// Verify token using constant-time comparison to prevent timing attacks
	expectedToken := strings.TrimSpace(cfg.PushToken.Token)
	if expectedToken == "" {
		return fmt.Errorf("push token is enabled but not configured")
	}

	if subtle.ConstantTimeCompare([]byte(token), []byte(expectedToken)) != 1 {
		banned, until, failCount := a.tokenBans.RecordFailure(ip, cfg.Security.TokenFailLimit, cfg.Security.TokenBanMinutes)
		if banned {
			return &BanError{
				Message:    "Too many failed token attempts. Please try again later.",
				RetryAfter: time.Until(until),
			}
		}
		log.Printf("[push] Invalid token (from %s, fail=%d)", ip, failCount)
		return fmt.Errorf("invalid push token")
	}

	// Success: clear any previous failures for this IP
	a.tokenBans.Reset(ip)
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
	"utf-8":        true,
	"utf8":         true,
	"gbk":          true,
	"gb2312":       true,
	"gb18030":      true,
	"big5":         true,
	"iso-8859-1":   true,
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

	// Verify push token if enabled
	if err := a.verifyPushToken(r); err != nil {
		if banErr, ok := err.(*BanError); ok {
			if banErr.RetryAfter > 0 {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(banErr.RetryAfter.Seconds())))
			}
			w.Header().Set("content-type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"error": banErr.Message,
				"msg":   []string{banErr.Message},
			})
			log.Printf("[push] Token IP banned from %s: %v", r.RemoteAddr, banErr.Message)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "Invalid or missing push token",
			"msg":   []string{err.Error()},
		})
		log.Printf("[push] Token verification failed from %s: %v", r.RemoteAddr, err)
		return
	}

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

	// Verify push token if enabled
	if err := a.verifyPushToken(r); err != nil {
		if banErr, ok := err.(*BanError); ok {
			if banErr.RetryAfter > 0 {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(banErr.RetryAfter.Seconds())))
			}
			w.Header().Set("content-type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"code":    429,
				"message": banErr.Message,
				"msg":     []string{banErr.Message},
			})
			log.Printf("[push] Token IP banned from %s: %v", r.RemoteAddr, banErr.Message)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"code":    401,
			"message": "Invalid or missing push token",
			"msg":     []string{err.Error()},
		})
		log.Printf("[push] Token verification failed from %s: %v", r.RemoteAddr, err)
		return
	}

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

	// Verify push token if enabled
	if err := a.verifyPushToken(r); err != nil {
		if banErr, ok := err.(*BanError); ok {
			if banErr.RetryAfter > 0 {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", int(banErr.RetryAfter.Seconds())))
			}
			w.Header().Set("content-type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{
				"code":    429,
				"message": banErr.Message,
				"msg":     []string{banErr.Message},
			})
			log.Printf("[push] Token IP banned from %s: %v", r.RemoteAddr, banErr.Message)
			return
		}
		w.Header().Set("content-type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"code":    401,
			"message": "Invalid or missing push token",
			"msg":     []string{err.Error()},
		})
		log.Printf("[push] Token verification failed from %s: %v", r.RemoteAddr, err)
		return
	}

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
