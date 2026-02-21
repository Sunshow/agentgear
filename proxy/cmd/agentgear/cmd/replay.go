package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sunshow/agentgear/proxy/internal/replay"
)

var (
	replayServer      string
	replayDir         string
	replayTransformed bool
	replayHeaders     []string
	replayTimeout     int
	replayQuiet       bool
	replaySequence    string
	replayPath        string
)

var replayCmd = &cobra.Command{
	Use:   "replay",
	Short: "Replay session logs to a target server",
	Long: `Replay recorded session requests from log directories to a specified server.

Examples:
  # Replay a single session
  agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions/20260212-230152_5e0b909d

  # Replay all sessions in a directory
  agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions

  # Use transformed request body
  agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions --transformed

  # Add API key header
  agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions -H "X-Api-Key:sk-xxx"

  # Replay only specific sequences
  agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions --seq 1,3

  # Override request path to test a different endpoint
  agentgear replay -s http://127.0.0.1:9000 -d ./logs/sessions -p /warp_us10/v1/messages`,
	Run: runReplay,
}

func init() {
	replayCmd.Flags().StringVarP(&replayServer, "server", "s", "", "target server URL (required)")
	replayCmd.Flags().StringVarP(&replayDir, "dir", "d", "", "session log directory path (required)")
	replayCmd.Flags().BoolVar(&replayTransformed, "transformed", false, "use transformed request body")
	replayCmd.Flags().StringArrayVarP(&replayHeaders, "header", "H", nil, "extra headers in Key:Value format (repeatable)")
	replayCmd.Flags().IntVarP(&replayTimeout, "timeout", "t", 600, "request timeout in seconds")
	replayCmd.Flags().BoolVarP(&replayQuiet, "quiet", "q", false, "quiet mode, only print summary")
	replayCmd.Flags().StringVar(&replaySequence, "seq", "", "replay only specific sequences (comma-separated, e.g. 1,3,5)")
	replayCmd.Flags().StringVarP(&replayPath, "path", "p", "", "override request path (e.g. /warp_us10/v1/messages)")

	_ = replayCmd.MarkFlagRequired("server")
	_ = replayCmd.MarkFlagRequired("dir")
}

func runReplay(cmd *cobra.Command, args []string) {
	sequences, err := replay.ParseSequences(replaySequence)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	opts := replay.Options{
		Server:       replayServer,
		Dir:          replayDir,
		Transformed:  replayTransformed,
		Headers:      replay.ParseHeaders(replayHeaders),
		Timeout:      replayTimeout,
		Quiet:        replayQuiet,
		Sequences:    sequences,
		PathOverride: replayPath,
	}

	if err := replay.Run(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
