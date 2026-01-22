// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/git"
	"github.com/ShayCichocki/alphie/internal/merge"
)

// MergeProcessorConfig contains configuration for the merge executor.
type MergeProcessorConfig struct {
	// MaxRetries is the maximum number of retry attempts for semantic merge.
	MaxRetries int
	// RetryBaseDelay is the base delay between retries (exponential backoff).
	RetryBaseDelay time.Duration
	// SemanticMergeTimeout is the maximum time to wait for semantic merge.
	SemanticMergeTimeout time.Duration
}

// DefaultMergeProcessorConfig returns sensible defaults.
func DefaultMergeProcessorConfig() MergeProcessorConfig {
	return MergeProcessorConfig{
		MaxRetries:           3,
		RetryBaseDelay:       2 * time.Second,
		SemanticMergeTimeout: 5 * time.Minute,
	}
}

// MergeProcessor handles merge execution with retry logic.
// It encapsulates the core merge algorithm: try git merge, then semantic merge if needed.
type MergeProcessor struct {
	merger         *merge.Handler
	semanticMerger *SemanticMerger
	factory        func() *SemanticMerger
	config         MergeProcessorConfig
	sessionBranch  string
	greenfield     bool
	humanResolver  merge.HumanMergeResolver // For interactive conflict resolution
	repoPath       string
	orchestrator   *Orchestrator // For merge conflict blocking
	git            git.Runner    // For git operations in resolver
}

// NewMergeProcessor creates a new MergeProcessor.
func NewMergeProcessor(
	merger *merge.Handler,
	semanticMerger *SemanticMerger,
	factory func() *SemanticMerger,
	config MergeProcessorConfig,
	sessionBranch string,
	greenfield bool,
	humanResolver merge.HumanMergeResolver,
	repoPath string,
) *MergeProcessor {
	return &MergeProcessor{
		merger:         merger,
		semanticMerger: semanticMerger,
		factory:        factory,
		config:         config,
		sessionBranch:  sessionBranch,
		greenfield:     greenfield,
		humanResolver:  humanResolver,
		repoPath:       repoPath,
	}
}

// SetOrchestrator sets the orchestrator reference for merge conflict blocking.
func (e *MergeProcessor) SetOrchestrator(o *Orchestrator) {
	e.orchestrator = o
}

// SetGitRunner sets the git runner for merge resolver operations.
func (e *MergeProcessor) SetGitRunner(g git.Runner) {
	e.git = g
}

// Execute performs the merge operation for a request.
// It first tries a git merge, then falls back to semantic merge if needed.
// Returns the merge outcome indicating success or failure with details.
func (e *MergeProcessor) Execute(ctx context.Context, req *MergeRequest) MergeOutcome {
	// Step 1: Try git merge (with retry for greenfield)
	mergeResult, err := e.tryGitMerge(req)
	if err != nil {
		return MergeOutcome{
			Success: false,
			Error:   fmt.Errorf("git merge failed: %w", err),
			Reason:  "git merge operation failed",
		}
	}

	// Step 2: If git merge succeeded, we're done
	if mergeResult.Success {
		_ = e.merger.DeleteBranch(req.AgentBranch)
		return MergeOutcome{
			Success: true,
			Reason:  "git merge succeeded",
		}
	}

	// Step 3: Git merge failed, check if semantic merge is possible
	if !mergeResult.NeedsSemanticMerge {
		return MergeOutcome{
			Success:       false,
			Error:         mergeResult.Error,
			Reason:        "merge failed without semantic merge option",
			ConflictFiles: mergeResult.ConflictFiles,
		}
	}

	// Step 4: Try semantic merge with retries
	outcome := e.trySemanticMergeWithRetry(ctx, req, mergeResult.ConflictFiles)
	if !outcome.Success {
		outcome.ConflictFiles = mergeResult.ConflictFiles
	}
	return outcome
}

// tryGitMerge attempts a git merge, with retry logic for greenfield mode.
func (e *MergeProcessor) tryGitMerge(req *MergeRequest) (*merge.Result, error) {
	if e.greenfield {
		return e.merger.MergeWithRetry(req.AgentBranch, 3)
	}
	return e.merger.Merge(req.AgentBranch)
}

// trySemanticMergeWithRetry attempts semantic merge with exponential backoff.
func (e *MergeProcessor) trySemanticMergeWithRetry(ctx context.Context, req *MergeRequest, conflictFiles []string) MergeOutcome {
	if e.semanticMerger == nil && e.factory == nil {
		return MergeOutcome{
			Success: false,
			Error:   fmt.Errorf("no semantic merger available"),
			Reason:  "semantic merger not configured",
		}
	}

	targetBranch := e.sessionBranch
	if e.greenfield {
		targetBranch = "main"
	}

	var lastErr error
	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			delay := e.config.RetryBaseDelay * time.Duration(1<<uint(attempt-1))
			debugLog("[merge-executor] semantic merge retry %d for task %s, waiting %v", attempt, req.TaskID, delay)

			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return MergeOutcome{
					Success: false,
					Error:   ctx.Err(),
					Reason:  "context cancelled during retry backoff",
				}
			}
		}

		// Get a semantic merger - always use factory for fresh instance on retries
		var merger *SemanticMerger
		if attempt > 0 && e.factory != nil {
			debugLog("[merge-executor] creating fresh semantic merger for retry attempt %d", attempt)
			merger = e.factory()
		} else {
			// First attempt can use existing instance
			merger = e.semanticMerger
		}

		if merger == nil {
			lastErr = fmt.Errorf("no semantic merger available")
			continue
		}

		// Create timeout context
		mergeCtx, cancel := context.WithTimeout(ctx, e.config.SemanticMergeTimeout)

		result, err := merger.Merge(mergeCtx, targetBranch, req.AgentBranch, conflictFiles)

		// CRITICAL: Always cleanup the Claude process after merge attempt
		if merger != nil && merger.claude != nil {
			killErr := merger.claude.Kill()
			if killErr != nil {
				debugLog("[merge-executor] warning: failed to kill claude process: %v", killErr)
			}
		}

		cancel()

		if err != nil {
			lastErr = err
			debugLog("[merge-executor] semantic merge attempt %d failed for task %s: %v", attempt, req.TaskID, err)
			continue
		}

		if result.Success {
			_ = e.merger.DeleteBranch(req.AgentBranch)
			return MergeOutcome{
				Success: true,
				Reason:  fmt.Sprintf("semantic merge succeeded: %s", result.Reason),
			}
		}

		if result.NeedsHuman {
			// Don't retry if human intervention is explicitly needed
			return MergeOutcome{
				Success: false,
				Error:   fmt.Errorf("human intervention required"),
				Reason:  result.Reason,
			}
		}

		lastErr = fmt.Errorf("semantic merge failed: %s", result.Reason)
	}

	// All semantic merge attempts exhausted - escalate to human if resolver available
	if e.humanResolver != nil {
		debugLog("[merge-executor] escalating to human resolution for task %s after %d attempts", req.TaskID, e.config.MaxRetries+1)
		return e.escalateToHuman(ctx, req, conflictFiles, e.config.MaxRetries+1)
	}

	// No human resolver - BLOCK ORCHESTRATOR AND SPAWN DEDICATED AGENT
	if e.orchestrator != nil && e.factory != nil {
		debugLog("[merge-executor] spawning dedicated merge resolver agent for task %s", req.TaskID)

		// Set merge conflict state - blocks all scheduling
		e.orchestrator.SetMergeConflict(req.TaskID, conflictFiles)

		// Spawn merge resolver agent asynchronously
		go e.spawnMergeResolverAgent(ctx, req, conflictFiles)
	}

	return MergeOutcome{
		Success:       false,
		Error:         lastErr,
		Reason:        fmt.Sprintf("semantic merge failed after %d attempts, spawning dedicated resolver", e.config.MaxRetries+1),
		ConflictFiles: conflictFiles,
	}
}

// spawnMergeResolverAgent creates a dedicated agent to resolve merge conflicts.
func (e *MergeProcessor) spawnMergeResolverAgent(ctx context.Context, req *MergeRequest, conflictFiles []string) {
	if e.factory == nil {
		debugLog("[merge-executor] ERROR: no claude factory for merge resolver")
		return
	}

	// Create merge resolver
	resolver := NewMergeResolverAgent(
		&claudeFactoryAdapter{factory: e.factory},
		e.git,
		e.repoPath,
		e.orchestrator,
	)

	// Attempt resolution
	if err := resolver.Resolve(ctx, req, conflictFiles); err != nil {
		debugLog("[merge-executor] ERROR: merge resolver failed: %v", err)
		// Conflict remains - orchestrator stays blocked
		// User will need to intervene manually
	} else {
		debugLog("[merge-executor] SUCCESS: merge resolver completed - scheduling resumed")
	}
}

// claudeFactoryAdapter adapts the semantic merger factory to the ClaudeRunnerFactory interface.
type claudeFactoryAdapter struct {
	factory func() *SemanticMerger
}

func (a *claudeFactoryAdapter) NewRunner() agent.ClaudeRunner {
	// This is a workaround since we don't have direct access to the ClaudeRunnerFactory
	// The factory creates SemanticMerger which internally has a claude runner
	// For now, return nil and rely on the fact that we won't use this path
	// TODO: Refactor to pass ClaudeRunnerFactory directly
	return nil
}

// escalateToHuman presents conflicts to a human for resolution.
func (e *MergeProcessor) escalateToHuman(ctx context.Context, req *MergeRequest, conflictFiles []string, attemptNumber int) MergeOutcome {
	debugLog("[merge-executor] presenting %d conflicts to human for task %s", len(conflictFiles), req.TaskID)

	// Get target branch
	targetBranch := e.sessionBranch
	if e.greenfield {
		targetBranch = "main"
	}

	// Create conflict presenter
	presenter := merge.NewConflictPresenter(e.repoPath, git.NewRunner(e.repoPath))

	// Analyze conflicts
	presentations, err := presenter.AnalyzeMultipleConflicts(
		ctx,
		conflictFiles,
		targetBranch,
		req.AgentBranch,
		req.TaskID,
		req.AgentID,
		attemptNumber,
	)
	if err != nil {
		return MergeOutcome{
			Success: false,
			Error:   fmt.Errorf("failed to analyze conflicts: %w", err),
			Reason:  "conflict analysis failed",
		}
	}

	// Present conflicts to human
	resolution, err := e.humanResolver.PresentMultipleConflicts(ctx, presentations)
	if err != nil {
		return MergeOutcome{
			Success: false,
			Error:   fmt.Errorf("human resolution failed: %w", err),
			Reason:  "user declined to resolve conflicts",
		}
	}

	// Apply the resolution
	return e.applyHumanResolution(req, resolution, conflictFiles)
}

// applyHumanResolution applies the user's resolution choice.
func (e *MergeProcessor) applyHumanResolution(req *MergeRequest, resolution merge.Resolution, conflictFiles []string) MergeOutcome {
	switch resolution.Strategy {
	case merge.AcceptSession:
		debugLog("[merge-executor] applying AcceptSession resolution for task %s", req.TaskID)
		// Abort the merge and keep session state
		_ = e.merger.AbortMerge()
		return MergeOutcome{
			Success: true,
			Reason:  "user chose to keep session branch version",
		}

	case merge.AcceptAgent:
		debugLog("[merge-executor] applying AcceptAgent resolution for task %s", req.TaskID)
		// Accept all agent changes
		for _, file := range conflictFiles {
			_ = e.merger.CheckoutTheirs(file)
			_ = e.merger.StageFiles(file)
		}
		commitMsg := fmt.Sprintf("Merge agent-%s (user chose agent version)", req.TaskID)
		if err := e.merger.CommitMerge(commitMsg); err != nil {
			return MergeOutcome{
				Success: false,
				Error:   fmt.Errorf("failed to commit agent resolution: %w", err),
				Reason:  "commit failed after accepting agent changes",
			}
		}
		_ = e.merger.DeleteBranch(req.AgentBranch)
		return MergeOutcome{
			Success: true,
			Reason:  "user chose to accept agent branch version",
		}

	case merge.ManualMerge:
		debugLog("[merge-executor] applying ManualMerge resolution for task %s", req.TaskID)
		// User provided manually merged content
		// Write the merged content to files and commit
		for filePath, content := range resolution.SelectedFiles {
			if err := e.writeAndStageFile(filePath, content); err != nil {
				return MergeOutcome{
					Success: false,
					Error:   fmt.Errorf("failed to write merged file %s: %w", filePath, err),
					Reason:  "failed to apply manual merge",
				}
			}
		}
		commitMsg := fmt.Sprintf("Manual merge for agent-%s (user resolution)", req.TaskID)
		if err := e.merger.CommitMerge(commitMsg); err != nil {
			return MergeOutcome{
				Success: false,
				Error:   fmt.Errorf("failed to commit manual merge: %w", err),
				Reason:  "commit failed after manual merge",
			}
		}
		_ = e.merger.DeleteBranch(req.AgentBranch)
		return MergeOutcome{
			Success: true,
			Reason:  "user manually resolved conflicts",
		}

	case merge.SkipAgent:
		debugLog("[merge-executor] skipping agent for task %s per user request", req.TaskID)
		// Abort merge and mark task as blocked
		_ = e.merger.AbortMerge()
		return MergeOutcome{
			Success: false,
			Error:   fmt.Errorf("user chose to skip this agent's work"),
			Reason:  "user skipped agent work",
		}

	case merge.AbortSession:
		debugLog("[merge-executor] aborting session for task %s per user request", req.TaskID)
		_ = e.merger.AbortMerge()
		return MergeOutcome{
			Success: false,
			Error:   fmt.Errorf("user chose to abort the session"),
			Reason:  "user aborted session",
		}

	default:
		return MergeOutcome{
			Success: false,
			Error:   fmt.Errorf("unknown resolution strategy: %v", resolution.Strategy),
			Reason:  "invalid resolution strategy",
		}
	}
}

// writeAndStageFile writes content to a file and stages it.
func (e *MergeProcessor) writeAndStageFile(filePath, content string) error {
	fullPath := filepath.Join(e.repoPath, filePath)
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return e.merger.StageFiles(filePath)
}

// ConflictFiles returns the conflict files from a merge outcome context.
// This is used when passing to fallback strategy.
func (e *MergeProcessor) ConflictFiles(req *MergeRequest) []string {
	// Try git merge to get conflict files
	result, err := e.merger.Merge(req.AgentBranch)
	if err != nil || result.Success {
		return nil
	}
	return result.ConflictFiles
}
