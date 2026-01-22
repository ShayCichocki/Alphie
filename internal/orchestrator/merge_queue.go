// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/merge"
	"github.com/ShayCichocki/alphie/internal/orchestrator/policy"
)

// MergeRequest represents a pending merge operation.
type MergeRequest struct {
	// TaskID is the ID of the task being merged.
	TaskID string
	// AgentID is the ID of the agent that completed the task.
	AgentID string
	// AgentBranch is the branch containing the agent's work.
	AgentBranch string
	// Result is the execution result from the agent.
	Result *agent.ExecutionResult
	// ResultCh receives the merge outcome.
	ResultCh chan MergeOutcome
	// Context for cancellation.
	Ctx context.Context
}

// MergeOutcome represents the result of a merge operation.
type MergeOutcome struct {
	// Success indicates whether the merge completed successfully.
	Success bool
	// Error contains any error that occurred.
	Error error
	// FallbackUsed indicates whether we fell back from semantic merge.
	FallbackUsed bool
	// Reason provides context about the merge outcome.
	Reason string
	// ConflictFiles lists files that had conflicts (for fallback use).
	ConflictFiles []string
}

// MergeQueueConfig contains configuration for the merge queue.
type MergeQueueConfig struct {
	// MaxRetries is the maximum number of retry attempts for semantic merge.
	MaxRetries int
	// RetryBaseDelay is the base delay between retries (exponential backoff).
	RetryBaseDelay time.Duration
	// SemanticMergeTimeout is the maximum time to wait for semantic merge.
	SemanticMergeTimeout time.Duration
}

// DefaultMergeQueueConfig returns sensible defaults.
func DefaultMergeQueueConfig() MergeQueueConfig {
	return MergeQueueConfig{
		MaxRetries:           3,
		RetryBaseDelay:       2 * time.Second,
		SemanticMergeTimeout: 5 * time.Minute,
	}
}

// MergeQueue serializes merge operations to prevent race conditions.
// It processes merges one at a time, delegating to MergeProcessor for
// the core merge algorithm and FallbackStrategy for fallback handling.
type MergeQueue struct {
	// queue holds pending merge requests.
	queue chan *MergeRequest
	// merger is the git merge handler (also used by fallback).
	merger *merge.Handler
	// processor handles the core merge algorithm (git + semantic).
	processor *MergeProcessor
	// fallback handles fallback strategies when processor fails.
	fallback *FallbackStrategy
	// checkpoints manages checkpoint creation and rollback.
	checkpoints *merge.CheckpointManager
	// rollback handles rollback to previous checkpoints.
	rollback *merge.RollbackManager
	// stats tracks merge statistics.
	stats MergeQueueStats
	// mu protects stats.
	mu sync.RWMutex
	// wg tracks the worker goroutine.
	wg sync.WaitGroup
	// ctx is the queue's context.
	ctx context.Context
	// cancel cancels the queue's context.
	cancel context.CancelFunc
	// eventCh receives merge events for logging.
	eventCh chan<- OrchestratorEvent
}

// MergeQueueStats tracks merge queue statistics.
type MergeQueueStats struct {
	// TotalMerges is the total number of merges processed.
	TotalMerges int
	// SuccessfulMerges is the number of successful merges.
	SuccessfulMerges int
	// FailedMerges is the number of failed merges.
	FailedMerges int
	// SemanticMerges is the number of merges that required semantic resolution.
	SemanticMerges int
	// FallbackMerges is the number of merges that used fallback.
	FallbackMerges int
	// RetryCount is the total number of retry attempts.
	RetryCount int
}

// NewMergeQueue creates a new merge queue with default buffer size.
func NewMergeQueue(
	merger *merge.Handler,
	semanticMerger *SemanticMerger,
	semanticMergerFactory func() *SemanticMerger,
	sessionID string,
	sessionBranch string,
	greenfield bool,
	config MergeQueueConfig,
	eventCh chan<- OrchestratorEvent,
) *MergeQueue {
	return NewMergeQueueWithPolicy(merger, semanticMerger, semanticMergerFactory, sessionID, sessionBranch, greenfield, config, eventCh, nil)
}

// NewMergeQueueWithPolicy creates a new merge queue with configurable buffer size from policy.
func NewMergeQueueWithPolicy(
	merger *merge.Handler,
	semanticMerger *SemanticMerger,
	semanticMergerFactory func() *SemanticMerger,
	sessionID string,
	sessionBranch string,
	greenfield bool,
	config MergeQueueConfig,
	eventCh chan<- OrchestratorEvent,
	policyConfig *policy.Config,
) *MergeQueue {
	ctx, cancel := context.WithCancel(context.Background())

	// Determine buffer size from policy or use default
	bufferSize := 100
	if policyConfig != nil {
		bufferSize = policyConfig.Merge.QueueBufferSize
	}

	// Create processor config from queue config
	processorConfig := MergeProcessorConfig{
		MaxRetries:           config.MaxRetries,
		RetryBaseDelay:       config.RetryBaseDelay,
		SemanticMergeTimeout: config.SemanticMergeTimeout,
	}

	// Use NoOp resolver by default (will be replaced by orchestrator if interactive mode enabled)
	humanResolver := &merge.NoOpResolver{}

	// Create the merge processor (handles git + semantic merge)
	processor := NewMergeProcessor(
		merger,
		semanticMerger,
		semanticMergerFactory,
		processorConfig,
		sessionBranch,
		greenfield,
		humanResolver,
		merger.RepoPath(),
	)

	// Create the fallback strategy
	fallback := NewFallbackStrategy(merger, merger.RepoPath(), sessionBranch)

	// Create checkpoint and rollback managers
	gitRepo := merger.GitRunner()
	checkpoints := merge.NewCheckpointManager(sessionID, gitRepo)
	rollback := merge.NewRollbackManager(gitRepo, checkpoints)

	mq := &MergeQueue{
		queue:       make(chan *MergeRequest, bufferSize),
		merger:      merger,
		processor:   processor,
		fallback:    fallback,
		checkpoints: checkpoints,
		rollback:    rollback,
		eventCh:     eventCh,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start the worker goroutine
	mq.wg.Add(1)
	go mq.worker()

	return mq
}

// Enqueue adds a merge request to the queue.
// Returns a channel that will receive the merge outcome.
func (mq *MergeQueue) Enqueue(ctx context.Context, taskID, agentID, agentBranch string, result *agent.ExecutionResult) <-chan MergeOutcome {
	resultCh := make(chan MergeOutcome, 1)

	req := &MergeRequest{
		TaskID:      taskID,
		AgentID:     agentID,
		AgentBranch: agentBranch,
		Result:      result,
		ResultCh:    resultCh,
		Ctx:         ctx,
	}

	select {
	case mq.queue <- req:
		debugLog("[merge-queue] enqueued merge for task %s (queue size: %d)", taskID, len(mq.queue))
	case <-ctx.Done():
		resultCh <- MergeOutcome{
			Success: false,
			Error:   ctx.Err(),
			Reason:  "context cancelled before enqueue",
		}
	case <-mq.ctx.Done():
		resultCh <- MergeOutcome{
			Success: false,
			Error:   mq.ctx.Err(),
			Reason:  "merge queue shutting down",
		}
	}

	return resultCh
}

// Stop gracefully shuts down the merge queue.
func (mq *MergeQueue) Stop() {
	mq.cancel()
	close(mq.queue)
	mq.wg.Wait()
}

// Cleanup removes all checkpoint tags for this session.
// Should be called on successful session completion.
func (mq *MergeQueue) Cleanup() error {
	if mq.checkpoints != nil {
		return mq.checkpoints.Cleanup()
	}
	return nil
}

// GetCheckpoints returns the checkpoint manager for rollback operations.
func (mq *MergeQueue) GetCheckpoints() *merge.CheckpointManager {
	return mq.checkpoints
}

// GetRollback returns the rollback manager for rollback operations.
func (mq *MergeQueue) GetRollback() *merge.RollbackManager {
	return mq.rollback
}

// Stats returns a copy of the current statistics.
func (mq *MergeQueue) Stats() MergeQueueStats {
	mq.mu.RLock()
	defer mq.mu.RUnlock()
	return mq.stats
}

// QueueLength returns the number of pending merges.
func (mq *MergeQueue) QueueLength() int {
	return len(mq.queue)
}

// SetHumanResolver sets the human merge resolver for interactive conflict resolution.
// This should be called by the orchestrator to enable interactive mode.
func (mq *MergeQueue) SetHumanResolver(resolver merge.HumanMergeResolver) {
	if mq.processor != nil {
		mq.processor.humanResolver = resolver
	}
}

// worker processes merge requests sequentially.
func (mq *MergeQueue) worker() {
	defer mq.wg.Done()

	for req := range mq.queue {
		// Check if we should stop
		select {
		case <-mq.ctx.Done():
			req.ResultCh <- MergeOutcome{
				Success: false,
				Error:   mq.ctx.Err(),
				Reason:  "merge queue shutting down",
			}
			continue
		default:
		}

		// Process the merge
		outcome := mq.processMerge(req)

		// Update stats
		mq.mu.Lock()
		mq.stats.TotalMerges++
		if outcome.Success {
			mq.stats.SuccessfulMerges++
		} else {
			mq.stats.FailedMerges++
		}
		if outcome.FallbackUsed {
			mq.stats.FallbackMerges++
		}
		mq.mu.Unlock()

		// Send result
		req.ResultCh <- outcome
	}
}

// processMerge handles a single merge request by delegating to processor and fallback.
func (mq *MergeQueue) processMerge(req *MergeRequest) MergeOutcome {
	// Create checkpoint before merge attempt
	if mq.checkpoints != nil {
		if err := mq.checkpoints.CreateCheckpoint(req.AgentID, req.TaskID); err != nil {
			log.Printf("[merge_queue] warning: failed to create checkpoint for agent %s: %v", req.AgentID, err)
		}
	}

	mq.emitEvent(OrchestratorEvent{
		Type:      EventMergeStarted,
		TaskID:    req.TaskID,
		AgentID:   req.AgentID,
		Message:   "Starting merge operation",
		Timestamp: time.Now(),
	})

	// Delegate to processor for git + semantic merge
	outcome := mq.processor.Execute(req.Ctx, req)

	if outcome.Success {
		// Mark checkpoint as good
		if mq.checkpoints != nil {
			if err := mq.checkpoints.MarkGood(req.AgentID); err != nil {
				log.Printf("[merge_queue] warning: failed to mark checkpoint as good for agent %s: %v", req.AgentID, err)
			}
		}

		mq.emitEvent(OrchestratorEvent{
			Type:      EventMergeCompleted,
			TaskID:    req.TaskID,
			AgentID:   req.AgentID,
			Message:   fmt.Sprintf("Merge completed: %s", outcome.Reason),
			Timestamp: time.Now(),
		})
		return outcome
	}

	// Track semantic merge attempts
	if len(outcome.ConflictFiles) > 0 {
		mq.mu.Lock()
		mq.stats.SemanticMerges++
		mq.mu.Unlock()
	}

	// Processor failed, try fallback strategy
	if len(outcome.ConflictFiles) > 0 {
		conflictSummary := fmt.Sprintf("Attempting fallback merge strategy for %d conflict file(s)", len(outcome.ConflictFiles))
		log.Printf("[merge_queue] %s for task %s: %v", conflictSummary, req.TaskID, outcome.ConflictFiles)

		mq.emitEvent(OrchestratorEvent{
			Type:      EventMergeStarted,
			TaskID:    req.TaskID,
			AgentID:   req.AgentID,
			Message:   conflictSummary,
			Timestamp: time.Now(),
		})

		fallbackOutcome := mq.fallback.Attempt(req, outcome.ConflictFiles)
		if fallbackOutcome.Success {
			// Fallback succeeded - mark checkpoint as good
			if mq.checkpoints != nil {
				if err := mq.checkpoints.MarkGood(req.AgentID); err != nil {
					log.Printf("[merge_queue] warning: failed to mark checkpoint as good for agent %s: %v", req.AgentID, err)
				}
			}

			_ = mq.merger.DeleteBranch(req.AgentBranch)
			successMsg := fmt.Sprintf("Fallback merge completed: %s", fallbackOutcome.Reason)
			log.Printf("[merge_queue] %s for task %s", successMsg, req.TaskID)

			mq.emitEvent(OrchestratorEvent{
				Type:      EventMergeCompleted,
				TaskID:    req.TaskID,
				AgentID:   req.AgentID,
				Message:   successMsg,
				Timestamp: time.Now(),
			})
		} else {
			// Fallback failed - mark checkpoint as bad
			if mq.checkpoints != nil {
				if err := mq.checkpoints.MarkBad(req.AgentID); err != nil {
					log.Printf("[merge_queue] warning: failed to mark checkpoint as bad for agent %s: %v", req.AgentID, err)
				}
			}

			// Build detailed error message with conflict files
			errorMsg := fmt.Sprintf("Merge failed after fallback attempt: %s. Conflict files: %v",
				fallbackOutcome.Reason, outcome.ConflictFiles)
			log.Printf("[merge_queue] ERROR: %s for task %s (error: %v)", errorMsg, req.TaskID, fallbackOutcome.Error)

			mq.emitEvent(OrchestratorEvent{
				Type:      EventMergeCompleted,
				TaskID:    req.TaskID,
				AgentID:   req.AgentID,
				Message:   errorMsg,
				Error:     fallbackOutcome.Error,
				Timestamp: time.Now(),
			})
		}
		return fallbackOutcome
	}

	// No conflict files to attempt fallback - mark checkpoint as bad
	if mq.checkpoints != nil {
		if err := mq.checkpoints.MarkBad(req.AgentID); err != nil {
			log.Printf("[merge_queue] warning: failed to mark checkpoint as bad for agent %s: %v", req.AgentID, err)
		}
	}

	// Build detailed error message
	errorMsg := fmt.Sprintf("Merge failed: %s", outcome.Reason)
	if outcome.Error != nil {
		errorMsg = fmt.Sprintf("%s (error: %v)", errorMsg, outcome.Error)
	}
	log.Printf("[merge_queue] ERROR: %s for task %s", errorMsg, req.TaskID)

	mq.emitEvent(OrchestratorEvent{
		Type:      EventMergeCompleted,
		TaskID:    req.TaskID,
		AgentID:   req.AgentID,
		Message:   errorMsg,
		Error:     outcome.Error,
		Timestamp: time.Now(),
	})
	return outcome
}

// emitEvent sends an event if the event channel is configured.
func (mq *MergeQueue) emitEvent(event OrchestratorEvent) {
	if mq.eventCh == nil {
		return
	}
	select {
	case mq.eventCh <- event:
	default:
		// Don't block if channel is full
	}
}

