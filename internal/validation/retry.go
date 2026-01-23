// Package validation provides retry logic with validation feedback.
package validation

import (
	"context"
	"fmt"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (default: 3).
	MaxAttempts int
	// InjectFailureContext indicates whether to inject failure details into retry.
	InjectFailureContext bool
}

// DefaultRetryConfig returns sensible defaults for retry configuration.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:          3,
		InjectFailureContext: true,
	}
}

// RetryHandler manages task retry with validation feedback.
type RetryHandler struct {
	config RetryConfig
}

// NewRetryHandler creates a new retry handler.
func NewRetryHandler(config RetryConfig) *RetryHandler {
	return &RetryHandler{
		config: config,
	}
}

// RetryContext contains information for a retry attempt.
type RetryContext struct {
	// Attempt is the current attempt number (1-indexed).
	Attempt int
	// PreviousFailures contains failure summaries from previous attempts.
	PreviousFailures []string
	// ValidationFeedback is the detailed feedback from the last validation.
	ValidationFeedback string
	// ShouldEscalate indicates if max retries reached and escalation needed.
	ShouldEscalate bool
}

// ShouldRetry determines if another retry attempt should be made.
func (h *RetryHandler) ShouldRetry(attempt int) bool {
	return attempt < h.config.MaxAttempts
}

// BuildRetryContext creates retry context for the next attempt.
func (h *RetryHandler) BuildRetryContext(
	attempt int,
	previousFailures []string,
	lastValidation *ValidationResult,
) *RetryContext {
	ctx := &RetryContext{
		Attempt:          attempt + 1,
		PreviousFailures: previousFailures,
		ShouldEscalate:   !h.ShouldRetry(attempt),
	}

	if h.config.InjectFailureContext && lastValidation != nil {
		ctx.ValidationFeedback = h.buildValidationFeedback(lastValidation)
	}

	return ctx
}

// buildValidationFeedback creates helpful feedback for the next iteration.
func (h *RetryHandler) buildValidationFeedback(result *ValidationResult) string {
	if result == nil {
		return ""
	}

	feedback := fmt.Sprintf("Previous attempt failed: %s\n\n", result.FailureReason)
	feedback += "Validation Results:\n"
	feedback += result.Summary
	feedback += "\n\nPlease address these issues in your next attempt.\n"

	// Add specific guidance based on which layer failed
	if result.Layers.Layer1 != nil && !result.Layers.Layer1.Passed {
		feedback += "\n💡 Focus on: Verification contracts failed. Ensure all test commands pass.\n"
	} else if result.Layers.Layer2 != nil && !result.Layers.Layer2.Passed {
		feedback += "\n💡 Focus on: Build or tests failed. Check for compilation errors and test failures.\n"
	} else if result.Layers.Layer3 != nil && !result.Layers.Layer3.Passed {
		feedback += "\n💡 Focus on: Implementation doesn't match intent. Review the task requirements carefully.\n"
	} else if result.Layers.Layer4 != nil && !result.Layers.Layer4.Passed {
		feedback += "\n💡 Focus on: Code quality issues detected. Address completeness, correctness, and quality concerns.\n"
	}

	return feedback
}

// ExecuteWithRetry executes a task with retry logic and validation.
// This is a generic helper that can be used in the orchestrator or agent executor.
type ExecuteWithRetry struct {
	validator     *Validator
	retryHandler  *RetryHandler
	maxAttempts   int
	attemptDelay  time.Duration
}

// NewExecuteWithRetry creates a new execute-with-retry helper.
func NewExecuteWithRetry(validator *Validator, retryHandler *RetryHandler, maxAttempts int) *ExecuteWithRetry {
	return &ExecuteWithRetry{
		validator:    validator,
		retryHandler: retryHandler,
		maxAttempts:  maxAttempts,
		attemptDelay: 1 * time.Second, // Small delay between retries
	}
}

// TaskExecutor is the interface for executing a task.
// The agent executor should implement this interface.
type TaskExecutor interface {
	Execute(ctx context.Context, input TaskExecutionInput) (*TaskExecutionResult, error)
}

// TaskExecutionInput contains input for task execution.
type TaskExecutionInput struct {
	// Task is the task to execute (implementation-specific).
	Task interface{}
	// Options contains execution options (implementation-specific).
	Options interface{}
	// RetryContext contains retry-specific context if this is a retry.
	RetryContext *RetryContext
}

// TaskExecutionResult contains the result of task execution.
type TaskExecutionResult struct {
	// Success indicates if execution succeeded.
	Success bool
	// Output contains the execution output.
	Output string
	// Error contains error message if any.
	Error string
	// Implementation contains the code changes made.
	Implementation string
	// ModifiedFiles lists files that were changed.
	ModifiedFiles []string
}

// Execute runs a task with retry and validation logic.
// This is a template showing how to integrate validation with retry.
func (e *ExecuteWithRetry) Execute(
	ctx context.Context,
	executor TaskExecutor,
	execInput TaskExecutionInput,
	validationInput ValidationInput,
) (*TaskExecutionResult, *ValidationResult, error) {

	var lastResult *TaskExecutionResult
	var lastValidation *ValidationResult
	previousFailures := []string{}

	for attempt := 1; attempt <= e.maxAttempts; attempt++ {
		// Build retry context for this attempt
		retryCtx := e.retryHandler.BuildRetryContext(attempt-1, previousFailures, lastValidation)
		execInput.RetryContext = retryCtx

		// Execute the task
		result, err := executor.Execute(ctx, execInput)
		if err != nil {
			return nil, nil, fmt.Errorf("execution error on attempt %d: %w", attempt, err)
		}

		lastResult = result

		// If execution itself failed, record and retry
		if !result.Success {
			previousFailures = append(previousFailures, result.Error)
			if e.retryHandler.ShouldRetry(attempt) {
				time.Sleep(e.attemptDelay)
				continue
			}
			// Max retries reached
			break
		}

		// Execution succeeded - now validate
		validationInput.Implementation = result.Implementation
		validationInput.ModifiedFiles = result.ModifiedFiles

		validation, err := e.validator.Validate(ctx, validationInput)
		if err != nil {
			return result, nil, fmt.Errorf("validation error on attempt %d: %w", attempt, err)
		}

		lastValidation = validation

		// Check if validation passed
		if validation.AllPassed {
			// Success! All layers passed
			return result, validation, nil
		}

		// Validation failed - record and retry if possible
		previousFailures = append(previousFailures, validation.FailureReason)
		if !e.retryHandler.ShouldRetry(attempt) {
			// Max retries reached
			break
		}

		// Wait before next retry
		time.Sleep(e.attemptDelay)
	}

	// Max retries reached - return last result and validation for escalation
	return lastResult, lastValidation, nil
}
