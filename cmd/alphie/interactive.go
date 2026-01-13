package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/internal/config"
	"github.com/shayc/alphie/internal/learning"
	"github.com/shayc/alphie/internal/orchestrator"
	"github.com/shayc/alphie/internal/prog"
	"github.com/shayc/alphie/internal/state"
	"github.com/shayc/alphie/internal/tui"
	"github.com/shayc/alphie/pkg/models"
)

func runInteractive() error {
	if err := CheckClaudeCLI(); err != nil {
		return err
	}

	// Get repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Load tier configs
	tierConfigs, err := config.LoadTierConfigs(filepath.Join(repoPath, "configs"))
	if err != nil {
		tierConfigs = config.DefaultTierConfigs()
	}

	// Initialize state database
	stateDB, err := state.OpenProject(repoPath)
	if err != nil {
		return fmt.Errorf("open state database: %w", err)
	}
	defer stateDB.Close()

	if err := stateDB.Migrate(); err != nil {
		return fmt.Errorf("migrate database: %w", err)
	}

	// Initialize learning system
	learningsDBPath := filepath.Join(repoPath, ".alphie", "learnings.db")
	learningSys, err := learning.NewLearningSystem(learningsDBPath)
	if err != nil {
		learningSys = nil
	}
	if learningSys != nil {
		defer learningSys.Close()
	}

	// Initialize prog client
	projectName := filepath.Base(repoPath)
	progClient, err := prog.NewClientDefault(projectName)
	if err != nil {
		progClient = nil
	}
	if progClient != nil {
		defer progClient.Close()
	}

	// Create worktree manager for cleanup
	wtManager, err := agent.NewWorktreeManager("", repoPath)
	if err != nil {
		return fmt.Errorf("create worktree manager: %w", err)
	}

	// Startup cleanup: remove orphaned worktrees from previous interrupted sessions
	activeSessions, _ := getActiveSessions() // Ignore errors, use empty list if query fails
	if removed, err := wtManager.StartupCleanup(activeSessions); err == nil && removed > 0 {
		log.Printf("[interactive] cleaned up %d orphaned worktree(s) from previous sessions", removed)
	}

	// Create executor (use sonnet as default model)
	executor, err := agent.NewExecutor(agent.ExecutorConfig{
		RepoPath: repoPath,
		Model:    "sonnet",
	})
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}

	// Create orchestrator pool config
	poolCfg := orchestrator.PoolConfig{
		RepoPath:       repoPath,
		TierConfigs:    tierConfigs,
		Greenfield:     interactiveGreenfield,
		Executor:       executor,
		StateDB:        stateDB,
		LearningSystem: learningSys,
		ProgClient:     progClient,
	}

	pool := orchestrator.NewOrchestratorPool(poolCfg)

	// Set up signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("[interactive] received shutdown signal")
		pool.Stop()
		cancel()
	}()

	// Suppress log output while TUI is active
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	// Create TUI program
	program, app := tui.NewInteractiveProgram()

	// Create quick executor for !quick tasks
	quickExec := orchestrator.NewQuickExecutor(repoPath)

	// Track active tasks to know when ALL are done
	var activeTaskCount int32

	// Set task submit handler (runs async to avoid blocking TUI)
	app.SetTaskSubmitHandler(func(task string, tier models.Tier) {
		// Quick mode: single agent, no decomposition, direct execution
		if tier == models.TierQuick {
			atomic.AddInt32(&activeTaskCount, 1)
			go func() {
				defer atomic.AddInt32(&activeTaskCount, -1)

				program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Quick: %s", task)})

				result, err := quickExec.Execute(ctx, task)
				if err != nil {
					program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Quick failed: %v", err)})
					return
				}

				if !result.Success {
					msg := fmt.Sprintf("Quick failed: %s", result.Error)
					if result.LogFile != "" {
						msg += fmt.Sprintf(" [log: %s]", result.LogFile)
					}
					program.Send(tui.DebugLogMsg{Message: msg})
					return
				}

				msg := fmt.Sprintf("Quick done! (%s, ~%d tokens, $%.4f)",
					result.Duration.Round(100*time.Millisecond),
					result.TokensUsed,
					result.Cost)
				if result.LogFile != "" {
					msg += fmt.Sprintf(" [log: %s]", result.LogFile)
				}
				program.Send(tui.DebugLogMsg{Message: msg})
			}()
			return
		}

		// Submit async to avoid blocking the TUI
		atomic.AddInt32(&activeTaskCount, 1)
		go func() {
			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Queuing: %s (tier: %s)", task, tier)})

			_, err := pool.Submit(task, tier)
			if err != nil {
				program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Failed to submit task: %v", err)})
				atomic.AddInt32(&activeTaskCount, -1)
				return
			}

			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Started: %s", task)})
		}()
	})

	// Set task retry handler
	app.SetTaskRetryHandler(func(taskID, taskTitle string, tier models.Tier) {
		atomic.AddInt32(&activeTaskCount, 1)
		go func() {
			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Retrying: %s", taskTitle)})

			_, err := pool.Submit(taskTitle, tier)
			if err != nil {
				program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Failed to retry task: %v", err)})
				atomic.AddInt32(&activeTaskCount, -1)
				return
			}

			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Retry started: %s", taskTitle)})
		}()
	})

	// Forward events from pool to TUI
	go forwardPoolEventsToTUI(ctx, pool, program, &activeTaskCount)

	// If resume flag is set, load and submit incomplete tasks
	if interactiveResume && progClient != nil {
		go resumeIncompleteTasks(program, pool, progClient)
	}

	// Run TUI
	_, err = program.Run()
	if err != nil {
		return fmt.Errorf("run TUI: %w", err)
	}

	// Stop pool on exit
	if err := pool.Stop(); err != nil {
		log.Printf("[interactive] warning: failed to stop pool: %v", err)
	}

	// Final cleanup: ensure all worktrees are cleaned up on exit
	activeSessions, _ = getActiveSessions()
	if removed, err := wtManager.CleanupOrphans(activeSessions, nil); err == nil && removed > 0 {
		log.Printf("[interactive] cleaned up %d worktree(s) on exit", removed)
	}

	return nil
}

func resumeIncompleteTasks(program *tea.Program, pool *orchestrator.OrchestratorPool, progClient *prog.Client) {
	// Find in-progress epics
	epics, err := progClient.ListInProgressEpics()
	if err != nil {
		program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Failed to list epics: %v", err)})
		return
	}

	if len(epics) == 0 {
		program.Send(tui.DebugLogMsg{Message: "No in-progress tasks to resume"})
		return
	}

	// For each epic, get incomplete tasks and submit them
	for _, epic := range epics {
		tasks, err := progClient.GetIncompleteTasks(epic.ID)
		if err != nil {
			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Failed to get tasks for epic %s: %v", epic.ID, err)})
			continue
		}

		for _, task := range tasks {
			// Auto-detect tier from task title
			tier, cleanTask := tui.ClassifyTier(task.Title)

			_, err := pool.Submit(cleanTask, tier)
			if err != nil {
				program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Failed to resume task: %v", err)})
				continue
			}

			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Resumed: %s (tier: %s)", cleanTask, tier)})
		}
	}
}

func forwardPoolEventsToTUI(ctx context.Context, pool *orchestrator.OrchestratorPool, program *tea.Program, activeTaskCount *int32) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-pool.Events():
			if !ok {
				return
			}

			// Skip session_done events in interactive mode - users can keep submitting tasks
			if event.Type == orchestrator.EventSessionDone {
				continue
			}

			msg := tui.OrchestratorEventMsg{
				Type:          string(event.Type),
				TaskID:        event.TaskID,
				TaskTitle:     event.TaskTitle,
				AgentID:       event.AgentID,
				Message:       event.Message,
				Timestamp:     event.Timestamp,
				TokensUsed:    event.TokensUsed,
				Cost:          event.Cost,
				Duration:      event.Duration,
				LogFile:       event.LogFile,
				CurrentAction: event.CurrentAction,
			}
			if event.Error != nil {
				msg.Error = event.Error.Error()
			}

			program.Send(msg)

			// Track task completion
			if event.Type == orchestrator.EventTaskCompleted || event.Type == orchestrator.EventTaskFailed {
				atomic.AddInt32(activeTaskCount, -1)
			}
		}
	}
}
