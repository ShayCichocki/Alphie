package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// EscalationAction represents the user's choice when a task needs escalation.
type EscalationAction string

const (
	// EscalationRetry retries the task with same configuration.
	EscalationRetry EscalationAction = "retry"
	// EscalationSkip skips the task and its dependents, continues with remaining work.
	EscalationSkip EscalationAction = "skip"
	// EscalationAbort stops the entire orchestrator execution.
	EscalationAbort EscalationAction = "abort"
	// EscalationManualFix indicates user will manually fix in worktree, then resume.
	EscalationManualFix EscalationAction = "manual_fix"
)

// EscalationRequest contains information about a task that needs escalation.
type EscalationRequest struct {
	// Task is the task that needs escalation.
	Task *models.Task
	// Result is the execution result showing why it failed.
	Result *agent.ExecutionResult
	// Attempts is the number of attempts already made.
	Attempts int
	// FailureReason is a human-readable summary of why the task failed.
	FailureReason string
	// ValidationSummary contains detailed validation feedback if available.
	ValidationSummary string
	// WorktreePath is the path to the worktree for manual fixes.
	WorktreePath string
}

// EscalationResponse contains the user's choice and any additional information.
type EscalationResponse struct {
	// Action is the user's chosen action.
	Action EscalationAction
	// Message contains any additional message from the user.
	Message string
	// Timestamp is when the user responded.
	Timestamp time.Time
}

// EscalationHandler manages user escalation when tasks fail after max retries.
type EscalationHandler struct {
	orchestrator *Orchestrator

	// Escalation state
	mu                sync.RWMutex
	hasEscalation     bool
	currentRequest    *EscalationRequest
	responseCh        chan *EscalationResponse
	escalationTimeout time.Duration
}

// NewEscalationHandler creates a new escalation handler.
func NewEscalationHandler(orchestrator *Orchestrator) *EscalationHandler {
	return &EscalationHandler{
		orchestrator:      orchestrator,
		hasEscalation:     false,
		responseCh:        make(chan *EscalationResponse, 1),
		escalationTimeout: 30 * time.Minute, // 30 minute timeout for user response
	}
}

// NeedsEscalation determines if a task completion requires user escalation.
// This checks if the task failed after max retries or was aborted.
func (h *EscalationHandler) NeedsEscalation(task *models.Task, result *agent.ExecutionResult) bool {
	if result.Success && result.IsVerified() {
		// Task succeeded and passed verification - no escalation needed
		return false
	}

	// Task failed or didn't pass verification - check if max iterations reached
	if result.LoopIterations > 0 && result.LoopExitReason != "" {
		// If the loop exited due to max iterations, escalation needed
		if result.LoopExitReason == "max_iterations_reached" {
			return true
		}
	}

	// If execution count is >= max retries (3), escalation needed
	if task.ExecutionCount >= 3 {
		return true
	}

	return false
}

// RequestEscalation pauses the orchestrator and requests user input for a failed task.
// This blocks until the user responds or the timeout is reached.
func (h *EscalationHandler) RequestEscalation(ctx context.Context, req *EscalationRequest) (*EscalationResponse, error) {
	h.mu.Lock()
	if h.hasEscalation {
		h.mu.Unlock()
		return nil, fmt.Errorf("escalation already in progress")
	}
	h.hasEscalation = true
	h.currentRequest = req
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		h.hasEscalation = false
		h.currentRequest = nil
		h.mu.Unlock()
	}()

	// Pause the orchestrator - block new task scheduling
	h.orchestrator.pauseCtrl.Pause()
	defer h.orchestrator.pauseCtrl.Resume()

	// Log escalation
	if h.orchestrator.logger != nil {
		h.orchestrator.logger.Log("ESCALATION", "Task %s needs escalation after %d attempts: %s",
			req.Task.ID, req.Attempts, req.FailureReason)
	}

	// Emit escalation event for TUI
	h.emitEscalationEvent(req)

	// Wait for user response or timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case response := <-h.responseCh:
		// Log user's choice
		if h.orchestrator.logger != nil {
			h.orchestrator.logger.Log("ESCALATION", "User chose action: %s for task %s", response.Action, req.Task.ID)
		}
		return response, nil
	case <-time.After(h.escalationTimeout):
		// Timeout - default to abort for safety
		if h.orchestrator.logger != nil {
			h.orchestrator.logger.Log("ESCALATION", "Timeout after %v - defaulting to abort", h.escalationTimeout)
		}
		return &EscalationResponse{
			Action:    EscalationAbort,
			Message:   "Escalation timed out after 30 minutes",
			Timestamp: time.Now(),
		}, nil
	}
}

// RespondToEscalation sends the user's response to the waiting escalation request.
// This is called by the TUI or CLI when the user makes a choice.
func (h *EscalationHandler) RespondToEscalation(response *EscalationResponse) error {
	h.mu.RLock()
	if !h.hasEscalation {
		h.mu.RUnlock()
		return fmt.Errorf("no escalation in progress")
	}
	h.mu.RUnlock()

	// Send response
	select {
	case h.responseCh <- response:
		return nil
	default:
		return fmt.Errorf("failed to send escalation response")
	}
}

// GetCurrentRequest returns the current escalation request if one is active.
func (h *EscalationHandler) GetCurrentRequest() *EscalationRequest {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.currentRequest
}

// HasEscalation returns true if there is an active escalation.
func (h *EscalationHandler) HasEscalation() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.hasEscalation
}

// emitEscalationEvent sends an escalation event to the TUI.
func (h *EscalationHandler) emitEscalationEvent(req *EscalationRequest) {
	if h.orchestrator == nil {
		return
	}

	h.orchestrator.emitEvent(OrchestratorEvent{
		Type:      EventTaskEscalation,
		TaskID:    req.Task.ID,
		TaskTitle: req.Task.Title,
		ParentID:  req.Task.ParentID,
		AgentID:   req.Result.AgentID,
		Message:   fmt.Sprintf("Task escalation required: %s", req.FailureReason),
		Error:     fmt.Errorf("%s", req.FailureReason),
		Timestamp: time.Now(),
		LogFile:   req.Result.LogFile,
		Metadata: map[string]interface{}{
			"attempts":           req.Attempts,
			"failure_reason":     req.FailureReason,
			"validation_summary": req.ValidationSummary,
			"worktree_path":      req.WorktreePath,
			"loop_iterations":    req.Result.LoopIterations,
			"loop_exit_reason":   req.Result.LoopExitReason,
		},
	})
}

// HandleEscalationAction processes the user's chosen action and updates task state.
func (h *EscalationHandler) HandleEscalationAction(
	ctx context.Context,
	task *models.Task,
	result *agent.ExecutionResult,
	action EscalationAction,
) error {
	switch action {
	case EscalationRetry:
		return h.handleRetry(task, result)
	case EscalationSkip:
		return h.handleSkip(task)
	case EscalationAbort:
		return h.handleAbort(task)
	case EscalationManualFix:
		return h.handleManualFix(ctx, task, result)
	default:
		return fmt.Errorf("unknown escalation action: %s", action)
	}
}

// handleRetry resets the task for retry.
func (h *EscalationHandler) handleRetry(task *models.Task, result *agent.ExecutionResult) error {
	if h.orchestrator.logger != nil {
		h.orchestrator.logger.Log("ESCALATION", "Retrying task %s", task.ID)
	}

	// Reset task state for retry
	task.Status = models.TaskStatusPending
	task.Error = ""
	task.ExecutionCount = 0 // Reset retry count for fresh start

	// Emit retry event
	h.orchestrator.emitEvent(OrchestratorEvent{
		Type:      EventTaskRetry,
		TaskID:    task.ID,
		TaskTitle: task.Title,
		ParentID:  task.ParentID,
		Message:   "Task will be retried after user escalation",
		Timestamp: time.Now(),
	})

	return nil
}

// handleSkip marks the task as skipped and blocks its dependents.
func (h *EscalationHandler) handleSkip(task *models.Task) error {
	if h.orchestrator.logger != nil {
		h.orchestrator.logger.Log("ESCALATION", "Skipping task %s and its dependents", task.ID)
	}

	// Mark task as blocked/skipped
	task.Status = models.TaskStatusBlocked
	task.BlockedReason = "escalation_skipped"
	task.Error = "Task skipped by user after escalation"

	// Find and block all dependent tasks
	dependents := h.orchestrator.graph.GetDependents(task.ID)
	for _, depID := range dependents {
		depTask := h.orchestrator.graph.GetTask(depID)
		if depTask != nil && depTask.Status == models.TaskStatusPending {
			depTask.Status = models.TaskStatusBlocked
			depTask.BlockedReason = fmt.Sprintf("dependency_skipped:%s", task.ID)

			if h.orchestrator.logger != nil {
				h.orchestrator.logger.Log("ESCALATION", "Blocking dependent task %s", depID)
			}
		}
	}

	// Emit skip event
	h.orchestrator.emitEvent(OrchestratorEvent{
		Type:      EventTaskSkipped,
		TaskID:    task.ID,
		TaskTitle: task.Title,
		ParentID:  task.ParentID,
		Message:   fmt.Sprintf("Task skipped by user, %d dependents blocked", len(dependents)),
		Timestamp: time.Now(),
	})

	return nil
}

// handleAbort stops the entire orchestrator execution.
func (h *EscalationHandler) handleAbort(task *models.Task) error {
	if h.orchestrator.logger != nil {
		h.orchestrator.logger.Log("ESCALATION", "User chose to abort execution for task %s", task.ID)
	}

	// Mark task as failed
	task.Status = models.TaskStatusFailed
	task.Error = "Execution aborted by user during escalation"

	// Emit abort event
	h.orchestrator.emitEvent(OrchestratorEvent{
		Type:      EventAbort,
		TaskID:    task.ID,
		TaskTitle: task.Title,
		ParentID:  task.ParentID,
		Message:   "User aborted execution during task escalation",
		Timestamp: time.Now(),
	})

	// Return error to stop orchestrator
	return fmt.Errorf("execution aborted by user during task %s escalation", task.ID)
}

// handleManualFix waits for user to manually fix the code, then validates and merges.
func (h *EscalationHandler) handleManualFix(ctx context.Context, task *models.Task, result *agent.ExecutionResult) error {
	if h.orchestrator.logger != nil {
		h.orchestrator.logger.Log("ESCALATION", "Waiting for manual fix in worktree: %s", result.WorktreePath)
	}

	// Emit event telling user where to make changes
	h.orchestrator.emitEvent(OrchestratorEvent{
		Type:      EventManualFixRequired,
		TaskID:    task.ID,
		TaskTitle: task.Title,
		ParentID:  task.ParentID,
		Message:   fmt.Sprintf("Make changes in worktree: %s, then confirm", result.WorktreePath),
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"worktree_path": result.WorktreePath,
		},
	})

	// Wait for user confirmation (via another escalation response)
	// The TUI should provide a "Done" button that triggers this
	// For now, we mark the task as ready for validation

	// TODO: This needs integration with validation package to re-run validation
	// after manual changes. For now, we'll mark it as requiring validation.

	task.Status = models.TaskStatusPending
	task.Error = ""

	if h.orchestrator.logger != nil {
		h.orchestrator.logger.Log("ESCALATION", "Manual fix workflow initiated for task %s", task.ID)
	}

	return nil
}
