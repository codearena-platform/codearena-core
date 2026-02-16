package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "codearena",
	Short: "CodeArena Core Services CLI",
	Long: `CodeArena Core CLI: The unified orchestrator for the CodeArena platform.

This CLI allows you to run individual components (Engine, Runtime, Realtime) 
or a complete monolithic stack for development and testing.`,
	Example: `  # Run the full stack locally
  codearena start

  # Run only the Runtime service
  codearena runtime

  # Run the Engine connected to a remote Runtime
  codearena engine --runtime-addr=192.168.1.50:50051`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig, setupLogger)
}

func initConfig() {
	viper.SetEnvPrefix("CODEARENA")
	viper.AutomaticEnv() // Read in environment variables that match
	// Replace dashes with underscores for env var matching
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
}

func setupLogger() {
	levelStr := viper.GetString("LOG_LEVEL")
	if levelStr == "" {
		levelStr = "INFO"
	}

	var level slog.Level
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	var handler slog.Handler
	if strings.ToLower(viper.GetString("LOG_FORMAT")) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
