package main

import (
	"log/slog"
	"os"

	"github.com/codearena-platform/codearena-core/internal/app/runtime"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	runtimePort    string
	runtimeMaxBots int
)

var runtimeCmd = &cobra.Command{
	Use:   "runtime",
	Short: "Run the Runtime Service (Sandboxing)",
	Long: `Starts the Bot Execution Sandbox.

This service is responsible for:
- Managing Docker containers for bot code execution.
- Monitoring container health and resource usage.
- Exposing a gRPC API for the Engine to start/stop bots.`,
	Example: `  # Run on default port (50053)
  codearena runtime

  # Run on a custom port
  codearena runtime --port=6000`,
	Run: func(cmd *cobra.Command, args []string) {
		// Sync with viper
		runtimePort = viper.GetString("port")
		runtimeMaxBots = viper.GetInt("max-concurrent-bots")

		cfg := runtime.Config{
			Port:    runtimePort,
			MaxBots: runtimeMaxBots,
		}
		if err := runtime.Start(cfg); err != nil {
			slog.Error("Runtime Failed", "error", err)
			os.Exit(1)
		}
	},
}

func init() {
	runtimeCmd.Flags().StringVar(&runtimePort, "port", "50053", "gRPC Port for Runtime")
	runtimeCmd.Flags().IntVar(&runtimeMaxBots, "max-concurrent-bots", 10, "Maximum number of concurrent bots")

	viper.BindPFlag("port", runtimeCmd.Flags().Lookup("port"))
	viper.BindPFlag("max-concurrent-bots", runtimeCmd.Flags().Lookup("max-concurrent-bots"))

	rootCmd.AddCommand(runtimeCmd)
}
