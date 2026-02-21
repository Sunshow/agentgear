package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "agentgear",
	Short: "AgentGear - Agent API adapter proxy",
	Long:  "AgentGear is an Agent API adapter layer that handles protocol compatibility between different agents.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(replayCmd)
}
