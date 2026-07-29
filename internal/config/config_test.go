package config

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func validConfig() Config {
	return Config{
		Channels:       []ChannelConfig{{Name: "phone", Type: "bark", Token: "key"}},
		ChannelGroups:  []ChannelGroupConfig{{Name: "everyone", Use: []string{"phone"}}},
		DefaultChannel: "everyone",
		Auth:           AuthConfig{User: "admin", Pass: "secret"},
	}
}

func TestValidateReferences(t *testing.T) {
	if err := Validate(validConfig()); err != nil {
		t.Fatalf("valid configuration rejected: %v", err)
	}

	tests := []struct {
		name string
		edit func(*Config)
	}{
		{"duplicate target", func(cfg *Config) { cfg.ChannelGroups[0].Name = "phone" }},
		{"unknown group member", func(cfg *Config) { cfg.ChannelGroups[0].Use = []string{"missing"} }},
		{"unknown default", func(cfg *Config) { cfg.DefaultChannel = "missing" }},
		{"missing channel type", func(cfg *Config) { cfg.Channels[0].Type = "" }},
		{"missing authentication", func(cfg *Config) { cfg.Auth.Pass = "" }},
		{"unknown webhook target", func(cfg *Config) { cfg.Webhooks.Tawk.Chan = "missing" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.edit(&cfg)
			if err := Validate(cfg); err == nil {
				t.Fatal("invalid configuration was accepted")
			}
		})
	}
}

func TestSetRawRejectsStaleRevision(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "config.yaml"))
	if err := manager.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	staleRevision := manager.Revision()
	first := "channels:\n  - name: first\n    type: stub\n    token: stub\ndefault_channel: first\nauth:\n  user: admin\n  pass: secret\n"
	if err := manager.SetRawIfRevision(context.Background(), first, staleRevision); err != nil {
		t.Fatalf("first save failed: %v", err)
	}
	second := "channels:\n  - name: second\n    type: stub\n    token: stub\ndefault_channel: second\nauth:\n  user: admin\n  pass: secret\n"
	if err := manager.SetRawIfRevision(context.Background(), second, staleRevision); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("expected ErrConfigConflict, got %v", err)
	}
	if got := manager.Get().DefaultChannel; got != "first" {
		t.Fatalf("stale save changed configuration: got %q", got)
	}
}
