package agent

import (
	"context"

	"github.com/ShayCichocki/alphie/pkg/models"
)

// TaskExecutor defines the interface for task execution.
// This abstraction allows for testing and alternative implementations.
type TaskExecutor interface {
	// Execute runs a task with a single agent using default options.
	Execute(ctx context.Context, task *models.Task, tierIgnored interface{}) (*ExecutionResult, error)
	// ExecuteWithOptions runs a task with a single agent using the provided options.
	ExecuteWithOptions(ctx context.Context, task *models.Task, tierIgnored interface{}, opts *ExecuteOptions) (*ExecutionResult, error)
}

// Compile-time verification that Executor implements TaskExecutor.
var _ TaskExecutor = (*Executor)(nil)

// ClaudeRunner defines the interface for Claude execution backends.
// This interface is implemented by both:
// - ClaudeProcess (subprocess-based, uses claude CLI)
// - ClaudeAPIAdapter (direct API calls via Anthropic SDK)
type ClaudeRunner interface {
	// Start launches Claude with the given prompt and working directory.
	Start(prompt, workDir string) error

	// StartWithOptions launches Claude with additional options.
	StartWithOptions(prompt, workDir string, opts *StartOptions) error

	// Output returns a channel that receives stream events from Claude.
	// The channel is closed when execution completes.
	Output() <-chan StreamEvent

	// Wait waits for execution to complete and returns any error.
	Wait() error

	// Kill terminates execution immediately.
	Kill() error

	// Stderr returns any captured stderr output.
	// For API-based implementations, returns empty string.
	Stderr() string

	// PID returns the process ID.
	// For API-based implementations, returns 0.
	PID() int
}

// ClaudeRunnerFactory creates ClaudeRunner instances.
// This allows the executor to switch between subprocess and API implementations.
type ClaudeRunnerFactory interface {
	// NewRunner creates a new ClaudeRunner instance with the given context.
	NewRunner() ClaudeRunner
}

// Verify ClaudeProcess implements ClaudeRunner at compile time.
var _ ClaudeRunner = (*ClaudeProcess)(nil)

// TokenAggregator tracks tokens across multiple agents.
// This interface allows mocking token tracking in tests.
type TokenAggregator interface {
	// Add adds a tracker for an agent ID.
	Add(agentID string, tracker *TokenTracker)
	// Remove removes a tracker for an agent ID.
	Remove(agentID string)
}

// Verify AggregateTracker implements TokenAggregator at compile time.
var _ TokenAggregator = (*AggregateTracker)(nil)

// AgentLifecycle manages agent lifecycle operations.
// This interface allows mocking agent lifecycle management in tests.
type AgentLifecycle interface {
	// CreateWithID creates a new agent in pending state with the given ID.
	CreateWithID(agentID, taskID, worktreePath string) (*models.Agent, error)
	// Start transitions an agent from pending to running.
	Start(agentID string, pid int) error
	// Complete transitions a running agent to done.
	Complete(agentID string) error
	// Fail transitions an agent to failed state.
	Fail(agentID string, reason string) error
	// UpdateUsage updates the token usage and cost for an agent.
	UpdateUsage(agentID string, tokensUsed int64, cost float64) error
}

// Verify Manager implements AgentLifecycle at compile time.
var _ AgentLifecycle = (*Manager)(nil)
