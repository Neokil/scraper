package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Address              string
	ProfilesDir          string
	WorkerRequestTimeout time.Duration
	WorkerOfflineTimeout time.Duration
	DefaultIdleTimeout   time.Duration
	DashboardPoll        time.Duration
	LogLevel             string
}

func Load() (Config, error) {
	workerRequestTimeout, err := duration("WORKER_REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	workerOfflineTimeout, err := duration("WORKER_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	defaultIdleTimeout, err := duration("DEFAULT_IDLE_TIMEOUT", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}
	dashboardPoll, err := duration("DASHBOARD_POLL_INTERVAL", 3*time.Second)
	if err != nil {
		return Config{}, err
	}
	cfg := Config{
		Address:              env("SERVER_ADDRESS", ":8080"),
		ProfilesDir:          env("PROFILES_DIR", "./profiles"),
		WorkerRequestTimeout: workerRequestTimeout,
		WorkerOfflineTimeout: workerOfflineTimeout,
		DefaultIdleTimeout:   defaultIdleTimeout,
		DashboardPoll:        dashboardPoll,
		LogLevel:             env("LOG_LEVEL", "info"),
	}
	if cfg.DefaultIdleTimeout <= 0 {
		return Config{}, fmt.Errorf("DEFAULT_IDLE_TIMEOUT must be positive")
	}
	if cfg.WorkerRequestTimeout <= 0 || cfg.WorkerOfflineTimeout <= 0 {
		return Config{}, fmt.Errorf("worker timeouts must be positive")
	}
	if cfg.DashboardPoll <= 0 {
		return Config{}, fmt.Errorf("DASHBOARD_POLL_INTERVAL must be positive")
	}
	switch cfg.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return Config{}, fmt.Errorf("LOG_LEVEL must be debug, info, warn, or error")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func duration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}
	if parsed, err := time.ParseDuration(value); err == nil {
		return parsed, nil
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}
	return 0, fmt.Errorf("%s must be a duration or an integer number of seconds", key)
}
