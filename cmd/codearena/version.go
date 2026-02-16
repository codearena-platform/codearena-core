package main

import (
	"github.com/spf13/cobra"
)

var version = "0.1.0" // Ideally set via ldflags during build

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of CodeArena",
	Long:  `All software has versions. This is CodeArena's.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf("CodeArena Core v%s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
