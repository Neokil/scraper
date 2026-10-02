package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/neokil/scraper/scrape-api/internal/api"
	"github.com/neokil/scraper/scrape-api/internal/config"
	"github.com/neokil/scraper/scrape-api/internal/dashboard"
	"github.com/neokil/scraper/scrape-api/internal/profiles"
	"github.com/neokil/scraper/scrape-api/internal/registry"
	"github.com/neokil/scraper/scrape-api/internal/workerclient"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			runHealthcheck()
			return
		case "version":
			fmt.Println(version)
			return
		}
	}
	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	level := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	profileRepo, err := profiles.New(cfg.ProfilesDir)
	if err != nil {
		logger.Error("profile repository unavailable", "error", err)
		os.Exit(1)
	}
	reg := registry.New()
	dashboardHandler, err := dashboard.New(reg, profileRepo, cfg.DashboardPoll)
	if err != nil {
		logger.Error("dashboard unavailable", "error", err)
		os.Exit(1)
	}
	app := api.New(cfg, reg, profileRepo, workerclient.New(cfg.WorkerRequestTimeout), logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go app.StartMaintenance(ctx)

	server := &http.Server{
		Addr:              cfg.Address,
		Handler:           app.Router(dashboardHandler),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       35 * time.Second,
		WriteTimeout:      2 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		logger.Info("scrape-api listening", "address", cfg.Address, "version", version)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func runHealthcheck() {
	target := os.Getenv("HEALTHCHECK_URL")
	if target == "" {
		target = "http://127.0.0.1:8080/healthz"
	}
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(target)
	if err != nil {
		os.Exit(1)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
