package main

import (
	"log"
	"os"

	"github.com/codearena-platform/codearena-core/internal/app/runtime"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "50053"
	}

	cfg := runtime.Config{
		Port: port,
	}

	if err := runtime.Start(cfg); err != nil {
		log.Fatalf("Runtime Failed: %v", err)
	}
}
