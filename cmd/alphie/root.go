package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// Global flags
var (
	greenfieldEnabled bool // Greenfield mode: merge directly to main
)

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
	Short: "Spec-Driven Development Orchestrator",
	Long: `Alphie takes a specification and orchestrates parallel agents to implement it.

Core capabilities:
- Parses spec into dependency graph (DAG)
- Spawns parallel agents in isolated git worktrees
- Validates each task with 4-layer verification
- Handles merge conflicts intelligently
- Iterates until implementation matches spec exactly

Available commands:
  version    Show version information
  implement  Implement a specification
  audit      Audit implementation against spec
  init       Initialize alphie in a project
  cleanup    Clean up orphaned worktrees
  help       Help about any command

Use "alphie [command] --help" for more information about a command.`,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Set version for --version flag
	rootCmd.Version = Version()

	// Add global persistent flags
	rootCmd.PersistentFlags().BoolVar(&greenfieldEnabled, "greenfield", false, "Greenfield mode: merge directly to main (no session branch)")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(implementCmd)
	rootCmd.AddCommand(auditCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(cleanupCmd)
}
