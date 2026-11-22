package main

import (
	"errors"
	"testing"

	"github.com/codearena-platform/codearena-core/internal/app/core"
)

func TestRunUsesSharedConfig(t *testing.T) {
	t.Setenv("CODEARENA_GRPC_PORT", ":9001")
	t.Setenv("CODEARENA_ARENA_WIDTH", "640")
	t.Setenv("CODEARENA_TICK_RATE", "30")

	var got core.Config
	called := false
	original := startEngine
	startEngine = func(cfg core.Config) error {
		called = true
		got = cfg
		return nil
	}
	t.Cleanup(func() { startEngine = original })

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !called {
		t.Fatal("engine was not started")
	}
	if got.GRPCPort != ":9001" || got.ArenaWidth != 640 || got.TickRate != 30 {
		t.Errorf("unexpected config passed to engine: %+v", got)
	}
	if got.RuntimeAddr != core.DefaultRuntimeAddr {
		t.Errorf("runtime address = %q, want shared default %q", got.RuntimeAddr, core.DefaultRuntimeAddr)
	}
}

func TestRunRejectsInvalidConfigBeforeStarting(t *testing.T) {
	t.Setenv("CODEARENA_TICK_RATE", "not-a-number")

	called := false
	original := startEngine
	startEngine = func(core.Config) error {
		called = true
		return nil
	}
	t.Cleanup(func() { startEngine = original })

	if err := run(); err == nil {
		t.Fatal("run accepted an invalid tick rate")
	}
	if called {
		t.Error("engine was started despite invalid configuration")
	}
}

func TestRunPropagatesStartError(t *testing.T) {
	want := errors.New("start failed")
	original := startEngine
	startEngine = func(core.Config) error { return want }
	t.Cleanup(func() { startEngine = original })

	err := run()
	if !errors.Is(err, want) {
		t.Fatalf("run error = %v, want %v", err, want)
	}
}
