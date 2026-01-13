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
		Greenfield:     false,
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

	// Set task submit handler (runs async to avoid blocking TUI)
	app.SetTaskSubmitHandler(func(task string, tier models.Tier) {
		// Quick mode: single agent, no decomposition, direct execution
		if tier == models.TierQuick {
			go func() {
				program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Quick: %s", task)})

				result, err := quickExec.Execute(ctx, task)
				if err != nil {
					program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Quick failed: %v", err)})
					return
				}

				if !result.Success {
					program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Quick failed: %s", result.Error)})
					return
				}

				program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Quick done! (%s, ~%d tokens, $%.4f)",
					result.Duration.Round(100*time.Millisecond),
					result.TokensUsed,
					result.Cost)})
			}()
			return
		}

		// Submit async to avoid blocking the TUI
		go func() {
			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Queuing: %s (tier: %s)", task, tier)})

			_, err := pool.Submit(task, tier)
			if err != nil {
				program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Failed to submit task: %v", err)})
				return
			}

			program.Send(tui.DebugLogMsg{Message: fmt.Sprintf("Started: %s", task)})
		}()
	})

	// Forward events from pool to TUI
	go forwardPoolEventsToTUI(ctx, pool, program)

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

func forwardPoolEventsToTUI(ctx context.Context, pool *orchestrator.OrchestratorPool, program *tea.Program) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-pool.Events():
			if !ok {
				return
			}

			msg := tui.OrchestratorEventMsg{
				Type:       string(event.Type),
				TaskID:     event.TaskID,
				TaskTitle:  event.TaskTitle,
				AgentID:    event.AgentID,
				Message:    event.Message,
				Timestamp:  event.Timestamp,
				TokensUsed: event.TokensUsed,
				Cost:       event.Cost,
				Duration:   event.Duration,
			}
			if event.Error != nil {
				msg.Error = event.Error.Error()
			}

			program.Send(msg)

			// Handle session done
			if event.Type == orchestrator.EventSessionDone {
				success := event.Error == nil
				program.Send(tui.SessionDoneMsg{
					Success: success,
					Message: event.Message,
				})
			}
		}
	}
}
