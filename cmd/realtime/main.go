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

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	cfg := realtime.Config{
		Port:       port,
		EngineAddr: engineAddr,
		RedisAddr:  redisAddr,
	}

	if err := realtime.Start(cfg); err != nil {
		log.Fatalf("Realtime Failed: %v", err)
	}
}
