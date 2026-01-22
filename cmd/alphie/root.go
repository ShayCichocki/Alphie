package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var interactiveResume bool
var interactiveGreenfield bool

// CheckClaudeCLI verifies that the 'claude' CLI is available in PATH.
// Returns an error with installation instructions if not found.
func CheckClaudeCLI() error {
	_, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH\n\n" +
			"Alphie requires the Claude Code CLI to orchestrate agents.\n\n" +
			"Install it with:\n" +
			"  npm install -g @anthropic-ai/claude-code\n\n" +
			"For more information, visit:\n" +
			"  https://docs.anthropic.com/en/docs/claude-code")
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "alphie",
	Short: "Agent Orchestrator & Learning Engine",
	Long: `Alphie orchestrates parallel Claude Code agents on workstreams,
accumulates learnings, and manages tasks to maximize development throughput.

With no arguments, launches interactive mode with a persistent TUI where you
can type tasks and watch them execute in parallel.

Core capabilities:
- Decomposes work into parallelizable tasks
- Spawns isolated agents in git worktrees
- Self-improves code via Ralph-loop (critique, improve, repeat)
- Learns from failures and successes
- Merges safely via session branches`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractive()
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Add flags for interactive mode
	rootCmd.Flags().BoolVar(&interactiveResume, "resume", false, "Resume incomplete tasks from previous sessions")
	rootCmd.Flags().BoolVar(&interactiveGreenfield, "greenfield", false, "Direct merge to main (skip session branches)")

	// Add subcommands
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(learnCmd)
	rootCmd.AddCommand(cleanupCmd)
	rootCmd.AddCommand(baselineCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(implementCmd)
}
