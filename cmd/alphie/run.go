package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/internal/config"
	"github.com/shayc/alphie/internal/learning"
	"github.com/shayc/alphie/internal/orchestrator"
	"github.com/shayc/alphie/internal/prog"
	"github.com/shayc/alphie/internal/state"
	"github.com/shayc/alphie/internal/tui"
	"github.com/shayc/alphie/pkg/models"
)

var (
	runTier       string
	runGreenfield bool
	runHeadless   bool
	runEpicID     string
)

var runCmd = &cobra.Command{
	Use:   "run <task>",
	Short: "Run a task with agent orchestration",
	Long: `Run a task using parallel Claude Code agents.

The task will be decomposed into parallelizable subtasks,
each executed in an isolated git worktree. Agents self-improve
code via the Ralph-loop (critique, improve, repeat).

Tier selection (--tier):
  - quick:     Single agent, no decomposition, direct execution
  - scout:     Fast exploration, 2 agents, haiku model
  - builder:   Standard work, 3 agents, sonnet model (default)
  - architect: Complex work, 5 agents, opus model

Use --tier quick for simple tasks (typo fixes, color changes, renames).
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

	// Determine tier: use explicit flag or auto-select based on task signals
	var tier models.Tier
	if cmd.Flags().Changed("tier") {
		// User explicitly set the tier flag
		tier = models.Tier(runTier)
		if !tier.Valid() {
			return fmt.Errorf("invalid tier %q: must be quick, scout, builder, or architect", runTier)
		}
	} else {
		// Auto-select tier based on task description signals
		tier = orchestrator.SelectTier(taskDescription)
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

	// Create executor
	if verbose {
		fmt.Println("[DEBUG] Creating executor...")
	}
	executor, err := agent.NewExecutor(agent.ExecutorConfig{
		RepoPath: repoPath,
		Model:    model,
	})
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}
	if verbose {
		fmt.Println("[DEBUG] Executor created")
	}

	// Create Claude processes for decomposer and merger
	decomposerClaude := agent.NewClaudeProcess(ctx)
	mergerClaude := agent.NewClaudeProcess(ctx)
	secondReviewerClaude := agent.NewClaudeProcess(ctx)

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
	maxAgents := maxAgentsFromTierConfigs(tier, tierConfigs)
	if verbose {
		fmt.Printf("[DEBUG] Max agents: %d\n", maxAgents)
	}

	// Create orchestrator
	if verbose {
		fmt.Println("[DEBUG] Creating orchestrator...")
		fmt.Printf("[DEBUG]   ResumeEpicID: %q\n", runEpicID)
		fmt.Printf("[DEBUG]   Greenfield: %v\n", runGreenfield)
	}
	orch := orchestrator.NewOrchestrator(orchestrator.OrchestratorConfig{
		RepoPath:             repoPath,
		Tier:                 tier,
		MaxAgents:            maxAgents,
		TierConfigs:          tierConfigs,
		Greenfield:           runGreenfield,
		DecomposerClaude:     decomposerClaude,
		MergerClaude:         mergerClaude,
		SecondReviewerClaude: secondReviewerClaude,
		Executor:             executor,
		StateDB:              db,
		LearningSystem:       learningSystem,
		ProgClient:           progClient,
		ResumeEpicID:         runEpicID,
	})
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

// modelForTier returns the Claude model to use for a given tier.
func modelForTier(tier models.Tier) string {
	switch tier {
	case models.TierQuick:
		return "claude-haiku-3-5-20241022"
	case models.TierScout:
		return "claude-haiku-3-5-20241022"
	case models.TierBuilder:
		return "claude-sonnet-4-20250514"
	case models.TierArchitect:
		return "claude-opus-4-20250514"
	default:
		return "claude-sonnet-4-20250514"
	}
}

// maxAgentsFromTierConfigs returns the maximum concurrent agents from tier configs.
// Falls back to hardcoded defaults if tier configs are nil.
func maxAgentsFromTierConfigs(tier models.Tier, tierConfigs *config.TierConfigs) int {
	if tierConfigs != nil {
		tc := tierConfigs.Get(tier)
		if tc != nil && tc.MaxAgents > 0 {
			return tc.MaxAgents
		}
	}
	// Fallback to hardcoded defaults
	switch tier {
	case models.TierQuick:
		return 1
	case models.TierScout:
		return 2
	case models.TierBuilder:
		return 3
	case models.TierArchitect:
		return 5
	default:
		return 3
	}
}

// consumeEventsHeadless prints orchestrator events to stdout.
func consumeEventsHeadless(events <-chan orchestrator.OrchestratorEvent) {
	for event := range events {
		switch event.Type {
		case orchestrator.EventTaskStarted:
			agentShort := event.AgentID
			if len(agentShort) > 8 {
				agentShort = agentShort[:8]
			}
			fmt.Printf("[STARTED] %s (agent: %s)\n", event.Message, agentShort)
		case orchestrator.EventTaskCompleted:
			fmt.Printf("[DONE] %s\n", event.Message)
		case orchestrator.EventTaskFailed:
			fmt.Printf("[FAILED] %s: %v\n", event.Message, event.Error)
		case orchestrator.EventMergeStarted:
			fmt.Printf("[MERGE] %s\n", event.Message)
		case orchestrator.EventMergeCompleted:
			fmt.Printf("[MERGED] %s\n", event.Message)
		case orchestrator.EventSessionDone:
			fmt.Printf("[SESSION] %s\n", event.Message)
		case orchestrator.EventTaskBlocked:
			fmt.Printf("[BLOCKED] %s: %v\n", event.Message, event.Error)
		}
	}
}

// runWithTUI runs the orchestrator with an interactive TUI.
func runWithTUI(ctx context.Context, orch *orchestrator.Orchestrator, task string) (retErr error) {
	verbose := os.Getenv("ALPHIE_DEBUG") != ""

	// Suppress log output while TUI is active (it corrupts the display)
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	// Recover from panics
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("PANIC in runWithTUI: %v", r)
		}
	}()

	if verbose {
		fmt.Println("[DEBUG] runWithTUI: Creating TUI program...")
	}

	program, app := tui.NewPanelProgram()
	if program == nil {
		return fmt.Errorf("failed to create TUI program (nil)")
	}
	if app == nil && verbose {
		fmt.Println("[DEBUG] Warning: TUI app is nil")
	}

	if verbose {
		fmt.Println("[DEBUG] runWithTUI: TUI program created")
	}

	// Channel to signal orchestrator completion
	orchDone := make(chan error, 1)

	// Start event forwarding goroutine
	if verbose {
		fmt.Println("[DEBUG] runWithTUI: Starting event forwarding...")
	}
	go forwardEventsToTUI(program, orch.Events())

	// Start orchestrator in background
	if verbose {
		fmt.Println("[DEBUG] runWithTUI: Starting orchestrator...")
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				orchDone <- fmt.Errorf("PANIC in orchestrator: %v", r)
			}
		}()
		orchDone <- orch.Run(ctx, task)
	}()

	if verbose {
		fmt.Println("[DEBUG] runWithTUI: Starting TUI, switching to alt-screen...")
	}

	tuiDone := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				tuiDone <- fmt.Errorf("PANIC in TUI: %v", r)
			}
		}()
		_, err := program.Run()
		tuiDone <- err
	}()

	debugLog := func(msg string) {
		if verbose {
			program.Send(tui.DebugLogMsg{Message: msg})
		}
	}

	debugLog("TUI started, waiting for completion...")

	// Wait for either completion
	select {
	case err := <-orchDone:
		debugLog(fmt.Sprintf("Orchestrator done, err=%v", err))
		// Orchestrator finished - send session done message
		if err != nil {
			program.Send(tui.SessionDoneMsg{Success: false, Message: err.Error()})
		} else {
			program.Send(tui.SessionDoneMsg{Success: true, Message: "Task completed successfully"})
		}
		// Wait for user to quit TUI (press q) so they can see the result
		<-tuiDone
		return err

	case err := <-tuiDone:
		if verbose {
			fmt.Printf("[DEBUG] runWithTUI: TUI done, err=%v\n", err)
		}
		return err
	}
}

// forwardEventsToTUI converts orchestrator events to TUI messages.
func forwardEventsToTUI(program *tea.Program, events <-chan orchestrator.OrchestratorEvent) {
	for event := range events {
		// Convert to TUI message
		errStr := ""
		if event.Error != nil {
			errStr = event.Error.Error()
		}
		msg := tui.OrchestratorEventMsg{
			Type:          string(event.Type),
			TaskID:        event.TaskID,
			TaskTitle:     event.TaskTitle,
			AgentID:       event.AgentID,
			Message:       event.Message,
			Error:         errStr,
			Timestamp:     event.Timestamp,
			TokensUsed:    event.TokensUsed,
			Cost:          event.Cost,
			Duration:      event.Duration,
			LogFile:       event.LogFile,
			CurrentAction: event.CurrentAction,
		}
		program.Send(msg)
	}
}

// runQuickMode executes a task directly without orchestration.
// Uses QuickExecutor for fast, single-agent execution on the current branch.
func runQuickMode(ctx context.Context, repoPath, task string, verbose bool) error {
	if verbose {
		fmt.Printf("[DEBUG] Quick mode task: %s\n", task)
	}

	fmt.Printf("Quick mode: %s\n", task)

	// Create and run quick executor
	executor := orchestrator.NewQuickExecutor(repoPath)
	result, err := executor.Execute(ctx, task)
	if err != nil {
		return fmt.Errorf("quick execution failed: %w", err)
	}

	// Print result
	if result.Output != "" {
		fmt.Println(result.Output)
	}

	if !result.Success {
		fmt.Printf("\nQuick mode failed: %s\n", result.Error)
		return fmt.Errorf("quick mode failed: %s", result.Error)
	}

	fmt.Printf("\nDone! (%s, ~%d tokens, $%.4f)\n",
		result.Duration.Round(100*time.Millisecond),
		result.TokensUsed,
		result.Cost)
	return nil
}

// checkAndReportResumableSessions checks for and reports resumable sessions from prog state.
// This is called on startup when not explicitly resuming an epic.
func checkAndReportResumableSessions(progClient *prog.Client, repoPath string) error {
	// Create worktree manager for orphan detection
	wtManager, err := agent.NewWorktreeManager("", repoPath)
	if err != nil {
		// Non-fatal - continue without worktree management
		wtManager = nil
	}

	// Create session resume checker
	checker := orchestrator.NewSessionResumeChecker(progClient, wtManager, repoPath)

	// Check for resumable sessions
	result, err := checker.CheckForResumableSessions()
	if err != nil {
		return fmt.Errorf("check for resumable sessions: %w", err)
	}

	// Report any resumable sessions found
	if result.HasResumableSessions {
		suggestion := orchestrator.FormatResumeSuggestion(result)
		if suggestion != "" {
			fmt.Print(suggestion)
		}
	}

	// Perform startup worktree cleanup
	if wtManager != nil && len(result.OrphanedWorktrees) > 0 {
		// Auto-cleanup orphaned worktrees at startup
		cleaned, err := checker.CleanupOrphanedWorktrees(func(path string) {
			fmt.Printf("Cleaned up orphaned worktree: %s\n", path)
		})
		if err != nil {
			fmt.Printf("Warning: worktree cleanup encountered errors: %v\n", err)
		} else if cleaned > 0 {
			fmt.Printf("Cleaned up %d orphaned worktree(s)\n", cleaned)
		}
	}

	// If resuming an epic would be possible, reconcile worktree state
	if result.RecommendedEpicID != "" && wtManager != nil {
		if err := checker.ReconcileProgStateWithWorktrees(result.RecommendedEpicID); err != nil {
			fmt.Printf("Warning: failed to reconcile prog state with worktrees: %v\n", err)
		}
	}

	return nil
}
