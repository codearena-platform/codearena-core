package main

import (
	"log"
	"os"

	"github.com/codearena-platform/codearena-core/internal/app/core"
)

func main() {
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = ":50051"
	}

	webPort := os.Getenv("WEB_PORT")
	if webPort == "" {
		webPort = ":50052"
	}

	realtimePort := os.Getenv("REALTIME_PORT")
	if realtimePort == "" {
		realtimePort = ":8081"
	}

	runtimeAddr := os.Getenv("RUNTIME_ADDR")
	if runtimeAddr == "" {
		runtimeAddr = "localhost:50053"
	}

	// Engine binary includes embedded Realtime unless explicitly disabled
	noRealtime := os.Getenv("NO_REALTIME") == "true"

	cfg := core.Config{
		GRPCPort:     grpcPort,
		WebPort:      webPort,
		RealtimePort: realtimePort,
		RuntimeAddr:  runtimeAddr,
		RunRealtime:  !noRealtime,
	}

	if err := core.Start(cfg); err != nil {
		log.Fatalf("Engine Failed: %v", err)
	}
}
