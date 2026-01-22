package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/config"
	"github.com/ShayCichocki/alphie/internal/learning"
	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/ShayCichocki/alphie/pkg/models"
)

var (
	runTier        string
	runGreenfield  bool
	runHeadless    bool
	runEpicID      string
	runQuick       bool
	runParallel    bool
	runSingle      bool
	runPassthrough bool
)

var runCmd = &cobra.Command{
	Use:   "run <task>",
	Short: "Run a task with agent orchestration",
	Long: `Run a task using parallel Claude Code agents.

The task will be decomposed into parallelizable subtasks,
each executed in an isolated git worktree. Agents self-improve
code via the Ralph-loop (critique, improve, repeat).

Execution modes (auto-detected by default):
  --quick      Force single-agent mode (no decomposition)
  --parallel   Force parallel decomposition mode
  --single     Force single-agent with decomposition (one task at a time)

Tier selection (--tier):
  - quick:     Single agent, no decomposition, direct execution
  - scout:     Fast exploration, 2 agents, haiku model
  - builder:   Standard work, 3 agents, sonnet model (default)
  - architect: Complex work, 5 agents, opus model

Auto-detection routes:
  - Setup/scaffolding work → Quick mode (faster, no merge conflicts)
  - Bug fixes → Quick mode (focused single-agent)
  - Feature work → Parallel mode (decomposed into independent tasks)
  - Refactoring → Depends on scope

Use --greenfield for new projects to merge directly to main.

Cross-session continuity:
  Use --epic <id> to resume an incomplete epic from a previous session.
  Completed tasks will be skipped, and remaining tasks will be executed.
  Run 'prog list -p <project> --type epic' to see available epics.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runTask,
}

func init() {
	runCmd.Flags().StringVar(&runTier, "tier", "builder", "Agent tier: quick, scout, builder, or architect")
	runCmd.Flags().BoolVar(&runGreenfield, "greenfield", false, "Direct merge to main (skip session branch)")
	runCmd.Flags().BoolVar(&runHeadless, "headless", false, "Run without TUI (headless mode)")
	runCmd.Flags().StringVar(&runEpicID, "epic", "", "Resume an existing prog epic by ID (cross-session continuity)")
	runCmd.Flags().BoolVar(&runQuick, "quick", false, "Force quick mode: single agent, no decomposition")
	runCmd.Flags().BoolVar(&runParallel, "parallel", false, "Force parallel mode: decompose and run multiple agents")
	runCmd.Flags().BoolVar(&runSingle, "single", false, "Force single mode: decompose but run one agent at a time")
	runCmd.Flags().BoolVar(&runPassthrough, "passthrough", false, "Bypass orchestration, run Claude directly (debugging/cost control)")
}

func runTask(cmd *cobra.Command, args []string) (retErr error) {
	// Recover from panics and report them
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("PANIC in runTask: %v", r)
		}
	}()

	taskDescription := args[0]
	verbose := os.Getenv("ALPHIE_DEBUG") != ""

	if verbose {
		fmt.Println("[DEBUG] Starting runTask...")
		fmt.Printf("[DEBUG] Task: %s\n", taskDescription)
		fmt.Printf("[DEBUG] Epic ID: %q\n", runEpicID)
		fmt.Printf("[DEBUG] Tier: %s\n", runTier)
		fmt.Printf("[DEBUG] Headless: %v\n", runHeadless)
	}

	// Check that Claude CLI is available
	if verbose {
		fmt.Println("[DEBUG] Checking Claude CLI...")
	}
	if err := CheckClaudeCLI(); err != nil {
		return err
	}
	if verbose {
		fmt.Println("[DEBUG] Claude CLI OK")
	}

	// Passthrough mode: bypass all orchestration, run Claude directly
	if runPassthrough {
		if verbose {
			fmt.Println("[DEBUG] Running in passthrough mode...")
		}
		repoPath, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		return runPassthroughMode(context.Background(), repoPath, taskDescription, verbose)
	}

	// Get current working directory as repo path (needed early for quick mode)
	if verbose {
		fmt.Println("[DEBUG] Getting working directory...")
	}
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	if verbose {
		fmt.Printf("[DEBUG] Repo path: %s\n", repoPath)
	}

	// Create context with cancellation for all modes
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, shutting down...")
		cancel()
	}()

	// Determine execution mode: check explicit flags first, then auto-detect
	var tier models.Tier
	var forceMaxAgents int // 0 means use tier default

	// Check for explicit mode flags (mutually exclusive)
	modeCount := 0
	if runQuick {
		modeCount++
	}
	if runParallel {
		modeCount++
	}
	if runSingle {
		modeCount++
	}
	if modeCount > 1 {
		return fmt.Errorf("--quick, --parallel, and --single are mutually exclusive")
	}

	// Handle explicit mode flags
	if runQuick {
		// Force quick mode regardless of task analysis
		tier = models.TierQuick
		if verbose {
			fmt.Println("[DEBUG] Forced quick mode via --quick flag")
		}
	} else if runSingle {
		// Force single-agent decomposition mode
		if cmd.Flags().Changed("tier") {
			tier = models.Tier(runTier)
		} else {
			tier = models.TierBuilder // Default to builder for single mode
		}
		forceMaxAgents = 1 // Run one agent at a time
		if verbose {
			fmt.Println("[DEBUG] Forced single mode via --single flag")
		}
	} else if runParallel {
		// Force parallel mode, skip auto-quick-detection
		if cmd.Flags().Changed("tier") {
			tier = models.Tier(runTier)
		} else {
			tier = models.TierBuilder // Default to builder for parallel mode
		}
		if verbose {
			fmt.Println("[DEBUG] Forced parallel mode via --parallel flag")
		}
	} else if cmd.Flags().Changed("tier") {
		// User explicitly set the tier flag
		tier = models.Tier(runTier)
		if !tier.Valid() {
			return fmt.Errorf("invalid tier %q: must be quick, scout, builder, or architect", runTier)
		}
	} else {
		// Auto-select tier based on task description signals
		// This uses RequestAnalyzer to route setup/bugfix → quick mode
		tier = orchestrator.SelectTier(taskDescription)
		if verbose {
			fmt.Printf("[DEBUG] Auto-selected tier: %s\n", tier)
		}
	}

	// Quick mode: single agent, no decomposition, direct execution
	if tier == models.TierQuick {
		if verbose {
			fmt.Println("[DEBUG] Running in quick mode...")
		}
		return runQuickMode(ctx, repoPath, taskDescription, verbose)
	}

	// Open state database
	if verbose {
		fmt.Println("[DEBUG] Opening state database...")
	}
	db, err := state.OpenProject(repoPath)
	if err != nil {
		return fmt.Errorf("open state database: %w", err)
	}
	defer db.Close()
	if verbose {
		fmt.Println("[DEBUG] State database opened")
	}

	// Run migrations
	if verbose {
		fmt.Println("[DEBUG] Running migrations...")
	}
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}
	if verbose {
		fmt.Println("[DEBUG] Migrations complete")
	}

	// Initialize prog client for cross-session task management
	// Use the repo directory name as the project identifier
	projectName := filepath.Base(repoPath)
	if verbose {
		fmt.Printf("[DEBUG] Initializing prog client for project: %s\n", projectName)
	}
	progClient, err := prog.NewClientDefault(projectName)
	if err != nil {
		// Prog client is optional - log warning and continue
		fmt.Printf("Warning: prog client unavailable: %v\n", err)
		progClient = nil
	} else {
		defer progClient.Close()
		if verbose {
			fmt.Println("[DEBUG] Prog client initialized")
		}
	}

	// Check for resumable sessions if not explicitly resuming an epic
	if runEpicID == "" && progClient != nil {
		if verbose {
			fmt.Println("[DEBUG] Checking for resumable sessions...")
		}
		if err := checkAndReportResumableSessions(progClient, repoPath); err != nil {
			// Non-fatal - just log the warning
			fmt.Printf("Warning: session recovery check failed: %v\n", err)
		}
	} else if runEpicID != "" && verbose {
		fmt.Printf("[DEBUG] Resuming epic: %s\n", runEpicID)
	}

	// Determine model based on tier
	model := modelForTier(tier)
	if verbose {
		fmt.Printf("[DEBUG] Selected model: %s\n", model)
	}

	// Create runner factory for direct API calls (needed by executor)
	runnerFactory, err := createRunnerFactory()
	if err != nil {
		return fmt.Errorf("create runner factory: %w", err)
	}

	// Create executor
	if verbose {
		fmt.Println("[DEBUG] Creating executor...")
	}
	executor, err := agent.NewExecutor(agent.ExecutorConfig{
		RepoPath:      repoPath,
		Model:         model,
		RunnerFactory: runnerFactory,
	})
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}
	if verbose {
		fmt.Println("[DEBUG] Executor created")
	}

	// Create Claude runners for decomposer and merger
	decomposerClaude := runnerFactory.NewRunner()
	mergerClaude := runnerFactory.NewRunner()
	secondReviewerClaude := runnerFactory.NewRunner()
	if verbose {
		fmt.Println("[DEBUG] Using direct Anthropic API")
	}

	// Load tier configurations from YAML (fallback to defaults if missing)
	tierConfigs, err := config.LoadTierConfigs(filepath.Join(repoPath, "configs"))
	if err != nil {
		// Configs not found or invalid - use hardcoded defaults
		tierConfigs = config.DefaultTierConfigs()
	}

	// Initialize learning system for auto-learning and retrieval
	learningsDBPath := filepath.Join(repoPath, ".alphie", "learnings.db")
	learningSystem, err := learning.NewLearningSystem(learningsDBPath)
	if err != nil {
		// Learning system is optional - log warning and continue
		fmt.Printf("Warning: learning system unavailable: %v\n", err)
		learningSystem = nil
	}

	// Determine max agents based on tier (from tier configs or fallback)
	// forceMaxAgents overrides the tier default when --single flag is used
	maxAgents := maxAgentsFromTierConfigs(tier, tierConfigs)
	if forceMaxAgents > 0 {
		maxAgents = forceMaxAgents
	}
	if verbose {
		fmt.Printf("[DEBUG] Max agents: %d\n", maxAgents)
	}

	// Create orchestrator
	if verbose {
		fmt.Println("[DEBUG] Creating orchestrator...")
		fmt.Printf("[DEBUG]   ResumeEpicID: %q\n", runEpicID)
		fmt.Printf("[DEBUG]   Greenfield: %v\n", runGreenfield)
	}
	orch := orchestrator.New(
		orchestrator.RequiredConfig{
			RepoPath: repoPath,
			Tier:     tier,
			Executor: executor,
		},
		orchestrator.WithMaxAgents(maxAgents),
		orchestrator.WithTierConfigs(tierConfigs),
		orchestrator.WithGreenfield(runGreenfield),
		orchestrator.WithDecomposerClaude(decomposerClaude),
		orchestrator.WithMergerClaude(mergerClaude),
		orchestrator.WithSecondReviewerClaude(secondReviewerClaude),
		orchestrator.WithRunnerFactory(runnerFactory),
		orchestrator.WithStateDB(db),
		orchestrator.WithLearningSystem(learningSystem),
		orchestrator.WithProgClient(progClient),
		orchestrator.WithResumeEpicID(runEpicID),
	)
	defer orch.Stop()
	if verbose {
		fmt.Println("[DEBUG] Orchestrator created")
	}

	// Run in headless or TUI mode
	if verbose {
		fmt.Printf("[DEBUG] Running in %s mode\n", map[bool]string{true: "headless", false: "TUI"}[runHeadless])
	}
	if runHeadless {
		// Headless mode: print events to stdout
		go consumeEventsHeadless(orch.Events())

		fmt.Printf("Starting task: %s\n", taskDescription)
		fmt.Printf("  Tier: %s\n", tier)
		fmt.Printf("  Max agents: %d\n", maxAgents)
		fmt.Printf("  Greenfield: %v\n", runGreenfield)
		fmt.Println()

		if err := orch.Run(ctx, taskDescription); err != nil {
			return fmt.Errorf("orchestration failed: %w", err)
		}

		fmt.Println("\nTask completed successfully!")
		return nil
	}

	// TUI mode: run with interactive interface
	if verbose {
		fmt.Println("[DEBUG] Starting TUI mode...")
	}
	return runWithTUI(ctx, orch, taskDescription)
}
