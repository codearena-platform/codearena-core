package main

import (
	"log"

	"github.com/codearena-platform/codearena-core/internal/app/core"
)

func main() {
	// The standalone binary uses the same config struct, defaults and
	// validation as `codearena engine` and `codearena start`.
	cfg, err := core.ConfigFromEnv()
	if err != nil {
		log.Fatalf("Invalid engine configuration: %v", err)
	}
	if err := core.Start(cfg); err != nil {
		log.Fatalf("Engine Failed: %v", err)
	}
}
