package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/spf13/cobra"
)

const baselinePath = ".alphie/baseline.json"

var baselineCmd = &cobra.Command{
	Use:   "baseline [show|capture|reset]",
	Short: "Show or reset baseline snapshot",
	Long: `Manage the baseline snapshot for quality gates.

At session start, Alphie captures which tests/lints currently fail.
This baseline is used to enforce "no regressions" during the session.

Commands:
  alphie baseline          # Show current baseline
  alphie baseline show     # Show current baseline
  alphie baseline capture  # Capture new baseline
  alphie baseline reset    # Delete baseline

Baseline enforcement:
  - Pre-existing failures are allowed (not agent's fault)
  - New failures are blocked (agent must fix)
  - Worsening existing failures is blocked`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		subcommand := "show"
		if len(args) > 0 {
			subcommand = args[0]
		}

		switch subcommand {
		case "show":
			showBaseline()
		case "capture":
			captureBaseline()
		case "reset":
			resetBaseline()
		default:
			fmt.Fprintf(os.Stderr, "Unknown subcommand: %s\n", subcommand)
			fmt.Fprintln(os.Stderr, "Use: show, capture, or reset")
			os.Exit(1)
		}
	},
}

func showBaseline() {
	baseline, err := agent.LoadBaseline(baselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No baseline captured yet.")
			fmt.Println("Run 'alphie baseline capture' to capture a baseline.")
			return
		}
		fmt.Fprintf(os.Stderr, "Error loading baseline: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Baseline captured at: %s\n", baseline.CapturedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Failing tests: %d\n", len(baseline.FailingTests))
	for _, t := range baseline.FailingTests {
		fmt.Printf("  - %s\n", t)
	}
	fmt.Printf("Lint errors: %d\n", len(baseline.LintErrors))
	for _, e := range baseline.LintErrors {
		fmt.Printf("  - %s\n", e)
	}
	fmt.Printf("Type errors: %d\n", len(baseline.TypeErrors))
	for _, e := range baseline.TypeErrors {
		fmt.Printf("  - %s\n", e)
	}
}

func captureBaseline() {
	fmt.Println("Capturing baseline...")

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	baseline, err := agent.CaptureBaseline(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error capturing baseline: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d failing tests, %d lint errors, %d type errors\n",
		len(baseline.FailingTests), len(baseline.LintErrors), len(baseline.TypeErrors))

	// Ensure .alphie directory exists
	dir := filepath.Dir(baselinePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	if err := baseline.Save(baselinePath); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving baseline: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Baseline saved to %s\n", baselinePath)
}

func resetBaseline() {
	if err := os.Remove(baselinePath); err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No baseline to delete.")
			return
		}
		fmt.Fprintf(os.Stderr, "Error deleting baseline: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Baseline deleted.")
}
