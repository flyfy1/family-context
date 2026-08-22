package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := loadDotEnv("../.env", ".env", "../.agent-machine.env", ".agent-machine.env"); err != nil {
		return err
	}
	dataDir, err := prepareStorageRoot()
	if err != nil {
		return err
	}

	store, err := openStore(filepath.Join(dataDir, "family-daily.db"))
	if err != nil {
		return err
	}
	defer store.Close()

	ai, err := newAudioProcessorFromEnv()
	if err != nil {
		return err
	}

	apiToken := envOr("FAMILY_API_TOKEN", "family-daily-local")
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		adminToken := os.Getenv("ADMIN_API_TOKEN")
		if adminToken == "" || secureEqual(adminToken, apiToken) {
			return errors.New("production requires ADMIN_API_TOKEN distinct from FAMILY_API_TOKEN")
		}
	}
	app := newApp(store, ai, filepath.Join(dataDir, "media"), apiToken)
	server := &http.Server{
		Addr:              envOr("ADDR", ":8080"),
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      3 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("Family Daily backend listening on %s (AI_MODE=%s, local storage=%s)", server.Addr, envOr("AI_MODE", "gemini"), dataDir)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
