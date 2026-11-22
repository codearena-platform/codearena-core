package main

import (
	"fmt"
	"log"

	"github.com/codearena-platform/codearena-core/internal/app/core"
)

// startEngine is a variable so tests can verify the config path without
// binding real ports or starting Docker.
var startEngine = core.Start

func run() error {
	// The standalone binary uses the same config struct, defaults and
	// validation as `codearena engine` and `codearena start`.
	cfg, err := core.ConfigFromEnv()
	if err != nil {
		return fmt.Errorf("invalid engine configuration: %w", err)
	}
	if err := startEngine(cfg); err != nil {
		return fmt.Errorf("engine failed: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Engine Failed: %v", err)
	}
}
