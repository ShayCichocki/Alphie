// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/git"
	"github.com/ShayCichocki/alphie/internal/merge"
	"github.com/ShayCichocki/alphie/internal/protect"
)

// MergeStrategyConfig contains dependencies for creating merge components.
type MergeStrategyConfig struct {
	// RepoPath is the path to the git repository.
	RepoPath string
	// SessionBranch is the session branch name (for normal mode).
	SessionBranch string
	// GitRunner provides git operations.
	GitRunner git.Runner
	// MergerClaude is the Claude runner for semantic merges.
	MergerClaude agent.ClaudeRunner
	// SecondReviewerClaude is the Claude runner for second reviews.
	SecondReviewerClaude agent.ClaudeRunner
	// Protected is the protected area checker for second review triggers.
	Protected *protect.Detector
	// Greenfield indicates if this is a new project (merge directly to main).
	Greenfield bool
}

// MergeStrategy configures how merge operations work for a session.
type MergeStrategy struct {
	cfg MergeStrategyConfig
}

// NewMergeStrategy creates a new MergeStrategy.
func NewMergeStrategy(cfg MergeStrategyConfig) *MergeStrategy {
	return &MergeStrategy{cfg: cfg}
}

// TargetBranch returns the branch that agents merge into.
func (s *MergeStrategy) TargetBranch() string {
	if s.cfg.Greenfield {
		return "main"
	}
	return s.cfg.SessionBranch
}

// CreateMerger creates a merge.Handler for git operations.
func (s *MergeStrategy) CreateMerger() *merge.Handler {
	return merge.NewHandlerWithRunner(s.TargetBranch(), s.cfg.RepoPath, s.cfg.GitRunner)
}

// CreateSemanticMerger creates a SemanticMerger for AI-assisted conflict resolution.
func (s *MergeStrategy) CreateSemanticMerger() *SemanticMerger {
	return NewSemanticMerger(s.cfg.MergerClaude, s.cfg.RepoPath)
}

// CreateSecondReviewer creates a SecondReviewer if configured, nil otherwise.
// Returns nil for greenfield mode (no second review needed).
func (s *MergeStrategy) CreateSecondReviewer() *SecondReviewer {
	if s.cfg.Greenfield || s.cfg.SecondReviewerClaude == nil {
		return nil
	}
	return NewSecondReviewer(s.cfg.Protected, s.cfg.SecondReviewerClaude)
}

// IsGreenfield returns true if this is a greenfield (new project) strategy.
func (s *MergeStrategy) IsGreenfield() bool {
	return s.cfg.Greenfield
}
