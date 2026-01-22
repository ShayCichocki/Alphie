package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/internal/tui"
)

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
			Type:           string(event.Type),
			TaskID:         event.TaskID,
			TaskTitle:      event.TaskTitle,
			ParentID:       event.ParentID,
			AgentID:        event.AgentID,
			Message:        event.Message,
			Error:          errStr,
			Timestamp:      event.Timestamp,
			TokensUsed:     event.TokensUsed,
			Cost:           event.Cost,
			Duration:       event.Duration,
			LogFile:        event.LogFile,
			CurrentAction:  event.CurrentAction,
			OriginalTaskID: event.OriginalTaskID,
		}
		program.Send(msg)
	}
}
