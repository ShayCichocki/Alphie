// Package agent provides the AI agent implementation for Alphie.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/shayc/alphie/internal/learning"
	"github.com/shayc/alphie/pkg/models"
)

// ExecutionResult contains the outcome of a single task execution.
type ExecutionResult struct {
	// Success indicates whether the task completed successfully.
	Success bool
	// Output contains the captured output from the agent.
	Output string
	// Error contains the error message if execution failed.
	Error string
	// TokensUsed is the total number of tokens consumed.
	TokensUsed int64
	// Cost is the total cost in dollars.
	Cost float64
	// Duration is how long the execution took.
	Duration time.Duration
	// AgentID is the ID of the agent that executed the task.
	AgentID string
	// WorktreePath is the path to the worktree used for execution.
	WorktreePath string
	// Model is the Claude model that was dynamically selected for this task.
	Model string
	// SuggestedLearnings contains potential learnings extracted from failures.
	// These need user confirmation before being stored.
	SuggestedLearnings []*learning.SuggestedLearning
	// LogFile is the path to the detailed execution log.
	LogFile string
}

// Executor wires together worktree creation, subprocess management,
// stream parsing, token tracking, and cleanup for single-agent task execution.
type Executor struct {
	worktreeMgr     *WorktreeManager
	tokenTracker    *AggregateTracker
	agentMgr        *Manager
	model           string
	failureAnalyzer *learning.FailureAnalyzer
	taskTimeout     time.Duration
}

// ExecutorConfig contains configuration options for the Executor.
type ExecutorConfig struct {
	// WorktreeBaseDir is where worktrees are created (defaults to ~/.cache/alphie/worktrees).
	WorktreeBaseDir string
	// RepoPath is the path to the main git repository.
	RepoPath string
	// Model is the Claude model to use for cost calculation.
	Model string
	// TaskTimeout is the maximum duration for a single task execution.
	// Default is 10 minutes if not specified.
	TaskTimeout time.Duration
}

// NewExecutor creates a new Executor with the given configuration.
func NewExecutor(cfg ExecutorConfig) (*Executor, error) {
	worktreeMgr, err := NewWorktreeManager(cfg.WorktreeBaseDir, cfg.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("create worktree manager: %w", err)
	}

	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}

	taskTimeout := cfg.TaskTimeout
	if taskTimeout == 0 {
		taskTimeout = 10 * time.Minute // Default 10 minute timeout
	}

	return &Executor{
		worktreeMgr:     worktreeMgr,
		tokenTracker:    NewAggregateTracker(),
		agentMgr:        NewManager(),
		model:           model,
		failureAnalyzer: learning.NewFailureAnalyzer(),
		taskTimeout:     taskTimeout,
	}, nil
}

// ProgressUpdate contains current execution progress information.
type ProgressUpdate struct {
	// AgentID is the ID of the agent executing the task.
	AgentID string
	// TokensUsed is the current total tokens consumed.
	TokensUsed int64
	// Cost is the current total cost in dollars.
	Cost float64
	// Duration is time elapsed since execution started.
	Duration time.Duration
	// CurrentAction describes what the agent is doing right now (e.g., "Reading auth.go").
	CurrentAction string
}

// ProgressCallback is called periodically during task execution with progress updates.
type ProgressCallback func(update ProgressUpdate)

// ExecuteOptions contains optional parameters for task execution.
type ExecuteOptions struct {
	// Learnings are relevant learnings retrieved for this task.
	// They are injected into the agent's prompt to provide context.
	Learnings []*learning.Learning
	// OnProgress is called periodically with execution progress updates.
	// Can be nil if progress updates are not needed.
	OnProgress ProgressCallback
}

// Execute runs a single task with a single agent.
// It creates an isolated worktree, starts the Claude Code process,
// streams and parses output, tracks tokens, waits for completion,
// cleans up the worktree, and returns the result.
func (e *Executor) Execute(ctx context.Context, task *models.Task, tier models.Tier) (*ExecutionResult, error) {
	return e.ExecuteWithOptions(ctx, task, tier, nil)
}

// ExecuteWithOptions runs a single task with a single agent, accepting optional parameters.
// It creates an isolated worktree, starts the Claude Code process,
// streams and parses output, tracks tokens, waits for completion,
// cleans up the worktree, and returns the result.
func (e *Executor) ExecuteWithOptions(ctx context.Context, task *models.Task, tier models.Tier, opts *ExecuteOptions) (*ExecutionResult, error) {
	startTime := time.Now()
	result := &ExecutionResult{}

	// Apply task timeout
	ctx, cancel := context.WithTimeout(ctx, e.taskTimeout)
	defer cancel()

	// Create log file for this task
	logDir := filepath.Join(e.worktreeMgr.RepoPath(), ".alphie", "logs")
	_ = os.MkdirAll(logDir, 0755)
	logFileName := fmt.Sprintf("task-%s-%s.log", task.ID[:8], startTime.Format("150405"))
	logFile := filepath.Join(logDir, logFileName)
	result.LogFile = logFile

	// 1. Create worktree
	worktree, err := e.worktreeMgr.Create(task.ID)
	if err != nil {
		return nil, fmt.Errorf("create worktree: %w", err)
	}
	result.WorktreePath = worktree.Path

	// Ensure cleanup happens regardless of outcome
	defer func() {
		// Force remove the worktree on cleanup
		_ = e.worktreeMgr.Remove(worktree.Path, true)
	}()

	// 2. Create agent and token tracker
	agent, err := e.agentMgr.Create(task.ID, worktree.Path)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	result.AgentID = agent.ID

	// Select model dynamically based on task keywords and tier
	selectedModel := SelectModel(task, tier)
	result.Model = selectedModel
	tracker := NewTokenTracker(selectedModel)
	e.tokenTracker.Add(agent.ID, tracker)
	defer e.tokenTracker.Remove(agent.ID)

	// 3. Build the prompt from task
	prompt := e.buildPrompt(task, tier, opts)

	// 4. Start Claude Code process
	proc := NewClaudeProcess(ctx)
	if err := proc.Start(prompt, worktree.Path); err != nil {
		_ = e.agentMgr.Fail(agent.ID, fmt.Sprintf("failed to start process: %v", err))
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	// Transition agent to running
	if err := e.agentMgr.Start(agent.ID, proc.PID()); err != nil {
		_ = proc.Kill()
		return nil, fmt.Errorf("start agent: %w", err)
	}

	// 5. Stream and parse output, track tokens
	var outputBuilder strings.Builder
	var currentAction string
	lastProgressUpdate := time.Now()
	progressInterval := 2 * time.Second // Send progress updates every 2 seconds
	for event := range proc.Output() {
		e.processStreamEvent(event, tracker, &outputBuilder)

		// Track current tool action
		if event.ToolAction != "" {
			currentAction = event.ToolAction
		}

		// Send periodic progress updates
		if opts != nil && opts.OnProgress != nil && time.Since(lastProgressUpdate) >= progressInterval {
			usage := tracker.GetUsage()
			opts.OnProgress(ProgressUpdate{
				AgentID:       agent.ID,
				TokensUsed:    usage.TotalTokens,
				Cost:          tracker.GetCost(),
				Duration:      time.Since(startTime),
				CurrentAction: currentAction,
			})
			lastProgressUpdate = time.Now()
		}
	}

	// 6. Wait for completion
	procErr := proc.Wait()

	// Capture final results
	result.Output = outputBuilder.String()
	result.Duration = time.Since(startTime)

	usage := tracker.GetUsage()
	result.TokensUsed = usage.TotalTokens
	result.Cost = tracker.GetCost()

	// Update agent with usage
	_ = e.agentMgr.UpdateUsage(agent.ID, usage.TotalTokens, result.Cost)

	// 7. Auto-commit any changes made by the agent
	// This ensures changes are preserved when the worktree is removed
	if procErr == nil {
		if err := e.autoCommitChanges(worktree.Path, task.Title); err != nil {
			// Log but don't fail - agent might have made no changes
			result.Output += fmt.Sprintf("\n[Auto-commit: %v]", err)
		}
	}

	// 8. Determine success/failure
	if procErr != nil || ctx.Err() != nil {
		result.Success = false
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = fmt.Sprintf("task timed out after %v", e.taskTimeout)
		} else if procErr != nil {
			result.Error = procErr.Error()
			if stderr := proc.Stderr(); stderr != "" {
				result.Error += "; stderr: " + stderr
			}
		} else {
			result.Error = ctx.Err().Error()
		}
		_ = e.agentMgr.Fail(agent.ID, result.Error)

		// Capture potential learnings from failure
		if e.failureAnalyzer != nil {
			result.SuggestedLearnings = e.failureAnalyzer.AnalyzeFailure(result.Output, result.Error)
		}
	} else {
		result.Success = true
		_ = e.agentMgr.Complete(agent.ID)
	}

	// Write detailed log file
	var logContent strings.Builder
	logContent.WriteString(fmt.Sprintf("Task: %s\n", task.Title))
	logContent.WriteString(fmt.Sprintf("Task ID: %s\n", task.ID))
	logContent.WriteString(fmt.Sprintf("Tier: %s\n", tier))
	logContent.WriteString(fmt.Sprintf("Model: %s\n", result.Model))
	logContent.WriteString(fmt.Sprintf("Started: %s\n", startTime.Format(time.RFC3339)))
	logContent.WriteString(fmt.Sprintf("Duration: %s\n", result.Duration))
	logContent.WriteString(fmt.Sprintf("Tokens: %d\n", result.TokensUsed))
	logContent.WriteString(fmt.Sprintf("Cost: $%.4f\n", result.Cost))
	logContent.WriteString(fmt.Sprintf("Success: %v\n", result.Success))
	if result.Error != "" {
		logContent.WriteString(fmt.Sprintf("Error: %s\n", result.Error))
	}
	logContent.WriteString("\n--- Output ---\n")
	logContent.WriteString(result.Output)
	logContent.WriteString("\n")
	_ = os.WriteFile(logFile, []byte(logContent.String()), 0644)

	return result, nil
}

// buildPrompt constructs the prompt for the Claude Code agent.
func (e *Executor) buildPrompt(task *models.Task, tier models.Tier, opts *ExecuteOptions) string {
	var sb strings.Builder

	// Inject scope guidance at task start to prevent scope creep
	sb.WriteString(ScopeGuidancePrompt)
	sb.WriteString("\n")

	sb.WriteString("You are working on a task.\n\n")
	sb.WriteString("Task ID: ")
	sb.WriteString(task.ID)
	sb.WriteString("\n")
	sb.WriteString("Title: ")
	sb.WriteString(task.Title)
	sb.WriteString("\n")

	if task.Description != "" {
		sb.WriteString("\nDescription:\n")
		sb.WriteString(task.Description)
		sb.WriteString("\n")
	}

	sb.WriteString("\nTier: ")
	sb.WriteString(string(tier))
	sb.WriteString("\n")

	// Add tier-specific guidance
	switch tier {
	case models.TierScout:
		sb.WriteString("\nYou are operating as a Scout agent. Focus on exploration, research, and lightweight tasks.\n")
	case models.TierBuilder:
		sb.WriteString("\nYou are operating as a Builder agent. Focus on implementation and standard development tasks.\n")
	case models.TierArchitect:
		sb.WriteString("\nYou are operating as an Architect agent. Focus on complex design, architecture, and system-level decisions.\n")
	}

	// Inject relevant learnings if available
	if opts != nil && len(opts.Learnings) > 0 {
		sb.WriteString("\n## Relevant Learnings\n")
		sb.WriteString("The following learnings from previous experiences may be helpful:\n\n")
		for i, l := range opts.Learnings {
			sb.WriteString(fmt.Sprintf("### Learning %d\n", i+1))
			sb.WriteString(fmt.Sprintf("- **When**: %s\n", l.Condition))
			sb.WriteString(fmt.Sprintf("- **Do**: %s\n", l.Action))
			sb.WriteString(fmt.Sprintf("- **Result**: %s\n", l.Outcome))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\nPlease complete this task. When finished, provide a summary of what was done.\n")

	return sb.String()
}

// processStreamEvent processes a single stream event, updating the token tracker
// and capturing output.
func (e *Executor) processStreamEvent(event StreamEvent, tracker *TokenTracker, output *strings.Builder) {
	switch event.Type {
	case StreamEventAssistant:
		// Capture assistant messages as output
		if event.Message != "" {
			output.WriteString(event.Message)
			output.WriteString("\n")
		}

	case StreamEventResult:
		// Capture result messages
		if event.Message != "" {
			output.WriteString("\n--- Result ---\n")
			output.WriteString(event.Message)
			output.WriteString("\n")
		}

	case StreamEventError:
		// Capture error messages
		if event.Error != "" {
			output.WriteString("\n--- Error ---\n")
			output.WriteString(event.Error)
			output.WriteString("\n")
		}
	}

	// Try to extract token usage from raw JSON
	if event.Raw != nil {
		e.extractTokenUsage(event.Raw, tracker)
	}
}

// extractTokenUsage attempts to extract token usage information from raw JSON.
func (e *Executor) extractTokenUsage(raw json.RawMessage, tracker *TokenTracker) {
	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return
	}

	// Look for usage field
	usageData, ok := data["usage"].(map[string]interface{})
	if !ok {
		return
	}

	var usage MessageDeltaUsage

	if input, ok := usageData["input_tokens"].(float64); ok {
		usage.InputTokens = int64(input)
	}
	if output, ok := usageData["output_tokens"].(float64); ok {
		usage.OutputTokens = int64(output)
	}

	if usage.InputTokens > 0 || usage.OutputTokens > 0 {
		tracker.Update(usage)
	}
}

// GetAgentManager returns the agent lifecycle manager.
func (e *Executor) GetAgentManager() *Manager {
	return e.agentMgr
}

// GetTokenTracker returns the aggregate token tracker.
func (e *Executor) GetTokenTracker() *AggregateTracker {
	return e.tokenTracker
}

// GetWorktreeManager returns the worktree manager.
func (e *Executor) GetWorktreeManager() *WorktreeManager {
	return e.worktreeMgr
}

// GetFailureAnalyzer returns the failure analyzer for extracting learnings.
func (e *Executor) GetFailureAnalyzer() *learning.FailureAnalyzer {
	return e.failureAnalyzer
}

// autoCommitChanges commits any uncommitted changes in the worktree.
// This ensures agent changes are preserved when the worktree is removed.
func (e *Executor) autoCommitChanges(worktreePath, taskTitle string) error {
	// Check if there are any changes to commit
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = worktreePath
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("check git status: %w", err)
	}

	// No changes to commit
	if len(statusOutput) == 0 {
		return fmt.Errorf("no changes to commit")
	}

	// Stage all changes
	addCmd := exec.Command("git", "add", "-A")
	addCmd.Dir = worktreePath
	if output, err := addCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %s: %w", string(output), err)
	}

	// Commit with task title as message
	commitMsg := fmt.Sprintf("Agent: %s", taskTitle)
	commitCmd := exec.Command("git", "commit", "-m", commitMsg)
	commitCmd.Dir = worktreePath
	if output, err := commitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %s: %w", string(output), err)
	}

	return nil
}
