package main

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

func TestEnvironmentVariableMapping(t *testing.T) {
	// Clean up environment after test
	defer os.Unsetenv("CODEARENA_GRPC_PORT")
	defer os.Unsetenv("CODEARENA_ARENA_WIDTH")

	os.Setenv("CODEARENA_GRPC_PORT", ":9999")
	os.Setenv("CODEARENA_ARENA_WIDTH", "1200")

	// initConfig is called by Cobra init, but we can call it manually for testing
	viper.Reset()
	initConfig()

	if port := viper.GetString("grpc-port"); port != ":9999" {
		t.Errorf("expected grpc-port to be %q, got %q", ":9999", port)
	}

	if width := viper.GetFloat64("arena-width"); width != 1200 {
		t.Errorf("expected arena-width to be 1200, got %f", width)
	}
}

func TestEnvKeyReplacer(t *testing.T) {
	defer os.Unsetenv("CODEARENA_LOG_LEVEL")
	os.Setenv("CODEARENA_LOG_LEVEL", "DEBUG")

	viper.Reset()
	initConfig()

	if level := viper.GetString("log-level"); level != "DEBUG" {
		t.Errorf("expected log-level to be DEBUG, got %q", level)
	}
}
