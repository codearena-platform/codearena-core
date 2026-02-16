package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"text/tabwriter"

	pb "github.com/codearena-platform/codearena-core/pkg/api/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	replayAddr      string
	replayStartTick int64
	replayEndTick   int64
)

var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Match replay and history management",
	Long:  "Commands to list previous matches, view event logs, and discover highlights.",
}

var replayListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all historical matches",
	Run: func(cmd *cobra.Command, args []string) {
		conn, err := grpc.Dial(replayAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			slog.Error("Failed to connect to core", "error", err)
			os.Exit(1)
		}
		defer conn.Close()
		client := pb.NewMatchServiceClient(conn)

		resp, err := client.ListMatches(context.Background(), &pb.Empty{})
		if err != nil {
			slog.Error("Failed to list matches", "error", err)
			os.Exit(1)
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "MATCH ID\tSTATUS")
		for _, m := range resp.Matches {
			fmt.Fprintf(w, "%s\t%s\n", m.MatchId, m.Status)
		}
		w.Flush()
	},
}

var replayHighlightsCmd = &cobra.Command{
	Use:   "highlights [match-id]",
	Short: "View highlights for a specific match",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		matchID := args[0]
		conn, err := grpc.Dial(replayAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			slog.Error("Failed to connect to core", "error", err)
			os.Exit(1)
		}
		defer conn.Close()
		client := pb.NewMatchServiceClient(conn)

		resp, err := client.GetMatchHighlights(context.Background(), &pb.ReplayRequest{MatchId: matchID})
		if err != nil {
			slog.Error("Failed to get highlights", "error", err)
			os.Exit(1)
		}

		fmt.Printf("Highlights for Match: %s\n", matchID)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "TICK\tTYPE\tDESCRIPTION")
		for _, m := range resp.Moments {
			fmt.Fprintf(w, "%d\t%s\t%s\n", m.Tick, m.Type, m.Description)
		}
		w.Flush()
	},
}

var replayLogsCmd = &cobra.Command{
	Use:   "logs [match-id]",
	Short: "Dump event logs for a specific match",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		matchID := args[0]
		conn, err := grpc.Dial(replayAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			slog.Error("Failed to connect to core", "error", err)
			os.Exit(1)
		}
		defer conn.Close()
		client := pb.NewMatchServiceClient(conn)

		resp, err := client.GetMatchReplay(context.Background(), &pb.ReplayRequest{
			MatchId:   matchID,
			StartTick: replayStartTick,
			EndTick:   replayEndTick,
		})
		if err != nil {
			slog.Error("Failed to get logs", "error", err)
			os.Exit(1)
		}

		fmt.Printf("Event Logs for Match: %s\n", matchID)
		for _, ev := range resp.Events {
			fmt.Printf("[%d] %v\n", ev.Tick, ev.Event)
		}
	},
}

func init() {
	replayCmd.PersistentFlags().StringVar(&replayAddr, "addr", "localhost:50051", "Address of the core service")
	replayLogsCmd.Flags().Int64Var(&replayStartTick, "start", 0, "Start tick")
	replayLogsCmd.Flags().Int64Var(&replayEndTick, "end", 0, "End tick")

	replayCmd.AddCommand(replayListCmd)
	replayCmd.AddCommand(replayHighlightsCmd)
	replayCmd.AddCommand(replayLogsCmd)

	rootCmd.AddCommand(replayCmd)
}
