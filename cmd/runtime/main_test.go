package main

import (
	"errors"
	"testing"

	"github.com/codearena-platform/codearena-core/internal/app/runtime"
)

func TestRunUsesConfiguredPort(t *testing.T) {
	t.Setenv("PORT", "60053")
	var got runtime.Config
	original := startRuntime
	startRuntime = func(cfg runtime.Config) error {
		got = cfg
		return nil
	}
	t.Cleanup(func() { startRuntime = original })

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got.Port != "60053" {
		t.Errorf("port = %q, want 60053", got.Port)
	}
}

func TestRunUsesDefaultPort(t *testing.T) {
	t.Setenv("PORT", "")
	var got runtime.Config
	original := startRuntime
	startRuntime = func(cfg runtime.Config) error {
		got = cfg
		return nil
	}
	t.Cleanup(func() { startRuntime = original })

	if err := run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got.Port != "50053" {
		t.Errorf("port = %q, want 50053", got.Port)
	}
}

func TestRunPropagatesError(t *testing.T) {
	want := errors.New("start failed")
	original := startRuntime
	startRuntime = func(runtime.Config) error { return want }
	t.Cleanup(func() { startRuntime = original })

	if err := run(); !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
