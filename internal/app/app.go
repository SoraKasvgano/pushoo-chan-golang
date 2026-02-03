package app

import (
	"context"
	"embed"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"pushoo-chan-gover/internal/config"
	"pushoo-chan-gover/internal/httpapi"
	"pushoo-chan-gover/internal/push"
	"pushoo-chan-gover/internal/store"
)

type Options struct {
	Addr        string
	ConfigPath  string
	FrontendDir string
	SQLitePath  string // optional override
	EmbeddedFS  embed.FS
}

type Server struct {
	Addr    string
	Handler http.Handler
}

func New(opts Options) (*Server, func(), error) {
	if opts.Addr == "" {
		opts.Addr = ":8084"
	}
	if opts.ConfigPath == "" {
		return nil, nil, errors.New("ConfigPath is required")
	}
	if opts.FrontendDir == "" {
		opts.FrontendDir = "frontend"
	}

	// If the user didn't put config.yaml in cwd but uses the docker volume layout,
	// keep it convenient by auto-falling-back to ./user_config/config.yaml.
	if _, err := os.Stat(opts.ConfigPath); err != nil {
		if os.IsNotExist(err) {
			alt := filepath.Join(filepath.Dir(opts.ConfigPath), "user_config", "config.yaml")
			if _, err2 := os.Stat(alt); err2 == nil {
				opts.ConfigPath = alt
			}
		}
	}

	cfg := config.NewManager(opts.ConfigPath)
	if err := cfg.Reload(context.Background()); err != nil {
		return nil, nil, err
	}

	// Start hot reload watcher
	cfg.StartWatching(context.Background(), 3*time.Second)

	eventHub := httpapi.NewEventHub()
	providers := push.NewProviderRegistry()
	providers.Register(push.NewStubProvider())
	providers.Register(push.NewBarkProvider())
	providers.Register(push.NewTelegramProvider())
	providers.Register(push.NewWebhookProvider())

	pusher := push.NewService(cfg, providers, push.ServiceOptions{
		MaxRetry:      5,
		RetryInterval: 3 * time.Second,
		HTTPTimeout:   10 * time.Second,
		OnEvent:       eventHub.Broadcast,
	})

	var st store.Store = store.NewNoop()
	var stClose func() = func() {}
	sqlitePath := opts.SQLitePath
	if sqlitePath == "" {
		sqlitePath = cfg.Get().SQLite.Path
	}
	if sqlitePath != "" {
		s, closeFn, err := store.NewSQLite(sqlitePath, store.SQLiteOptions{
			RecordChannelMessages: cfg.Get().SQLite.RecordChannelMessages,
		})
		if err != nil {
			return nil, nil, err
		}
		st = s
		stClose = closeFn
	}

	api := httpapi.New(httpapi.Options{
		Config:      cfg,
		Push:        pusher,
		Store:       st,
		EventHub:    eventHub,
		FrontendDir: opts.FrontendDir,
		EmbeddedFS:  opts.EmbeddedFS,
	})

	var cleanupCancel context.CancelFunc = func() {}
	if ms, ok := st.(store.MaintenanceStore); ok {
		sqlCfg := cfg.Get().SQLite
		if sqlCfg.CleanupDays > 0 && sqlCfg.CleanupIntervalHours > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			cleanupCancel = cancel
			interval := time.Duration(sqlCfg.CleanupIntervalHours) * time.Hour
			go func() {
				ticker := time.NewTicker(interval)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						before := time.Now().Add(-time.Duration(sqlCfg.CleanupDays) * 24 * time.Hour)
						if _, err := ms.Cleanup(context.Background(), before); err != nil {
							log.Printf("[store] auto cleanup failed: %v", err)
						}
					}
				}
			}()
			log.Printf("[store] auto cleanup enabled: keep %d days, interval %v", sqlCfg.CleanupDays, interval)
		}
	}

	cleanup := func() {
		cfg.StopWatching()
		cleanupCancel()
		stClose()
	}

	return &Server{
		Addr:    opts.Addr,
		Handler: api.Handler(),
	}, cleanup, nil
}
