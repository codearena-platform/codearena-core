package main

import (
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/codearena-platform/codearena-core/internal/app/core"
	"github.com/codearena-platform/codearena-core/internal/app/runtime"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Vars reused from engine/realtime/runtime where appropriate or defined locally if specific to start
var (
	startGrpcPort     string
	startWebPort      string
	startRealtimePort string
	startRuntimeAddr  string
	startArenaWidth   float32
	startArenaHeight  float32
	startTickRate     int
	startRedisAddr    string
	startDbPath       string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start services (Monolith/Hybrid mode)",
	Long: `Starts the Core services in a unified manner.

Modes:
1. **Monolith (Default)**: Starts Engine, Realtime, and a local Runtime (Docker) in a single process.
   Ideal for local development.

2. **Hybrid**: Starts Engine and Realtime locally, but connects to a remote Runtime.
   Triggered by setting --runtime-addr to a non-local address.`,
	Example: `  # Monolith Mode (starts everything locally)
  codearena start

  # Hybrid Mode (connects to remote runtime)
  codearena start --runtime-addr=192.168.1.50:50053`,
	Run: func(cmd *cobra.Command, args []string) {
		startGrpcPort = viper.GetString("grpc-port")
		startWebPort = viper.GetString("web-port")
		startRealtimePort = viper.GetString("realtime-port")
		startRuntimeAddr = viper.GetString("runtime-addr")
		startArenaWidth = float32(viper.GetFloat64("arena-width"))
		startArenaHeight = float32(viper.GetFloat64("arena-height"))
		startTickRate = viper.GetInt("tick-rate")
		startDbPath = viper.GetString("db-path")

		localRuntime, runtimeAddr := determineRuntimeMode(startRuntimeAddr)

		var wg sync.WaitGroup

		if localRuntime {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := runtime.Start(runtime.Config{Port: "50053"}); err != nil {
					slog.Error("Local Runtime Failed", "error", err)
					os.Exit(1)
				}
			}()
			// Delay slightly to allow runtime port to bind
			time.Sleep(500 * time.Millisecond)
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := core.Config{
				GRPCPort:     startGrpcPort,
				WebPort:      startWebPort,
				RealtimePort: startRealtimePort,
				RuntimeAddr:  runtimeAddr,
				RunRealtime:  true,
				ArenaWidth:   startArenaWidth,
				ArenaHeight:  startArenaHeight,
				TickRate:     startTickRate,
				RedisAddr:    startRedisAddr,
				DBPath:       startDbPath,
			}
			if err := core.Start(cfg); err != nil {
				slog.Error("Engine Failed", "error", err)
				os.Exit(1)
			}
		}()

		wg.Wait()
	},
}

func determineRuntimeMode(addr string) (bool, string) {
	if addr != "" && addr != "localhost:50053" && addr != ":50053" {
		slog.Info("Hybrid Mode: Using remote Runtime", "address", addr)
		return false, addr
	}
	slog.Info("Monolith Mode: Starting local Runtime Service")
	return true, "localhost:50053"
}

func init() {
	startCmd.Flags().StringVar(&startGrpcPort, "grpc-port", ":50051", "gRPC Port for Engine")
	startCmd.Flags().StringVar(&startWebPort, "web-port", ":50052", "Web Port for Engine/Dashboard")
	startCmd.Flags().StringVar(&startRealtimePort, "realtime-port", ":8081", "Websocket Port for Realtime")
	startCmd.Flags().StringVar(&startRuntimeAddr, "runtime-addr", "", "Address of the Runtime Service (leave empty for local)")
	startCmd.Flags().Float32Var(&startArenaWidth, "arena-width", 800, "Width of the game arena")
	startCmd.Flags().Float32Var(&startArenaHeight, "arena-height", 600, "Height of the game arena")
	startCmd.Flags().IntVar(&startTickRate, "tick-rate", 60, "Game loop ticks per second")
	startCmd.Flags().StringVar(&startRedisAddr, "redis-addr", "localhost:6379", "Redis address for horizontal scaling")
	startCmd.Flags().StringVar(&startDbPath, "db-path", "codearena.db", "Path to SQLite database file")

	viper.BindPFlag("grpc-port", startCmd.Flags().Lookup("grpc-port"))
	viper.BindPFlag("web-port", startCmd.Flags().Lookup("web-port"))
	viper.BindPFlag("realtime-port", startCmd.Flags().Lookup("realtime-port"))
	viper.BindPFlag("runtime-addr", startCmd.Flags().Lookup("runtime-addr"))
	viper.BindPFlag("arena-width", startCmd.Flags().Lookup("arena-width"))
	viper.BindPFlag("arena-height", startCmd.Flags().Lookup("arena-height"))
	viper.BindPFlag("tick-rate", startCmd.Flags().Lookup("tick-rate"))
	viper.BindPFlag("redis-addr", startCmd.Flags().Lookup("redis-addr"))
	viper.BindPFlag("db-path", startCmd.Flags().Lookup("db-path"))

	rootCmd.AddCommand(startCmd)
}
