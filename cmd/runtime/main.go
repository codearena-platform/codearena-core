package main

import (
	"fmt"
	"log"
	"os"

	"github.com/codearena-platform/codearena-core/internal/app/runtime"
)

// startRuntime is overridable in tests so the binary's env handling can be
// verified without starting Docker.
var startRuntime = runtime.Start

func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "50053"
	}
	if err := startRuntime(runtime.Config{Port: port}); err != nil {
		return fmt.Errorf("runtime failed: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Runtime Failed: %v", err)
	}
}
