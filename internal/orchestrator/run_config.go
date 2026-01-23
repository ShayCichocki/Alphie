// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/orchestrator/policy"
)

// OrchestratorRunConfig contains runtime configuration that is immutable after construction.
// This consolidates configuration values that define how an orchestration session operates.
type OrchestratorRunConfig struct {
	// SessionID is the unique identifier for this orchestration session.
	SessionID string

	// RepoPath is the path to the git repository being worked on.
	RepoPath string

	// Tier is the agent tier for task execution (determines question allowance, etc.).
	Tier interface{}

	// MaxAgents is the maximum number of concurrent agents allowed.
	MaxAgents int

	// Greenfield indicates if this is a new project (changes branch handling).
	Greenfield bool

	// OriginalTaskID is the task ID from the TUI's task_entered event.
	// Used to link epic_created events back to the original task for deduplication.
	OriginalTaskID string

	// Baseline is the session baseline for regression detection.
	// Captured at session start and passed to all agent executions.
	Baseline *agent.Baseline

	// Policy contains configurable policy parameters.
	Policy *policy.Config
}

// NewRunConfig creates a new OrchestratorRunConfig with the given values.
func NewRunConfig(
	sessionID string,
	repoPath string,
	tier interface{},
	maxAgents int,
	greenfield bool,
	originalTaskID string,
	policyConfig *policy.Config,
) *OrchestratorRunConfig {
	if policyConfig == nil {
		policyConfig = policy.Default()
	}
	return &OrchestratorRunConfig{
		SessionID:      sessionID,
		RepoPath:       repoPath,
		Tier:           tier,
		MaxAgents:      maxAgents,
		Greenfield:     greenfield,
		OriginalTaskID: originalTaskID,
		Policy:         policyConfig,
	}
}
