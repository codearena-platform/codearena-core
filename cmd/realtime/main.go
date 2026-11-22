package main

import (
	"fmt"
	"log"
	"os"

	"github.com/codearena-platform/codearena-core/internal/app/realtime"
)

// startRealtime is overridable in tests so the binary's env handling can be
// verified without opening sockets.
var startRealtime = realtime.Start

func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = ":8081"
	}
	engineAddr := os.Getenv("ENGINE_ADDR")
	if engineAddr == "" {
		engineAddr = "localhost:50051"
	}
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	if err := startRealtime(realtime.Config{
		Port:       port,
		EngineAddr: engineAddr,
		RedisAddr:  redisAddr,
	}); err != nil {
		return fmt.Errorf("realtime failed: %w", err)
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("Realtime Failed: %v", err)
	}
}
