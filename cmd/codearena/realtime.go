package main

import (
	"log/slog"
	"os"

	"github.com/codearena-platform/codearena-core/internal/app/realtime"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	realtimePort       string
	realtimeEngineAddr string
	realtimeRedisAddr  string
)

var realtimeCmd = &cobra.Command{
	Use:   "realtime",
	Short: "Run the Realtime Service (Standalone)",
	Long: `Starts the Realtime WebSocket Service (Standalone Mode).

This component handles:
- Broadcasting game state updates to connected clients (web frontend).
- Managing WebSocket connections.
- Subscribing to events from the Engine via gRPC.`,
	Example: `  # Run on default port (:8081)
  codearena realtime

  # Connect to a remote Engine
  codearena realtime --engine-addr=10.0.0.4:50051`,
	Run: func(cmd *cobra.Command, args []string) {
		// Sync with viper
		realtimePort = viper.GetString("realtime-port")
		realtimeEngineAddr = viper.GetString("engine-addr")
		realtimeRedisAddr = viper.GetString("redis-addr")

		cfg := realtime.Config{
			Port:       realtimePort,
			EngineAddr: realtimeEngineAddr,
			RedisAddr:  realtimeRedisAddr,
		}
		if err := realtime.Start(cfg); err != nil {
			slog.Error("Realtime Failed", "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	realtimeCmd.Flags().StringVar(&realtimePort, "realtime-port", ":8081", "Port for the Realtime service")
	realtimeCmd.Flags().StringVar(&realtimeEngineAddr, "engine-addr", "localhost:50051", "Address of the Engine service")
	realtimeCmd.Flags().StringVar(&realtimeRedisAddr, "redis-addr", "localhost:6379", "Redis address for horizontal scaling")

	viper.BindPFlag("realtime-port", realtimeCmd.Flags().Lookup("realtime-port"))
	viper.BindPFlag("engine-addr", realtimeCmd.Flags().Lookup("engine-addr"))
	viper.BindPFlag("redis-addr", realtimeCmd.Flags().Lookup("redis-addr"))

	rootCmd.AddCommand(realtimeCmd)
}
