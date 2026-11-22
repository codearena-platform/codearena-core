package main

import (
	"errors"
	"testing"

	"github.com/codearena-platform/codearena-core/internal/app/realtime"
)

func TestRunUsesEnvironment(t *testing.T) {
	t.Setenv("PORT", ":9000")
	t.Setenv("ENGINE_ADDR", "engine:50051")
	t.Setenv("REDIS_ADDR", "redis:6379")

	var got realtime.Config
	original := startRealtime
	startRealtime = func(cfg realtime.Config) error {
		got = cfg
		return nil
	}
	t.Cleanup(func() { startRealtime = original })

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := realtime.Config{Port: ":9000", EngineAddr: "engine:50051", RedisAddr: "redis:6379"}
	if got != want {
		t.Errorf("config = %+v, want %+v", got, want)
	}
}

func TestRunUsesDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("ENGINE_ADDR", "")
	t.Setenv("REDIS_ADDR", "")

	var got realtime.Config
	original := startRealtime
	startRealtime = func(cfg realtime.Config) error {
		got = cfg
		return nil
	}
	t.Cleanup(func() { startRealtime = original })

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	want := realtime.Config{Port: ":8081", EngineAddr: "localhost:50051", RedisAddr: "localhost:6379"}
	if got != want {
		t.Errorf("config = %+v, want %+v", got, want)
	}
}

func TestRunPropagatesError(t *testing.T) {
	want := errors.New("start failed")
	original := startRealtime
	startRealtime = func(realtime.Config) error { return want }
	t.Cleanup(func() { startRealtime = original })

	if err := run(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
