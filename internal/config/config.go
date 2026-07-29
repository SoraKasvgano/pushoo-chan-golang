package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is kept compatible with the existing `config.yaml` format used by the TS version.
type Config struct {
	Channels       []ChannelConfig      `yaml:"channels" json:"channels"`
	ChannelGroups  []ChannelGroupConfig `yaml:"channel_groups,omitempty" json:"channel_groups"`
	DefaultChannel string               `yaml:"default_channel,omitempty" json:"default_channel"`
	Auth           AuthConfig           `yaml:"auth,omitempty" json:"auth"`
	PushToken      PushTokenConfig      `yaml:"push_token,omitempty" json:"push_token"`
	Security       SecurityConfig       `yaml:"security,omitempty" json:"security"`
	Webhooks       WebhookConfig        `yaml:"webhooks,omitempty" json:"webhooks"`

	// Optional extensions (ignored by older configs):
	SQLite SQLiteConfig `yaml:"sqlite,omitempty" json:"sqlite"`
}

type WebhookConfig struct {
	Tawk TawkWebhookConfig `yaml:"tawk,omitempty" json:"tawk"`
}

type TawkWebhookConfig struct {
	Secret string `yaml:"secret,omitempty" json:"secret"`
	Chan   string `yaml:"chan,omitempty" json:"chan"`
	Title  string `yaml:"title,omitempty" json:"title"`
}

type SQLiteConfig struct {
	Path                  string `yaml:"path,omitempty" json:"path"`
	CleanupDays           int    `yaml:"cleanup_days,omitempty" json:"cleanup_days"`
	CleanupIntervalHours  int    `yaml:"cleanup_interval_hours,omitempty" json:"cleanup_interval_hours"`
	RecordChannelMessages bool   `yaml:"record_channel_messages,omitempty" json:"record_channel_messages"`
}

type AuthConfig struct {
	User string `yaml:"user,omitempty" json:"user"`
	Pass string `yaml:"pass,omitempty" json:"pass"`
}

type PushTokenConfig struct {
	Enabled bool   `yaml:"enabled,omitempty" json:"enabled"`
	Token   string `yaml:"token,omitempty" json:"token"`
}

type SecurityConfig struct {
	AuthFailLimit       int `yaml:"auth_fail_limit,omitempty" json:"auth_fail_limit"`
	AuthBanMinutes      int `yaml:"auth_ban_minutes,omitempty" json:"auth_ban_minutes"`
	TokenFailLimit      int `yaml:"token_fail_limit,omitempty" json:"token_fail_limit"`
	TokenBanMinutes     int `yaml:"token_ban_minutes,omitempty" json:"token_ban_minutes"`
	IPBanMaxEntries     int `yaml:"ip_ban_max_entries,omitempty" json:"ip_ban_max_entries"`
	IPBanCleanupSeconds int `yaml:"ip_ban_cleanup_seconds,omitempty" json:"ip_ban_cleanup_seconds"`
	IPBanIdleMinutes    int `yaml:"ip_ban_idle_minutes,omitempty" json:"ip_ban_idle_minutes"`
}

type ChannelConfig struct {
	Name  string `yaml:"name" json:"name"`
	Type  string `yaml:"type" json:"type"`
	Token string `yaml:"token" json:"token"`
}

type ChannelGroupConfig struct {
	Name string   `yaml:"name" json:"name"`
	Use  []string `yaml:"use,omitempty" json:"use"`
}

const DefaultConfigYAML = `# pushoo-chan Configuration File
# This file was automatically generated on first run
# Edit this file to configure your notification channels

channels:
  # Example: Stub channel (for testing)
  - name: stub_channel
    type: stub
    token: stub

  # Example: Telegram Bot
  # - name: my_telegram
  #   type: telegram
  #   token: YOUR_BOT_TOKEN_HERE

  # Example: Bark (iOS notification)
  # - name: my_bark
  #   type: bark
  #   token: YOUR_BARK_KEY_HERE

  # Example: Webhook
  # - name: my_webhook
  #   type: webhook
  #   token: https://your-webhook-url.com/notify

channel_groups:
  # Group multiple channels together
  - name: stub_group
    use:
      - stub_channel

  # Example: Send to multiple channels at once
  # - name: all_channels
  #   use:
  #     - my_telegram
  #     - my_bark

# Default channel to use when no channel is specified
default_channel: stub_channel

# Optional: default settings for Tawk.to webhook forwarding.
# Webhook URL example:
# https://your-push-host/webhook/tawk?chan=stub_group&title=tawktochat
# webhooks:
#   tawk:
#     chan: stub_group
#     title: tawktochat
#     # Optional: set this if you enabled webhook secret in Tawk.to.
#     # secret: your_tawk_webhook_secret

# Authentication for web interface and config management (REQUIRED)
# IMPORTANT: Change these default credentials immediately!
auth:
  user: pushoo
  pass: pushoo

# Push API token protection (optional)
# When enabled, all push requests must include the token parameter
# Example: /send?token=YOUR_TOKEN&text=Hello&desp=World
push_token:
  enabled: false  # Set to true to enable token verification
  token: ""       # Will be auto-generated on first run if empty

# Brute-force protection (in-memory IP ban)
# auth_fail_limit: wrong password attempts before ban
# auth_ban_minutes: ban duration for auth failures
# token_fail_limit: wrong push token attempts before ban
# token_ban_minutes: ban duration for token failures
# ip_ban_max_entries: max IP entries kept in memory (0 = unlimited)
# ip_ban_cleanup_seconds: cleanup interval for expired/idle entries
# ip_ban_idle_minutes: remove IP entries idle for this long
security:
  auth_fail_limit: 5
  auth_ban_minutes: 10
  token_fail_limit: 10
  token_ban_minutes: 10
  ip_ban_max_entries: 10000
  ip_ban_cleanup_seconds: 60
  ip_ban_idle_minutes: 60

# SQLite database for storing push history (optional)
# sqlite:
#   path: ./data/pushoo.db
#   cleanup_days: 30
#   cleanup_interval_hours: 24
#   record_channel_messages: false
`

type Manager struct {
	path string

	mu      sync.RWMutex
	writeMu sync.Mutex
	raw     string
	parsed  Config

	// Hot reload support
	watchCancel context.CancelFunc
	lastModTime time.Time
}

func NewManager(path string) *Manager {
	return &Manager{path: path}
}

func (m *Manager) Path() string { return m.path }

func (m *Manager) GetRaw() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.raw == "" {
		return DefaultConfigYAML
	}
	return m.raw
}

func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.parsed
}

func (m *Manager) Revision() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sum := sha256.Sum256([]byte(m.raw))
	return hex.EncodeToString(sum[:])
}

func (m *Manager) Reload(_ context.Context) error {
	raw, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// First run: create default config file
			log.Printf("[config] Config file not found at %s, creating default config...", m.path)
			if err := m.createDefaultConfig(); err != nil {
				log.Printf("[config] Warning: failed to create default config file: %v", err)
				log.Printf("[config] Using in-memory default config")
			} else {
				log.Printf("[config] Default config file created successfully at %s", m.path)
			}

			// Load default config into memory
			m.mu.Lock()
			m.raw = DefaultConfigYAML
			m.parsed = Config{}
			_ = yaml.Unmarshal([]byte(DefaultConfigYAML), &m.parsed)
			m.mu.Unlock()
			return nil
		}
		return err
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return err
	}

	// Auto-generate push token if enabled but token is empty
	if cfg.PushToken.Enabled && cfg.PushToken.Token == "" {
		log.Printf("[config] Push token is enabled but empty, generating random token...")
		cfg.PushToken.Token = generateRandomToken()

		// Save the updated config with the new token
		updatedRaw, err := yaml.Marshal(&cfg)
		if err != nil {
			log.Printf("[config] Warning: failed to marshal config with new token: %v", err)
		} else {
			if err := os.WriteFile(m.path, updatedRaw, 0o644); err != nil {
				log.Printf("[config] Warning: failed to save config with new token: %v", err)
			} else {
				log.Printf("[config] Generated and saved new push token: %s", cfg.PushToken.Token)
				raw = updatedRaw
			}
		}
	}

	// Update last modification time
	if stat, err := os.Stat(m.path); err == nil {
		m.lastModTime = stat.ModTime()
	}

	m.mu.Lock()
	m.raw = string(raw)
	m.parsed = cfg
	m.mu.Unlock()

	log.Printf("[config] Configuration loaded from %s", m.path)
	return nil
}

// createDefaultConfig creates a default config.yaml file if it doesn't exist
func (m *Manager) createDefaultConfig() error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write default config
	if err := os.WriteFile(m.path, []byte(DefaultConfigYAML), 0o644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Update last modification time
	if stat, err := os.Stat(m.path); err == nil {
		m.lastModTime = stat.ModTime()
	}

	return nil
}

func (m *Manager) SetRaw(_ context.Context, raw string) error {
	return m.SetRawIfRevision(context.Background(), raw, "")
}

var ErrConfigConflict = errors.New("configuration changed since it was loaded")

func (m *Manager) SetRawIfRevision(_ context.Context, raw, expectedRevision string) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	var cfg Config
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		return fmt.Errorf("invalid YAML: %w", err)
	}
	if err := Validate(cfg); err != nil {
		return err
	}
	if expectedRevision != "" && expectedRevision != m.Revision() {
		return ErrConfigConflict
	}
	// Write atomically to reduce risk of partial writes.
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, []byte(raw), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return m.Reload(context.Background())
}

func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.Auth.User) == "" || strings.TrimSpace(cfg.Auth.Pass) == "" {
		return errors.New("auth.user and auth.pass are required")
	}
	targets := make(map[string]string, len(cfg.Channels)+len(cfg.ChannelGroups))
	for i, ch := range cfg.Channels {
		ch.Name = strings.TrimSpace(ch.Name)
		if ch.Name == "" || strings.TrimSpace(ch.Type) == "" {
			return fmt.Errorf("channel %d must have a name and type", i+1)
		}
		if _, exists := targets[ch.Name]; exists {
			return fmt.Errorf("duplicate target name %q", ch.Name)
		}
		targets[ch.Name] = "channel"
	}
	for i, group := range cfg.ChannelGroups {
		group.Name = strings.TrimSpace(group.Name)
		if group.Name == "" {
			return fmt.Errorf("channel group %d must have a name", i+1)
		}
		if _, exists := targets[group.Name]; exists {
			return fmt.Errorf("duplicate target name %q", group.Name)
		}
		targets[group.Name] = "group"
	}
	for _, group := range cfg.ChannelGroups {
		for _, name := range group.Use {
			if targets[name] != "channel" {
				return fmt.Errorf("channel group %q references unknown channel %q", group.Name, name)
			}
		}
	}
	if cfg.DefaultChannel != "" {
		if _, ok := targets[cfg.DefaultChannel]; !ok {
			return fmt.Errorf("default_channel references unknown target %q", cfg.DefaultChannel)
		}
	}
	if cfg.Webhooks.Tawk.Chan != "" {
		if _, ok := targets[cfg.Webhooks.Tawk.Chan]; !ok {
			return fmt.Errorf("webhooks.tawk.chan references unknown target %q", cfg.Webhooks.Tawk.Chan)
		}
	}
	return nil
}

// StartWatching starts watching the config file for changes and automatically reloads it.
// It uses polling instead of inotify/fsnotify for better cross-platform compatibility.
func (m *Manager) StartWatching(ctx context.Context, interval time.Duration) {
	if interval == 0 {
		interval = 3 * time.Second // Default check interval
	}

	watchCtx, cancel := context.WithCancel(ctx)
	m.watchCancel = cancel

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		log.Printf("[config] Started watching config file: %s (interval: %v)", m.path, interval)

		for {
			select {
			case <-watchCtx.Done():
				log.Printf("[config] Stopped watching config file")
				return
			case <-ticker.C:
				m.checkAndReload()
			}
		}
	}()
}

// StopWatching stops the config file watcher.
func (m *Manager) StopWatching() {
	if m.watchCancel != nil {
		m.watchCancel()
		m.watchCancel = nil
	}
}

// checkAndReload checks if the config file has been modified and reloads if necessary.
func (m *Manager) checkAndReload() {
	stat, err := os.Stat(m.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("[config] Error checking file: %v", err)
		}
		return
	}

	modTime := stat.ModTime()
	if modTime.After(m.lastModTime) {
		log.Printf("[config] Config file changed, reloading...")
		if err := m.Reload(context.Background()); err != nil {
			log.Printf("[config] Error reloading config: %v", err)
		}
	}
}

// generateRandomToken generates a random token for push API authentication
func generateRandomToken() string {
	b := make([]byte, 32) // 32 bytes = 64 hex characters
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based token if random generation fails
		return fmt.Sprintf("token_%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
