package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	if os.Getenv("FAMILY_DAILY_BACKFILL_TRANSCRIPTS") == "1" {
		report, backfillErr := runTranscriptBackfill(context.Background(), store, ai, dataDir)
		if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
			return err
		}
		if backfillErr != nil {
			return backfillErr
		}
		if len(report.Failures) > 0 {
			return fmt.Errorf("transcript backfill completed with %d failures", len(report.Failures))
		}
		return nil
	}

	adminToken := envOr("ADMIN_API_TOKEN", "family-daily-admin-local")
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		if strings.TrimSpace(os.Getenv("ADMIN_API_TOKEN")) == "" {
			return errors.New("production requires ADMIN_API_TOKEN")
		}
		if !validProductionPublicBaseURL(os.Getenv("PUBLIC_BASE_URL")) {
			return errors.New("production requires PUBLIC_BASE_URL as an HTTPS origin")
		}
	}
	app := newApp(store, ai, filepath.Join(dataDir, "media"), adminToken)
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
	go app.startCoreJobs(ctx)
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
