package main

import (
	"fmt"

	"github.com/codearena-platform/codearena-core/internal/app/core"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	engineGrpcPort     string
	engineWebPort      string
	engineRealtimePort string
	engineRuntimeAddr  string
	engineNoRealtime   bool
	engineArenaWidth   float32
	engineArenaHeight  float32
	engineTickRate     int
	engineRedisAddr    string
	engineDbPath       string
)

var engineCmd = &cobra.Command{
	Use:   "engine",
	Short: "Run the Simulation Engine",
	Long: `Starts the Simulation Engine and API Gateway.
	
This component handles:
- Game logic and Physics simulation (Tick loop).
- Match management state.
- gRPC API for external services.
- Optionally embeds the Realtime WebSocket service (default: enabled).`,
	Example: `  # Run with defaults (includes Realtime)
  codearena engine

  # Run without Realtime (if Realtime is running separately)
  codearena engine --no-realtime

  # Connect to a remote Runtime service
  codearena engine --runtime-addr=10.0.0.5:50053`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Rebind to this command's flags: both engine and start register the
		// same viper keys, so the last init would otherwise win.
		if err := viper.BindPFlags(cmd.Flags()); err != nil {
			return fmt.Errorf("bind engine flags: %w", err)
		}

		engineGrpcPort = viper.GetString("grpc-port")
		engineWebPort = viper.GetString("web-port")
		engineRealtimePort = viper.GetString("realtime-port")
		engineRuntimeAddr = viper.GetString("runtime-addr")
		engineNoRealtime = viper.GetBool("no-realtime")
		engineArenaWidth = float32(viper.GetFloat64("arena-width"))
		engineArenaHeight = float32(viper.GetFloat64("arena-height"))
		engineTickRate = viper.GetInt("tick-rate")
		engineRedisAddr = viper.GetString("redis-addr")
		engineDbPath = viper.GetString("db-path")

		cfg := core.Config{
			GRPCPort:     engineGrpcPort,
			WebPort:      engineWebPort,
			RealtimePort: engineRealtimePort,
			RuntimeAddr:  engineRuntimeAddr,
			RunRealtime:  !engineNoRealtime,
			ArenaWidth:   engineArenaWidth,
			ArenaHeight:  engineArenaHeight,
			TickRate:     engineTickRate,
			RedisAddr:    engineRedisAddr,
			DBPath:       engineDbPath,
		}
		return core.Start(cfg)
	},
}

func init() {
	engineCmd.Flags().StringVar(&engineGrpcPort, "grpc-port", ":50051", "gRPC Port for Engine")
	engineCmd.Flags().StringVar(&engineWebPort, "web-port", ":50052", "Web Port for Engine/Dashboard")
	engineCmd.Flags().StringVar(&engineRealtimePort, "realtime-port", ":8081", "Websocket Port for Realtime (if enabled)")
	engineCmd.Flags().StringVar(&engineRuntimeAddr, "runtime-addr", "localhost:50053", "Address of the Runtime Service")
	engineCmd.Flags().BoolVar(&engineNoRealtime, "no-realtime", false, "Disable embedded Realtime service")
	engineCmd.Flags().Float32Var(&engineArenaWidth, "arena-width", 800, "Width of the game arena")
	engineCmd.Flags().Float32Var(&engineArenaHeight, "arena-height", 600, "Height of the game arena")
	engineCmd.Flags().IntVar(&engineTickRate, "tick-rate", 60, "Game loop ticks per second")
	engineCmd.Flags().StringVar(&engineRedisAddr, "redis-addr", "localhost:6379", "Redis address for horizontal scaling")
	engineCmd.Flags().StringVar(&engineDbPath, "db-path", "codearena.db", "Path to SQLite database file")

	viper.BindPFlag("grpc-port", engineCmd.Flags().Lookup("grpc-port"))
	viper.BindPFlag("web-port", engineCmd.Flags().Lookup("web-port"))
	viper.BindPFlag("realtime-port", engineCmd.Flags().Lookup("realtime-port"))
	viper.BindPFlag("runtime-addr", engineCmd.Flags().Lookup("runtime-addr"))
	viper.BindPFlag("no-realtime", engineCmd.Flags().Lookup("no-realtime"))
	viper.BindPFlag("arena-width", engineCmd.Flags().Lookup("arena-width"))
	viper.BindPFlag("arena-height", engineCmd.Flags().Lookup("arena-height"))
	viper.BindPFlag("tick-rate", engineCmd.Flags().Lookup("tick-rate"))
	viper.BindPFlag("redis-addr", engineCmd.Flags().Lookup("redis-addr"))
	viper.BindPFlag("db-path", engineCmd.Flags().Lookup("db-path"))

	rootCmd.AddCommand(engineCmd)
}
