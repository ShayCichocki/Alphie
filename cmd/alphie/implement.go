package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ShayCichocki/alphie/internal/architect"
	"github.com/ShayCichocki/alphie/internal/tui"
	"github.com/spf13/cobra"
)

var (
	implementAgents          int
	implementMaxIterations   int
	implementBudget          float64
	implementNoConvergeAfter int
	implementDryRun          bool
	implementResume          bool
	implementProject         string
	implementUseCLI          bool
)

var implementCmd = &cobra.Command{
	Use:   "implement <spec.md|spec.xml>",
	Short: "Implement architecture specification iteratively",
	Long: `Implement an architecture specification by iterating through audit-plan-execute cycles.

This command orchestrates the full architecture implementation loop:
  1. Parse the architecture document to extract features/requirements
  2. Audit the codebase to identify gaps (MISSING or PARTIAL implementations)
  3. Plan epics and tasks from identified gaps
  4. Execute tasks using parallel agents
  5. Repeat until complete or stop conditions are met

Supported formats:
  - Markdown (.md) - Standard markdown with sections and headers
  - XML (.xml) - Custom XML schemas with features/requirements

Stop conditions:
  - All features implemented (100% completion)
  - Maximum iterations reached (--max-iterations)
  - Budget exceeded (--budget)
  - No progress for N iterations (--no-converge-after)

Examples:
  alphie implement docs/architecture.md                    # Markdown spec
  alphie implement spec.xml                                # XML spec
  alphie implement spec.md --agents 5                      # Use 5 concurrent agents
  alphie implement spec.md --max-iterations 20             # Allow more iterations
  alphie implement spec.md --budget 10.00                  # Cap cost at $10
  alphie implement spec.md --dry-run                       # Show plan without executing
  alphie implement spec.md --project myproject             # Use specific prog project`,
	Args: cobra.ExactArgs(1),
	RunE: runImplement,
}

func init() {
	implementCmd.Flags().IntVar(&implementAgents, "agents", 3, "Max concurrent workers")
	implementCmd.Flags().IntVar(&implementMaxIterations, "max-iterations", 10, "Hard cap on iterations")
	implementCmd.Flags().Float64Var(&implementBudget, "budget", 0, "Cost limit in dollars (0 = unlimited)")
	implementCmd.Flags().IntVar(&implementNoConvergeAfter, "no-converge-after", 3, "Stop if no progress for N iterations")
	implementCmd.Flags().BoolVar(&implementDryRun, "dry-run", false, "Show plan without executing")
	implementCmd.Flags().BoolVar(&implementResume, "resume", false, "Resume from checkpoint")
	implementCmd.Flags().StringVar(&implementProject, "project", "", "Prog project name (defaults to directory name)")
	implementCmd.Flags().BoolVar(&implementUseCLI, "cli", false, "Use Claude CLI subprocess instead of API")
}

func runImplement(cmd *cobra.Command, args []string) error {
	archDoc := args[0]

	// Verify architecture document exists
	if _, err := os.Stat(archDoc); os.IsNotExist(err) {
		return fmt.Errorf("architecture document not found: %s", archDoc)
	}

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Determine project name
	projectName := implementProject
	if projectName == "" {
		// Use directory name as default project name
		projectName = repoPath[strings.LastIndex(repoPath, "/")+1:]
	}

	// Check for Claude CLI
	if err := CheckClaudeCLI(); err != nil {
		return err
	}

	// Display configuration
	fmt.Println("=== Alphie Implement ===")
	fmt.Println()
	fmt.Printf("Architecture: %s\n", archDoc)
	fmt.Printf("Repository:   %s\n", repoPath)
	fmt.Printf("Project:      %s\n", projectName)
	fmt.Println()
	fmt.Println("Configuration:")
	fmt.Printf("  Agents:           %d\n", implementAgents)
	fmt.Printf("  Max iterations:   %d\n", implementMaxIterations)
	if implementBudget > 0 {
		fmt.Printf("  Budget:           $%.2f\n", implementBudget)
	} else {
		fmt.Printf("  Budget:           unlimited\n")
	}
	fmt.Printf("  No-converge:      %d iterations\n", implementNoConvergeAfter)
	fmt.Printf("  Dry-run:          %v\n", implementDryRun)
	fmt.Printf("  Resume:           %v\n", implementResume)
	fmt.Println()

	// Handle dry-run mode
	if implementDryRun {
		return runImplementDryRun(archDoc, repoPath)
	}

	// Handle resume mode (placeholder for future implementation)
	if implementResume {
		fmt.Println("Note: Resume mode not yet fully implemented, starting fresh")
	}

	// Create TUI program
	program, _ := tui.NewImplementProgram()

	// Create context that can be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	progressCallback := func(event architect.ProgressEvent) {
		phaseStr := string(event.Phase)

		// Convert architect.WorkerInfo to tui.WorkerInfo
		activeWorkers := make(map[string]tui.WorkerInfo)
		for k, v := range event.ActiveWorkers {
			activeWorkers[k] = tui.WorkerInfo{
				AgentID:   v.AgentID,
				TaskID:    v.TaskID,
				TaskTitle: v.TaskTitle,
				Status:    v.Status,
			}
		}

		program.Send(tui.ImplementUpdateMsg{
			State: tui.ImplementState{
				Iteration:        event.Iteration,
				MaxIterations:    event.MaxIterations,
				FeaturesComplete: event.FeaturesComplete,
				FeaturesTotal:    event.FeaturesTotal,
				Cost:             event.Cost,
				CostBudget:       event.CostBudget,
				CurrentPhase:     phaseStr,
				WorkersRunning:   event.WorkersRunning,
				WorkersBlocked:   event.WorkersBlocked,
				ActiveWorkers:    activeWorkers,
			},
		})

		// Send log entry
		program.Send(tui.ImplementLogMsg{
			Timestamp: event.Timestamp,
			Phase:     phaseStr,
			Message:   event.Message,
		})
	}

	// Create runner factory (CLI subprocess or API)
	runnerFactory, err := createRunnerFactory(implementUseCLI)
	if err != nil {
		return fmt.Errorf("create runner factory: %w", err)
	}

	// Create and configure the controller
	controller := architect.NewController(
		implementMaxIterations,
		implementBudget,
		implementNoConvergeAfter,
		architect.WithRepoPath(repoPath),
		architect.WithProjectName(projectName),
		architect.WithProgressCallback(progressCallback),
		architect.WithRunnerFactory(runnerFactory),
	)

	// Run controller in background goroutine
	go func() {
		err := controller.Run(ctx, archDoc, implementAgents)
		program.Send(tui.ImplementDoneMsg{Err: err})
	}()

	// Run TUI (blocks until quit)
	if _, err := program.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

// runImplementDryRun shows what would be done without executing.
func runImplementDryRun(archDoc, repoPath string) error {
	fmt.Println("=== Dry Run Mode ===")
	fmt.Println()
	fmt.Println("Would perform the following steps:")
	fmt.Println()
	fmt.Printf("1. Parse architecture document: %s\n", archDoc)
	fmt.Println("   - Extract features and requirements")
	fmt.Println("   - Identify acceptance criteria")
	fmt.Println()
	fmt.Printf("2. Audit codebase: %s\n", repoPath)
	fmt.Println("   - Compare features against implementation")
	fmt.Println("   - Identify MISSING and PARTIAL features")
	fmt.Println("   - Generate gap report")
	fmt.Println()
	fmt.Println("3. Plan implementation:")
	fmt.Println("   - Group gaps into phases (Foundation, Refinement)")
	fmt.Println("   - Create prog epic for implementation")
	fmt.Println("   - Create tasks with dependencies")
	fmt.Println()
	fmt.Printf("4. Execute with %d concurrent agents\n", implementAgents)
	fmt.Println("   - Process tasks in dependency order")
	fmt.Println("   - Run /alphie skill pattern for each epic")
	fmt.Println()
	fmt.Println("5. Iterate until stop condition:")
	fmt.Printf("   - Max iterations: %d\n", implementMaxIterations)
	if implementBudget > 0 {
		fmt.Printf("   - Budget limit: $%.2f\n", implementBudget)
	}
	fmt.Printf("   - Convergence: %d iterations without progress\n", implementNoConvergeAfter)
	fmt.Println("   - Or: 100% completion")
	fmt.Println()
	fmt.Println("No changes made (dry-run mode)")
	return nil
}
