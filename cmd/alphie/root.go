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
	Short: "Spec-driven development orchestrator",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true, // Disable auto-generated completion command
	},
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
