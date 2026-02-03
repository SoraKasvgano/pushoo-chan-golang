package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"pushoo-chan-gover/internal/app"
)

func main() {
	var (
		addr          = flag.String("addr", ":8084", "listen address")
		configFile    = flag.String("config", "", "config file path (overrides env PUSHOO_CONFIG_FILE)")
		frontendDir   = flag.String("frontend", "", "frontend directory (defaults to ./frontend)")
		sqlitePath    = flag.String("sqlite", "", "sqlite db file path (optional; overrides config)")
		readTimeout   = flag.Duration("read-timeout", 10*time.Second, "http server read timeout")
		writeTimeout  = flag.Duration("write-timeout", 30*time.Second, "http server write timeout")
		idleTimeout   = flag.Duration("idle-timeout", 60*time.Second, "http server idle timeout")
		shutdownGrace = flag.Duration("shutdown-grace", 10*time.Second, "graceful shutdown timeout")
	)
	flag.Parse()

	log.Println("========================================")
	log.Println("  pushoo-chan (Go Version)")
	log.Println("  Unified Push Notification Service")
	log.Println("========================================")

	wd, err := os.Getwd()
	if err != nil {
		log.Fatalf("getwd: %v", err)
	}

	cfgPath := *configFile
	if cfgPath == "" {
		cfgPath = os.Getenv("PUSHOO_CONFIG_FILE")
	}
	if cfgPath == "" {
		// Keep compatible with the original project which reads `config.yaml` from cwd.
		cfgPath = filepath.Join(wd, "config.yaml")
	}

	feDir := *frontendDir
	if feDir == "" {
		feDir = filepath.Join(wd, "frontend")
	}

	log.Printf("[init] Working directory: %s", wd)
	log.Printf("[init] Config file: %s", cfgPath)
	log.Printf("[init] Frontend directory: %s", feDir)
	if *sqlitePath != "" {
		log.Printf("[init] SQLite database: %s", *sqlitePath)
	}

	srv, cleanup, err := app.New(app.Options{
		Addr:        *addr,
		ConfigPath:  cfgPath,
		FrontendDir: feDir,
		SQLitePath:  *sqlitePath,
		EmbeddedFS:  EmbeddedFrontend,
	})
	if err != nil {
		log.Fatalf("[init] Failed to initialize: %v", err)
	}
	defer cleanup()

	httpSrv := &http.Server{
		Addr:         srv.Addr,
		Handler:      srv.Handler,
		ReadTimeout:  *readTimeout,
		WriteTimeout: *writeTimeout,
		IdleTimeout:  *idleTimeout,
	}

	go func() {
		log.Println("========================================")
		log.Printf("[server] Listening on %s", httpSrv.Addr)
		log.Printf("[server] Web UI: http://localhost%s", httpSrv.Addr)
		log.Printf("[server] Health check: http://localhost%s/api/health", httpSrv.Addr)
		log.Println("[server] Press Ctrl+C to stop")
		log.Println("========================================")
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[server] Listen error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("\n[server] Shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), *shutdownGrace)
	defer cancel()
	if err := httpSrv.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "[server] Shutdown error: %v\n", err)
	}
	log.Println("[server] Server stopped")
}

