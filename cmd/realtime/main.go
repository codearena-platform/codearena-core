package main

import (
	"log"
	"os"

	"github.com/codearena-platform/codearena-core/internal/app/realtime"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":8081"
	}

	engineAddr := os.Getenv("ENGINE_ADDR")
	if engineAddr == "" {
		engineAddr = "localhost:50051"
	}

	cfg := realtime.Config{
		Port:       port,
		EngineAddr: engineAddr,
	}

	if err := realtime.Start(cfg); err != nil {
		log.Fatalf("Realtime Failed: %v", err)
	}
}
