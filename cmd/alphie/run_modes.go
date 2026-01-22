package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/orchestrator"
)

// runQuickMode executes a task directly without orchestration.
// Uses QuickExecutor for fast, single-agent execution on the current branch.
func runQuickMode(ctx context.Context, repoPath, task string, verbose bool) error {
	if verbose {
		fmt.Printf("[DEBUG] Quick mode task: %s\n", task)
	}

	fmt.Printf("Quick mode: %s\n", task)

	// Create runner factory
	runnerFactory, err := createRunnerFactory(runUseCLI)
	if err != nil {
		return fmt.Errorf("create runner factory: %w", err)
	}

	// Create and run quick executor
	executor := orchestrator.NewQuickExecutor(repoPath, runnerFactory)
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

// runPassthroughMode executes a task by directly invoking Claude without any orchestration.
// This is useful for debugging, cost control, or when you want the simplest possible execution.
// No worktrees, no decomposition, no ralph-loop, no quality gates - just Claude.
func runPassthroughMode(ctx context.Context, repoPath, task string, verbose bool) error {
	if verbose {
		fmt.Printf("[DEBUG] Passthrough mode task: %s\n", task)
	}

	fmt.Printf("Passthrough mode: %s\n", task)
	fmt.Println("(Bypassing orchestration - running Claude directly)")
	fmt.Println()

	startTime := time.Now()

	// Create Claude runner via API
	runnerFactory, err := createRunnerFactory(runUseCLI)
	if err != nil {
		return fmt.Errorf("create runner factory: %w", err)
	}
	claude := runnerFactory.NewRunner()

	// Build a simple prompt
	prompt := fmt.Sprintf(`You are working on a task in the current directory.

Task: %s

Please complete this task. When finished, provide a summary of what was done.`, task)

	// Start Claude with sonnet model (default for passthrough)
	opts := &agent.StartOptions{Model: "sonnet"}
	if err := claude.StartWithOptions(prompt, repoPath, opts); err != nil {
		return fmt.Errorf("start claude process: %w", err)
	}

	// Collect output
	var output strings.Builder
	var tokenTracker = agent.NewTokenTracker("sonnet")

	for event := range claude.Output() {
		switch event.Type {
		case agent.StreamEventAssistant:
			if event.Message != "" {
				output.WriteString(event.Message)
				output.WriteString("\n")
				// Print live output
				fmt.Print(event.Message)
			}
		case agent.StreamEventResult:
			if event.Message != "" {
				output.WriteString("\n--- Result ---\n")
				output.WriteString(event.Message)
				output.WriteString("\n")
				fmt.Printf("\n--- Result ---\n%s\n", event.Message)
			}
		case agent.StreamEventError:
			if event.Error != "" {
				fmt.Printf("\n--- Error ---\n%s\n", event.Error)
			}
		}

		// Try to extract token usage from raw JSON
		if event.Raw != nil {
			var data map[string]interface{}
			if err := json.Unmarshal(event.Raw, &data); err == nil {
				if usageData, ok := data["usage"].(map[string]interface{}); ok {
					var usage agent.MessageDeltaUsage
					if input, ok := usageData["input_tokens"].(float64); ok {
						usage.InputTokens = int64(input)
					}
					if outputTokens, ok := usageData["output_tokens"].(float64); ok {
						usage.OutputTokens = int64(outputTokens)
					}
					if usage.InputTokens > 0 || usage.OutputTokens > 0 {
						tokenTracker.Update(usage)
					}
				}
			}
		}
	}

	// Wait for process to complete
	procErr := claude.Wait()

	duration := time.Since(startTime)
	usage := tokenTracker.GetUsage()
	cost := tokenTracker.GetCost()

	fmt.Println()
	if procErr != nil {
		fmt.Printf("Passthrough mode failed: %v\n", procErr)
		return procErr
	}

	fmt.Printf("Done! (%s, ~%d tokens, $%.4f)\n",
		duration.Round(100*time.Millisecond),
		usage.TotalTokens,
		cost)

	return nil
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
