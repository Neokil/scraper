package config

import (
	"testing"
	"time"
)

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("WORKER_TIMEOUT", "eventually")
	if _, err := Load(); err == nil {
		t.Fatal("invalid duration was accepted")
	}
}

func TestLoadAcceptsIntegerSeconds(t *testing.T) {
	t.Setenv("WORKER_TIMEOUT", "45")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkerOfflineTimeout != 45*time.Second {
		t.Fatalf("unexpected timeout %s", cfg.WorkerOfflineTimeout)
	}
}
