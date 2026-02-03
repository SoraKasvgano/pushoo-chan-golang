package config

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is kept compatible with the existing `config.yaml` format used by the TS version.
type Config struct {
	Channels       []ChannelConfig      `yaml:"channels"`
	ChannelGroups  []ChannelGroupConfig `yaml:"channel_groups,omitempty"`
	DefaultChannel string               `yaml:"default_channel,omitempty"`
	Auth           AuthConfig           `yaml:"auth,omitempty"`

	// Optional extensions (ignored by older configs):
	SQLite SQLiteConfig `yaml:"sqlite,omitempty"`
}

type SQLiteConfig struct {
	Path string `yaml:"path,omitempty"`
}

type AuthConfig struct {
	User string `yaml:"user,omitempty"`
	Pass string `yaml:"pass,omitempty"`
}

type ChannelConfig struct {
	Name  string `yaml:"name"`
	Type  string `yaml:"type"`
	Token string `yaml:"token"`
}

type ChannelGroupConfig struct {
	Name string   `yaml:"name"`
	Use  []string `yaml:"use,omitempty"`
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

# Authentication for web interface and config management (REQUIRED)
# IMPORTANT: Change these default credentials immediately!
auth:
  user: pushoo
  pass: pushoo

# SQLite database for storing push history (optional)
# sqlite:
#   path: ./data/pushoo.db
`

type Manager struct {
	path string

	mu     sync.RWMutex
	raw    string
	parsed Config

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

