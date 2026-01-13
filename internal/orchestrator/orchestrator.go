// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/internal/config"
	"github.com/shayc/alphie/internal/learning"
	"github.com/shayc/alphie/internal/prog"
	"github.com/shayc/alphie/internal/state"
	"github.com/shayc/alphie/pkg/models"
)

// EventType represents the type of orchestrator event.
type EventType string

const (
	// EventTaskStarted indicates a task has started execution.
	EventTaskStarted EventType = "task_started"
	// EventTaskCompleted indicates a task completed successfully.
	EventTaskCompleted EventType = "task_completed"
	// EventTaskFailed indicates a task failed.
	EventTaskFailed EventType = "task_failed"
	// EventMergeStarted indicates a merge operation has started.
	EventMergeStarted EventType = "merge_started"
	// EventMergeCompleted indicates a merge operation completed.
	EventMergeCompleted EventType = "merge_completed"
	// EventSecondReviewStarted indicates a second review has started.
	EventSecondReviewStarted EventType = "second_review_started"
	// EventSecondReviewCompleted indicates a second review has completed.
	EventSecondReviewCompleted EventType = "second_review_completed"
	// EventSessionDone indicates the entire session is complete.
	EventSessionDone EventType = "session_done"
	// EventTaskBlocked indicates a task is blocked and cannot proceed.
	EventTaskBlocked EventType = "task_blocked"
	// EventTaskQueued indicates a task is ready and queued for execution.
	EventTaskQueued EventType = "task_queued"
	// EventAgentProgress provides periodic updates on agent execution.
	EventAgentProgress EventType = "agent_progress"
)

// OrchestratorEvent represents an event emitted by the orchestrator.
// These events are used to update the TUI and track progress.
type OrchestratorEvent struct {
	// Type is the kind of event.
	Type EventType
	// TaskID is the ID of the related task, if applicable.
	TaskID string
	// TaskTitle is the title of the related task, if applicable.
	TaskTitle string
	// AgentID is the ID of the related agent, if applicable.
	AgentID string
	// Message provides additional context about the event.
	Message string
	// Error contains error details for failure events.
	Error error
	// Timestamp is when the event occurred.
	Timestamp time.Time
	// TokensUsed is the current total tokens used (for progress events).
	TokensUsed int64
	// Cost is the current total cost (for progress events).
	Cost float64
	// Duration is the elapsed time (for progress events).
	Duration time.Duration
	// LogFile is the path to the detailed execution log.
	LogFile string
}

// OrchestratorConfig contains configuration options for the Orchestrator.
type OrchestratorConfig struct {
	// RepoPath is the path to the git repository.
	RepoPath string
	// Tier is the agent tier for task execution.
	Tier models.Tier
	// MaxAgents is the maximum number of concurrent agents.
	// If 0, the value is taken from TierConfigs (or defaults to 4).
	MaxAgents int
	// TierConfigs holds loaded tier configurations from YAML.
	// If nil, hardcoded defaults are used.
	TierConfigs *config.TierConfigs
	// Greenfield indicates if this is a new project (no session branch needed).
	Greenfield bool
	// DecomposerClaude is the Claude process for task decomposition.
	DecomposerClaude *agent.ClaudeProcess
	// MergerClaude is the Claude process for semantic merge operations.
	MergerClaude *agent.ClaudeProcess
	// SecondReviewerClaude is the Claude process for second review of high-risk diffs.
	// If nil, second review is disabled.
	SecondReviewerClaude *agent.ClaudeProcess
	// Executor is the agent executor for running tasks.
	Executor *agent.Executor
	// StateDB is the database for persisting session/agent/task state.
	StateDB *state.DB
	// LearningSystem provides learning retrieval and recording capabilities.
	// If nil, learning features are disabled.
	LearningSystem *learning.LearningSystem
	// ProgClient provides cross-session task management capabilities.
	// If nil, prog features are disabled.
	ProgClient *prog.Client
	// ResumeEpicID is an existing prog epic ID to resume.
	// If set, the orchestrator will load tasks from this epic instead of decomposing.
	// Completed tasks will be skipped, and in-progress/open tasks will be executed.
	ResumeEpicID string
}

// Orchestrator coordinates the entire workflow from request to completion.
// It wires together: decomposer -> graph -> scheduler -> agents -> merger.
type Orchestrator struct {
	decomposer     *Decomposer
	graph          *DependencyGraph
	scheduler      *Scheduler
	merger         *MergeHandler
	semanticMerger *SemanticMerger
	secondReviewer *SecondReviewer
	sessionMgr     *SessionBranchManager
	collision      *CollisionChecker
	protected      *ProtectedAreaDetector
	overrideGate   *ScoutOverrideGate
	learnings      *learning.LearningSystem
	progClient     *prog.Client
	tier           models.Tier
	executor       *agent.Executor
	repoPath       string
	maxAgents      int
	greenfield     bool
	stateDB        *state.DB
	sessionID      string

	// events is the channel for emitting orchestrator events.
	events chan OrchestratorEvent
	// stopCh signals the orchestrator to stop.
	stopCh chan struct{}
	// stopped indicates whether Stop has been called.
	stopped bool
	// mu protects mutable state.
	mu sync.RWMutex
	// wg tracks running goroutines.
	wg sync.WaitGroup
	// agentResults collects results from completed agents.
	agentResults map[string]*agent.ExecutionResult
	// resultsMu protects agentResults.
	resultsMu sync.Mutex
	// paused indicates whether the orchestrator is paused (no new agents will spawn).
	paused bool
	// pauseCond is used to signal when the orchestrator is unpaused.
	pauseCond *sync.Cond

	// progTaskIDs maps internal task IDs to prog task IDs.
	// This enables status updates to the cross-session tracking system.
	progTaskIDs map[string]string
	// progEpicID stores the prog epic ID for the current session.
	progEpicID string
}

// NewOrchestrator creates a new Orchestrator with the given configuration.
func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator {
	sessionID := uuid.New().String()[:8]

	// Create components
	decomposer := NewDecomposer(cfg.DecomposerClaude)
	graph := NewDependencyGraph()
	collision := NewCollisionChecker()
	protected := NewProtectedAreaDetector()

	// Create scout override gate for protected area question allowance
	// Use config from TierConfigs if available
	overrideCfg := DefaultScoutOverrideConfig()
	if cfg.TierConfigs != nil && cfg.TierConfigs.Scout != nil && cfg.TierConfigs.Scout.OverrideGates != nil {
		og := cfg.TierConfigs.Scout.OverrideGates
		overrideCfg.BlockedAfterNAttempts = og.BlockedAfterNAttempts
		overrideCfg.ProtectedAreaDetected = og.ProtectedAreaDetected
	}
	overrideGate := NewScoutOverrideGate(protected, overrideCfg)

	// Set up orchestrator tier configs for QuestionsAllowed
	if cfg.TierConfigs != nil {
		SetOrchestratorTierConfigs(cfg.TierConfigs)
	}

	// Session branch manager
	sessionMgr := NewSessionBranchManager(sessionID, cfg.RepoPath, cfg.Greenfield)

	// Merge handlers - target session branch normally, or main directly in greenfield mode
	var merger *MergeHandler
	var semanticMerger *SemanticMerger
	var secondReviewer *SecondReviewer
	if cfg.Greenfield {
		// Greenfield: merge agent branches directly to main
		merger = NewMergeHandler("main", cfg.RepoPath)
		semanticMerger = NewSemanticMerger(cfg.MergerClaude, cfg.RepoPath)
	} else {
		// Normal: merge to session branch first
		merger = NewMergeHandler(sessionMgr.GetBranchName(), cfg.RepoPath)
		semanticMerger = NewSemanticMerger(cfg.MergerClaude, cfg.RepoPath)
		// Second reviewer is optional - only created if Claude is configured
		if cfg.SecondReviewerClaude != nil {
			secondReviewer = NewSecondReviewer(protected, cfg.SecondReviewerClaude)
		}
	}

	// Determine maxAgents from config or TierConfigs
	maxAgents := cfg.MaxAgents
	if maxAgents <= 0 && cfg.TierConfigs != nil {
		tierCfg := cfg.TierConfigs.Get(cfg.Tier)
		if tierCfg != nil && tierCfg.MaxAgents > 0 {
			maxAgents = tierCfg.MaxAgents
		}
	}
	if maxAgents <= 0 {
		maxAgents = 4 // Default to 4 concurrent agents
	}

	o := &Orchestrator{
		decomposer:     decomposer,
		graph:          graph,
		scheduler:      nil, // Created in Run after graph is built
		merger:         merger,
		semanticMerger: semanticMerger,
		secondReviewer: secondReviewer,
		sessionMgr:     sessionMgr,
		collision:      collision,
		protected:      protected,
		overrideGate:   overrideGate,
		learnings:      cfg.LearningSystem,
		progClient:     cfg.ProgClient,
		tier:           cfg.Tier,
		executor:       cfg.Executor,
		repoPath:       cfg.RepoPath,
		maxAgents:      maxAgents,
		greenfield:     cfg.Greenfield,
		stateDB:        cfg.StateDB,
		sessionID:      sessionID,
		progEpicID:     cfg.ResumeEpicID,
		events:         make(chan OrchestratorEvent, 100),
		stopCh:         make(chan struct{}),
		agentResults:   make(map[string]*agent.ExecutionResult),
		progTaskIDs:    make(map[string]string),
	}
	o.pauseCond = sync.NewCond(&o.mu)
	return o
}

// Run executes the full orchestration workflow:
//  1. Decompose request into tasks (or resume from existing epic)
//  2. Build dependency graph
//  3. Create session branch
//  4. Loop: schedule -> spawn agents -> wait -> merge
//  5. Cleanup session or create PR
func (o *Orchestrator) Run(ctx context.Context, request string) error {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return fmt.Errorf("orchestrator has been stopped")
	}
	o.mu.Unlock()

	// Create a derived context that we can cancel
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Monitor stop channel
	go func() {
		select {
		case <-o.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Create session in state DB
	if err := o.createSessionState(request); err != nil {
		return fmt.Errorf("create session state: %w", err)
	}

	var tasks []*models.Task
	var err error

	// Check if we're resuming from an existing epic
	if o.progEpicID != "" {
		tasks, err = o.loadTasksFromProgEpic(ctx)
		if err != nil {
			o.updateSessionStatus(state.SessionFailed)
			return fmt.Errorf("load tasks from prog epic %s: %w", o.progEpicID, err)
		}
		log.Printf("[orchestrator] resuming epic %s with %d tasks", o.progEpicID, len(tasks))
	} else {
		// Step 1: Decompose request into tasks
		tasks, err = o.decomposer.Decompose(ctx, request)
		if err != nil {
			o.updateSessionStatus(state.SessionFailed)
			return fmt.Errorf("decompose request: %w", err)
		}

		if len(tasks) == 0 {
			o.updateSessionStatus(state.SessionFailed)
			return fmt.Errorf("no tasks generated from request")
		}

		// Create prog epic and tasks for cross-session tracking
		if err := o.createProgEpicAndTasks(request, tasks); err != nil {
			// Log but don't fail - prog is optional tracking
			log.Printf("[orchestrator] warning: failed to create prog epic/tasks: %v", err)
		}
	}

	// Persist tasks to state DB
	if err := o.persistTasks(tasks); err != nil {
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("persist tasks: %w", err)
	}

	// Step 2: Build dependency graph
	if err := o.graph.Build(tasks); err != nil {
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("build dependency graph: %w", err)
	}

	// Create scheduler now that graph is built
	o.scheduler = NewScheduler(o.graph, o.tier, o.maxAgents)
	o.scheduler.SetCollisionChecker(o.collision)

	// Step 3: Create session branch
	if err := o.sessionMgr.CreateBranch(); err != nil {
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("create session branch: %w", err)
	}

	// Step 4: Main execution loop
	if err := o.runLoop(ctx); err != nil {
		// On error, try to cleanup the session
		if !o.greenfield {
			_ = o.sessionMgr.Cleanup()
		} else {
			// In greenfield mode, ensure we're back on main branch
			_ = o.checkoutMain()
		}
		o.updateSessionStatus(state.SessionFailed)
		return fmt.Errorf("execution loop: %w", err)
	}

	// Step 5: Merge session branch to main
	if !o.greenfield && o.sessionMgr != nil {
		if err := o.sessionMgr.MergeToMain(); err != nil {
			log.Printf("[orchestrator] warning: failed to merge session to main: %v", err)
			// Don't fail the whole session for merge errors - work is still preserved on session branch
		} else {
			log.Printf("[orchestrator] merged session branch to main")
			// Cleanup the session branch now that it's merged
			if err := o.sessionMgr.Cleanup(); err != nil {
				log.Printf("[orchestrator] warning: failed to cleanup session branch: %v", err)
			}
		}
	}

	// Step 6: Mark session completed and emit done event
	o.updateSessionStatus(state.SessionCompleted)

	// Update prog epic status if all tasks are complete
	if o.progEpicID != "" && o.progClient != nil {
		if done, err := o.progClient.UpdateEpicStatusIfComplete(o.progEpicID); err != nil {
			log.Printf("[orchestrator] warning: failed to update epic status: %v", err)
		} else if done {
			log.Printf("[orchestrator] epic %s marked as done", o.progEpicID)
		}
	}

	o.emitEvent(OrchestratorEvent{
		Type:      EventSessionDone,
		Message:   "All tasks completed successfully",
		Timestamp: time.Now(),
	})

	return nil
}

// loadTasksFromProgEpic loads tasks from an existing prog epic for resumption.
// Completed tasks are loaded with status Done so they will be skipped.
// In-progress tasks are reset to Pending for re-execution.
func (o *Orchestrator) loadTasksFromProgEpic(ctx context.Context) ([]*models.Task, error) {
	if o.progClient == nil {
		return nil, fmt.Errorf("prog client not configured")
	}

	// Verify epic exists
	epic, err := o.progClient.GetEpic(o.progEpicID)
	if err != nil {
		return nil, fmt.Errorf("get epic: %w", err)
	}

	// Mark epic as in-progress if it's open
	if epic.Status == prog.StatusOpen {
		if err := o.progClient.Start(o.progEpicID); err != nil {
			log.Printf("[orchestrator] warning: failed to mark epic as in-progress: %v", err)
		}
	}

	// Get child tasks
	progTasks, err := o.progClient.GetChildTasks(o.progEpicID)
	if err != nil {
		return nil, fmt.Errorf("get child tasks: %w", err)
	}

	if len(progTasks) == 0 {
		return nil, fmt.Errorf("epic %s has no tasks", o.progEpicID)
	}

	// Convert prog tasks to internal tasks
	tasks := make([]*models.Task, 0, len(progTasks))
	for _, pt := range progTasks {
		// Map prog task ID to internal task ID for status sync
		internalID := uuid.New().String()[:8]
		o.progTaskIDs[internalID] = pt.ID

		// Convert status
		var status models.TaskStatus
		switch pt.Status {
		case prog.StatusDone:
			status = models.TaskStatusDone
		case prog.StatusCanceled:
			// Skip canceled tasks entirely
			continue
		default:
			// Open, in_progress, blocked all become pending for (re-)execution
			status = models.TaskStatusPending
		}

		task := &models.Task{
			ID:          internalID,
			Title:       pt.Title,
			Description: pt.Description,
			Status:      status,
			Tier:        o.tier,
			CreatedAt:   pt.CreatedAt,
		}

		tasks = append(tasks, task)
	}

	// Note: Dependencies from prog are not loaded here.
	// When resuming, tasks that were in-progress may have had their
	// dependencies already completed, so we execute them independently.
	// For more sophisticated dependency handling, we would need to map
	// prog dep IDs to internal task IDs.

	log.Printf("[orchestrator] loaded %d tasks from epic (skipped canceled, %d already done)",
		len(tasks), countDoneTasks(tasks))

	return tasks, nil
}

// countDoneTasks counts tasks with Done status.
func countDoneTasks(tasks []*models.Task) int {
	count := 0
	for _, t := range tasks {
		if t.Status == models.TaskStatusDone {
			count++
		}
	}
	return count
}

// runLoop is the main execution loop that schedules, spawns, and merges work.
func (o *Orchestrator) runLoop(ctx context.Context) error {
	// Track in-flight tasks
	type inflight struct {
		taskID   string
		agentID  string
		doneCh   chan *agent.ExecutionResult
		cancelFn context.CancelFunc
	}

	inflightTasks := make(map[string]*inflight)
	var inflightMu sync.Mutex

	// Aggregate channel for completion notifications
	completionCh := make(chan string, o.maxAgents)

	for {
		select {
		case <-ctx.Done():
			// Cancel all in-flight tasks
			inflightMu.Lock()
			for _, inf := range inflightTasks {
				inf.cancelFn()
			}
			inflightMu.Unlock()
			return ctx.Err()

		case agentID := <-completionCh:
			// Handle task completion
			inflightMu.Lock()
			var completedTask *inflight
			for _, inf := range inflightTasks {
				if inf.agentID == agentID {
					completedTask = inf
					break
				}
			}
			if completedTask != nil {
				delete(inflightTasks, completedTask.taskID)
			}
			inflightMu.Unlock()

			if completedTask != nil {
				// Get the result
				o.resultsMu.Lock()
				result := o.agentResults[agentID]
				o.resultsMu.Unlock()

				if result != nil {
					if err := o.handleTaskCompletion(ctx, completedTask.taskID, result); err != nil {
						return fmt.Errorf("handle completion for task %s: %w", completedTask.taskID, err)
					}
				}
			}

		default:
			// Check if we're done
			ready := o.scheduler.Schedule()
			inflightMu.Lock()
			inflightCount := len(inflightTasks)
			inflightMu.Unlock()

			if len(ready) == 0 && inflightCount == 0 {
				// No more tasks to schedule and none in flight - we're done
				return nil
			}

			if len(ready) == 0 {
				// Nothing to schedule, wait for completions
				select {
				case <-ctx.Done():
					return ctx.Err()
				case agentID := <-completionCh:
					// Re-process this completion in the next iteration
					go func() { completionCh <- agentID }()
				case <-time.After(100 * time.Millisecond):
					// Small delay to avoid busy-waiting
				}
				continue
			}

			// Check if paused - wait until resumed before spawning new agents
			o.mu.Lock()
			for o.paused && !o.stopped {
				// Wait for resume signal or context cancellation
				// Use a goroutine to check context and signal condition
				done := make(chan struct{})
				go func() {
					select {
					case <-ctx.Done():
						o.mu.Lock()
						o.pauseCond.Broadcast()
						o.mu.Unlock()
					case <-done:
					}
				}()
				o.pauseCond.Wait()
				close(done)
				if ctx.Err() != nil {
					o.mu.Unlock()
					return ctx.Err()
				}
			}
			if o.stopped {
				o.mu.Unlock()
				return fmt.Errorf("orchestrator stopped")
			}
			o.mu.Unlock()

			// Spawn agents for ready tasks
			for _, task := range ready {
				// Emit task queued event
				o.emitEvent(OrchestratorEvent{
					Type:      EventTaskQueued,
					TaskID:    task.ID,
					TaskTitle: task.Title,
					Message:   fmt.Sprintf("Task queued: %s", task.Title),
					Timestamp: time.Now(),
				})

				// Check for protected areas
				isProtected := o.protected.IsProtected(task.Description) || o.protected.IsProtected(task.Title)
				if isProtected {
					// For Scout tier, mark in override gate to allow questions
					// For other tiers, they can already ask questions, so proceed
					if o.tier == models.TierScout {
						o.overrideGate.SetProtectedArea(task.ID, true)
						log.Printf("[orchestrator] task %s touches protected area, Scout can now ask questions", task.ID)
					}
					// Note: We no longer block the task - let the agent proceed with question capability
				}

				// Retrieve relevant learnings for this task
				var taskLearnings []*learning.Learning
				if o.learnings != nil {
					learnings, err := o.learnings.OnTaskStart(task.Description, nil)
					if err != nil {
						log.Printf("[orchestrator] warning: failed to retrieve learnings for task %s: %v", task.ID, err)
					} else if len(learnings) > 0 {
						taskLearnings = learnings
						log.Printf("[orchestrator] retrieved %d learnings for task %s", len(learnings), task.ID)
					}
				}

				// Create agent context
				taskCtx, taskCancel := context.WithCancel(ctx)

				// Create agent
				agentModel := &models.Agent{
					ID:        uuid.New().String(),
					TaskID:    task.ID,
					Status:    models.AgentStatusRunning,
					StartedAt: time.Now(),
				}

				// Persist agent state
				o.createAgentState(agentModel)

				// Register with scheduler
				o.scheduler.OnAgentStart(agentModel)

				// Register with collision checker
				hints := &SchedulerHint{
					PathPrefixes: o.collision.extractPathPrefixes(task),
				}
				o.collision.RegisterAgent(agentModel.ID, hints)

				// Track in-flight task
				inf := &inflight{
					taskID:   task.ID,
					agentID:  agentModel.ID,
					doneCh:   make(chan *agent.ExecutionResult, 1),
					cancelFn: taskCancel,
				}

				inflightMu.Lock()
				inflightTasks[task.ID] = inf
				inflightMu.Unlock()

				// Update task status and assign to agent
				task.Status = models.TaskStatusInProgress
				task.AssignedTo = agentModel.ID
				o.updateTaskState(task)

				// Emit event
				o.emitEvent(OrchestratorEvent{
					Type:      EventTaskStarted,
					TaskID:    task.ID,
					TaskTitle: task.Title,
					AgentID:   agentModel.ID,
					Message:   fmt.Sprintf("Started task: %s", task.Title),
					Timestamp: time.Now(),
				})

				// Update prog task status to in_progress
				o.progStartTask(task.ID)

				// Spawn agent goroutine
				o.wg.Add(1)
				go func(t *models.Task, a *models.Agent, taskCtx context.Context, learnings []*learning.Learning) {
					defer o.wg.Done()

					opts := &agent.ExecuteOptions{
						Learnings: learnings,
						OnProgress: func(update agent.ProgressUpdate) {
							o.emitEvent(OrchestratorEvent{
								Type:       EventAgentProgress,
								TaskID:     t.ID,
								AgentID:    update.AgentID,
								Message:    fmt.Sprintf("Agent progress: %d tokens, $%.4f", update.TokensUsed, update.Cost),
								Timestamp:  time.Now(),
								TokensUsed: update.TokensUsed,
								Cost:       update.Cost,
								Duration:   update.Duration,
							})
						},
					}
					result, err := o.executor.ExecuteWithOptions(taskCtx, t, o.tier, opts)
					if err != nil {
						result = &agent.ExecutionResult{
							Success: false,
							Error:   err.Error(),
							AgentID: a.ID,
						}
					}

					// Store result
					o.resultsMu.Lock()
					o.agentResults[a.ID] = result
					o.resultsMu.Unlock()

					// Signal completion
					select {
					case completionCh <- a.ID:
					case <-taskCtx.Done():
					}
				}(task, agentModel, taskCtx, taskLearnings)
			}
		}
	}
}

// handleTaskCompletion processes a completed task and triggers merge if needed.
func (o *Orchestrator) handleTaskCompletion(ctx context.Context, taskID string, result *agent.ExecutionResult) error {
	task := o.graph.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task not found: %s", taskID)
	}

	// Unregister from collision checker
	o.collision.UnregisterAgent(result.AgentID)

	// Update scheduler
	o.scheduler.OnAgentComplete(result.AgentID)

	if result.Success {
		// Mark task as done
		task.Status = models.TaskStatusDone
		now := time.Now()
		task.CompletedAt = &now

		// Update state persistence
		o.updateTaskState(task)
		o.updateAgentState(result.AgentID, state.AgentDone)

		// Reset override gate state for this task
		if o.overrideGate != nil {
			o.overrideGate.Reset(taskID)
		}

		// Update prog task status to done
		o.progCompleteTask(taskID)

		// Capture learnings from successful completion
		o.captureLearningOnCompletion(task, result)

		// Emit completion event
		o.emitEvent(OrchestratorEvent{
			Type:      EventTaskCompleted,
			TaskID:    taskID,
			TaskTitle: task.Title,
			AgentID:   result.AgentID,
			Message:   fmt.Sprintf("Completed task: %s", task.Title),
			Timestamp: time.Now(),
			LogFile:   result.LogFile,
		})

		// Merge agent branch (to session branch, or directly to main in greenfield mode)
		if o.merger != nil {
			if err := o.performMerge(ctx, taskID, result); err != nil {
				return fmt.Errorf("merge failed: %w", err)
			}
		}
	} else {
		// Record failed attempt for scout override gate (blocked_after_n_attempts)
		if o.overrideGate != nil && o.tier == models.TierScout {
			attempts := o.overrideGate.RecordAttempt(taskID)
			if o.overrideGate.CanAskQuestion(taskID) {
				log.Printf("[orchestrator] task %s has %d failed attempts, Scout can now ask questions", taskID, attempts)
			}
		}

		// Mark task as failed
		task.Status = models.TaskStatusFailed

		// Update state persistence
		o.updateTaskState(task)
		o.updateAgentState(result.AgentID, state.AgentFailed)

		// Check learnings for known fixes to this error
		if o.learnings != nil && result.Error != "" {
			learnings, err := o.learnings.OnFailure(result.Error)
			if err != nil {
				log.Printf("[orchestrator] warning: failed to check learnings for error: %v", err)
			} else if len(learnings) > 0 {
				log.Printf("[orchestrator] found %d learnings for error in task %s", len(learnings), taskID)
				// Include suggested fixes in the event message
				var suggestions []string
				for _, l := range learnings {
					suggestions = append(suggestions, l.Action)
				}
				result.Error = fmt.Sprintf("%s (suggestions from learnings: %v)", result.Error, suggestions)
			}
		}

		// Update prog task status to blocked with failure reason
		o.progBlockTask(taskID, result.Error)

		// Emit failure event
		o.emitEvent(OrchestratorEvent{
			Type:      EventTaskFailed,
			TaskID:    taskID,
			TaskTitle: task.Title,
			AgentID:   result.AgentID,
			Message:   fmt.Sprintf("Task failed: %s", task.Title),
			Error:     fmt.Errorf("%s", result.Error),
			Timestamp: time.Now(),
			LogFile:   result.LogFile,
		})
	}

	return nil
}

// performMerge attempts to merge the agent's work into the session branch.
// If a second reviewer is configured and the diff triggers review conditions,
// a second review is performed before finalizing the merge.
func (o *Orchestrator) performMerge(ctx context.Context, taskID string, result *agent.ExecutionResult) error {
	// Log merge start to prog
	o.progLogTask(taskID, "Starting merge operation")

	// Emit merge started event
	o.emitEvent(OrchestratorEvent{
		Type:      EventMergeStarted,
		TaskID:    taskID,
		AgentID:   result.AgentID,
		Message:   "Starting merge operation",
		Timestamp: time.Now(),
	})

	// Construct agent branch name from the worktree
	// The agent branch is typically named after the task
	agentBranch := fmt.Sprintf("agent-%s", taskID)

	// Attempt merge - use retry logic for greenfield mode
	var mergeResult *MergeResult
	var err error
	if o.greenfield {
		// Greenfield mode: use intelligent retry with rebase
		mergeResult, err = o.merger.MergeWithRetry(agentBranch, 3)
	} else {
		// Normal mode: single merge attempt (already has one rebase retry built in)
		mergeResult, err = o.merger.Merge(agentBranch)
	}
	if err != nil {
		return fmt.Errorf("merge operation: %w", err)
	}

	if mergeResult.Success {
		// Log merge success to prog
		o.progLogTask(taskID, "Merge completed successfully")

		// Perform second review if configured
		if err := o.performSecondReview(ctx, taskID, result, mergeResult.Diff, mergeResult.ChangedFiles); err != nil {
			return fmt.Errorf("second review: %w", err)
		}

		// Clean up agent branch
		_ = o.merger.DeleteBranch(agentBranch)

		o.emitEvent(OrchestratorEvent{
			Type:      EventMergeCompleted,
			TaskID:    taskID,
			AgentID:   result.AgentID,
			Message:   "Merge completed successfully",
			Timestamp: time.Now(),
		})
		return nil
	}

	// If needs semantic merge, try that
	if mergeResult.NeedsSemanticMerge && o.semanticMerger != nil {
		// In greenfield mode, target branch is "main", otherwise use session branch
		targetBranch := o.sessionMgr.GetBranchName()
		if o.greenfield {
			targetBranch = "main"
		}
		semanticResult, err := o.semanticMerger.Merge(
			ctx,
			targetBranch,
			agentBranch,
			mergeResult.ConflictFiles,
		)
		if err != nil {
			return fmt.Errorf("semantic merge: %w", err)
		}

		if semanticResult.Success {
			// Log semantic merge success to prog
			o.progLogTask(taskID, fmt.Sprintf("Semantic merge completed: %s", semanticResult.Reason))

			// Perform second review if configured
			if err := o.performSecondReview(ctx, taskID, result, semanticResult.FinalDiff, semanticResult.ChangedFiles); err != nil {
				return fmt.Errorf("second review: %w", err)
			}

			// Clean up agent branch
			_ = o.merger.DeleteBranch(agentBranch)

			o.emitEvent(OrchestratorEvent{
				Type:      EventMergeCompleted,
				TaskID:    taskID,
				AgentID:   result.AgentID,
				Message:   fmt.Sprintf("Semantic merge completed: %s", semanticResult.Reason),
				Timestamp: time.Now(),
			})
			return nil
		}

		// Semantic merge also failed - needs human intervention
		if semanticResult.NeedsHuman {
			o.emitEvent(OrchestratorEvent{
				Type:      EventMergeCompleted,
				TaskID:    taskID,
				AgentID:   result.AgentID,
				Message:   fmt.Sprintf("Merge requires human intervention: %s", semanticResult.Reason),
				Error:     fmt.Errorf("human intervention required"),
				Timestamp: time.Now(),
			})
			return fmt.Errorf("merge requires human intervention: %s", semanticResult.Reason)
		}
	}

	// Merge failed
	return fmt.Errorf("merge failed: %v", mergeResult.Error)
}

// performSecondReview checks if a second review is needed and performs it.
// Returns nil if no review is needed or the review approves the changes.
// Returns an error if the review rejects the changes.
func (o *Orchestrator) performSecondReview(ctx context.Context, taskID string, result *agent.ExecutionResult, diff string, changedFiles []string) error {
	// Skip if second reviewer not configured
	if o.secondReviewer == nil {
		return nil
	}

	// Get the task for review context
	task := o.graph.GetTask(taskID)
	if task == nil {
		return nil // Shouldn't happen, but don't block merge
	}

	// Check if second review is triggered
	trigger := o.secondReviewer.ShouldSecondReview(diff, changedFiles, task)
	if !trigger.Triggered {
		return nil
	}

	// Emit second review started event
	o.emitEvent(OrchestratorEvent{
		Type:      EventSecondReviewStarted,
		TaskID:    taskID,
		AgentID:   result.AgentID,
		Message:   fmt.Sprintf("Second review triggered: %s", strings.Join(trigger.Reasons, "; ")),
		Timestamp: time.Now(),
	})

	log.Printf("[orchestrator] second review triggered for task %s: %v", taskID, trigger.Reasons)

	// Request the review
	reviewResult, err := o.secondReviewer.RequestReview(ctx, diff, task.Description)
	if err != nil {
		// Log the error but don't block the merge on review failure
		log.Printf("[orchestrator] warning: second review failed for task %s: %v", taskID, err)
		o.emitEvent(OrchestratorEvent{
			Type:      EventSecondReviewCompleted,
			TaskID:    taskID,
			AgentID:   result.AgentID,
			Message:   fmt.Sprintf("Second review failed: %v", err),
			Error:     err,
			Timestamp: time.Now(),
		})
		return nil // Don't block merge on review errors
	}

	// Process the review result
	if reviewResult.Approved {
		o.emitEvent(OrchestratorEvent{
			Type:      EventSecondReviewCompleted,
			TaskID:    taskID,
			AgentID:   result.AgentID,
			Message:   "Second review approved",
			Timestamp: time.Now(),
		})
		return nil
	}

	// Review rejected - block the merge
	concerns := strings.Join(reviewResult.Concerns, "; ")
	if concerns == "" {
		concerns = "no specific concerns provided"
	}

	o.emitEvent(OrchestratorEvent{
		Type:      EventSecondReviewCompleted,
		TaskID:    taskID,
		AgentID:   result.AgentID,
		Message:   fmt.Sprintf("Second review rejected: %s", concerns),
		Error:     fmt.Errorf("second review rejected"),
		Timestamp: time.Now(),
	})

	return fmt.Errorf("second review rejected: %s", concerns)
}

// Events returns a read-only channel of orchestrator events.
// This is used by the TUI to receive updates.
func (o *Orchestrator) Events() <-chan OrchestratorEvent {
	return o.events
}

// Stop signals the orchestrator to stop all work and clean up.
func (o *Orchestrator) Stop() error {
	o.mu.Lock()
	if o.stopped {
		o.mu.Unlock()
		return nil
	}
	o.stopped = true
	o.mu.Unlock()

	// Signal stop
	close(o.stopCh)

	// Wait for all goroutines to finish
	o.wg.Wait()

	// Close events channel
	close(o.events)

	// Cleanup session branch if not greenfield
	if !o.greenfield && o.sessionMgr != nil {
		if err := o.sessionMgr.Cleanup(); err != nil {
			return fmt.Errorf("cleanup session: %w", err)
		}
	} else if o.greenfield {
		// In greenfield mode, ensure we're back on main branch
		_ = o.checkoutMain()
	}

	return nil
}

// checkoutMain ensures the repository is on the main branch.
// This is used in greenfield mode to restore state after errors.
func (o *Orchestrator) checkoutMain() error {
	cmd := exec.Command("git", "checkout", "main")
	cmd.Dir = o.repoPath
	if err := cmd.Run(); err != nil {
		// Try master if main doesn't exist
		cmd = exec.Command("git", "checkout", "master")
		cmd.Dir = o.repoPath
		return cmd.Run()
	}
	return nil
}

// Pause pauses the orchestrator, preventing new agents from being spawned.
// Existing agents continue running until completion.
func (o *Orchestrator) Pause() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.paused {
		o.paused = true
		log.Printf("[orchestrator] paused - no new agents will be spawned")
	}
}

// Resume unpauses the orchestrator, allowing new agents to be spawned.
func (o *Orchestrator) Resume() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.paused {
		o.paused = false
		log.Printf("[orchestrator] resumed - agent spawning enabled")
		o.pauseCond.Broadcast()
	}
}

// IsPaused returns whether the orchestrator is currently paused.
func (o *Orchestrator) IsPaused() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.paused
}

// emitEvent sends an event to the events channel.
func (o *Orchestrator) emitEvent(event OrchestratorEvent) {
	select {
	case o.events <- event:
	default:
		// Channel full, drop event to avoid blocking
	}
}

// GetGraph returns the dependency graph for inspection.
func (o *Orchestrator) GetGraph() *DependencyGraph {
	return o.graph
}

// GetScheduler returns the scheduler for inspection.
func (o *Orchestrator) GetScheduler() *Scheduler {
	return o.scheduler
}

// GetSessionBranch returns the session branch name.
func (o *Orchestrator) GetSessionBranch() string {
	if o.sessionMgr != nil {
		return o.sessionMgr.GetBranchName()
	}
	return ""
}

// GetProgClient returns the prog client for cross-session task management.
// Returns nil if prog features are disabled.
func (o *Orchestrator) GetProgClient() *prog.Client {
	return o.progClient
}

// GetScoutOverrideGate returns the scout override gate for question allowance checks.
// This is used by components that need to check if a Scout agent can ask questions.
func (o *Orchestrator) GetScoutOverrideGate() *ScoutOverrideGate {
	return o.overrideGate
}

// QuestionsAllowedForTask returns the number of questions allowed for the current task.
// This considers the tier and any active override conditions.
func (o *Orchestrator) QuestionsAllowedForTask(taskID string) int {
	return QuestionsAllowed(o.tier, o.overrideGate, taskID)
}

// createSessionState creates a new session record in the state database.
func (o *Orchestrator) createSessionState(request string) error {
	if o.stateDB == nil {
		return nil // No-op if state DB not configured
	}

	session := &state.Session{
		ID:        o.sessionID,
		RootTask:  request,
		Tier:      string(o.tier),
		StartedAt: time.Now(),
		Status:    state.SessionActive,
	}

	return o.stateDB.CreateSession(session)
}

// updateSessionStatus updates the session status in the state database.
func (o *Orchestrator) updateSessionStatus(status state.SessionStatus) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	session, err := o.stateDB.GetSession(o.sessionID)
	if err != nil || session == nil {
		return
	}

	session.Status = status
	o.stateDB.UpdateSession(session)
}

// persistTasks creates task records in the state database.
func (o *Orchestrator) persistTasks(tasks []*models.Task) error {
	if o.stateDB == nil {
		return nil // No-op if state DB not configured
	}

	for _, t := range tasks {
		stateTask := &state.Task{
			ID:          t.ID,
			ParentID:    t.ParentID,
			Title:       t.Title,
			Description: t.Description,
			Status:      state.TaskStatus(t.Status),
			DependsOn:   t.DependsOn,
			Tier:        string(t.Tier),
			CreatedAt:   t.CreatedAt,
		}
		if err := o.stateDB.CreateTask(stateTask); err != nil {
			return fmt.Errorf("create task %s: %w", t.ID, err)
		}
	}

	return nil
}

// updateTaskState updates a task's status in the state database.
func (o *Orchestrator) updateTaskState(task *models.Task) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	stateTask := &state.Task{
		ID:          task.ID,
		ParentID:    task.ParentID,
		Title:       task.Title,
		Description: task.Description,
		Status:      state.TaskStatus(task.Status),
		DependsOn:   task.DependsOn,
		AssignedTo:  task.AssignedTo,
		Tier:        string(task.Tier),
		CreatedAt:   task.CreatedAt,
		CompletedAt: task.CompletedAt,
	}
	o.stateDB.UpdateTask(stateTask)
}

// createAgentState creates an agent record in the state database.
func (o *Orchestrator) createAgentState(a *models.Agent) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	stateAgent := &state.Agent{
		ID:           a.ID,
		TaskID:       a.TaskID,
		Status:       state.AgentStatus(a.Status),
		WorktreePath: a.WorktreePath,
		PID:          a.PID,
		StartedAt:    &a.StartedAt,
	}
	o.stateDB.CreateAgent(stateAgent)
}

// updateAgentState updates an agent's status in the state database.
func (o *Orchestrator) updateAgentState(agentID string, status state.AgentStatus) {
	if o.stateDB == nil {
		return // No-op if state DB not configured
	}

	agent, err := o.stateDB.GetAgent(agentID)
	if err != nil || agent == nil {
		return
	}

	agent.Status = status
	o.stateDB.UpdateAgent(agent)
}

// createProgEpicAndTasks creates a prog epic for the request and prog tasks for each subtask.
// This enables cross-session task tracking and continuity.
func (o *Orchestrator) createProgEpicAndTasks(request string, tasks []*models.Task) error {
	if o.progClient == nil {
		return nil // No-op if prog client not configured
	}

	// Truncate request for epic title (prog titles should be concise)
	epicTitle := request
	if len(epicTitle) > 100 {
		epicTitle = epicTitle[:97] + "..."
	}

	// Create the epic
	epicID, err := o.progClient.CreateEpic(epicTitle, &prog.EpicOptions{
		Description: request,
	})
	if err != nil {
		return fmt.Errorf("create epic: %w", err)
	}

	log.Printf("[orchestrator] created prog epic %s for request", epicID)

	// Store epic ID for later reference
	o.progEpicID = epicID

	// Map internal task IDs to prog task IDs for dependency resolution
	internalToProgID := make(map[string]string)

	// First pass: create all tasks (without dependencies)
	for _, task := range tasks {
		progTaskID, err := o.progClient.CreateTask(task.Title, &prog.TaskOptions{
			Description: task.Description,
			ParentID:    epicID,
		})
		if err != nil {
			return fmt.Errorf("create task %q: %w", task.Title, err)
		}
		internalToProgID[task.ID] = progTaskID
	}

	// Second pass: add dependencies
	for _, task := range tasks {
		if len(task.DependsOn) == 0 {
			continue
		}

		progTaskID := internalToProgID[task.ID]
		for _, depID := range task.DependsOn {
			progDepID, ok := internalToProgID[depID]
			if !ok {
				log.Printf("[orchestrator] warning: prog dependency %s not found for task %s", depID, task.ID)
				continue
			}
			if err := o.progClient.AddDependency(progTaskID, progDepID); err != nil {
				return fmt.Errorf("add dependency %s -> %s: %w", progTaskID, progDepID, err)
			}
		}
	}

	// Store the mapping for later status updates
	o.progTaskIDs = internalToProgID

	log.Printf("[orchestrator] created %d prog tasks under epic %s", len(tasks), epicID)
	return nil
}

// progTaskID returns the prog task ID for an internal task ID.
// Returns empty string if no mapping exists or prog is not configured.
func (o *Orchestrator) progTaskID(internalID string) string {
	if o.progTaskIDs == nil {
		return ""
	}
	return o.progTaskIDs[internalID]
}

// progStartTask marks a prog task as in_progress and logs the start event.
func (o *Orchestrator) progStartTask(internalID string) {
	if o.progClient == nil {
		return
	}
	progID := o.progTaskID(internalID)
	if progID == "" {
		return
	}

	if err := o.progClient.Start(progID); err != nil {
		log.Printf("[orchestrator] warning: failed to start prog task %s: %v", progID, err)
		return
	}
	if err := o.progClient.AddLog(progID, "Task execution started"); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task start %s: %v", progID, err)
	}
}

// progLogTask adds a log entry to a prog task.
func (o *Orchestrator) progLogTask(internalID, message string) {
	if o.progClient == nil {
		return
	}
	progID := o.progTaskID(internalID)
	if progID == "" {
		return
	}

	if err := o.progClient.AddLog(progID, message); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task %s: %v", progID, err)
	}
}

// progCompleteTask marks a prog task as done and logs the completion.
func (o *Orchestrator) progCompleteTask(internalID string) {
	if o.progClient == nil {
		return
	}
	progID := o.progTaskID(internalID)
	if progID == "" {
		return
	}

	if err := o.progClient.AddLog(progID, "Task completed successfully"); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task completion %s: %v", progID, err)
	}
	if err := o.progClient.Done(progID); err != nil {
		log.Printf("[orchestrator] warning: failed to complete prog task %s: %v", progID, err)
	}
}

// progBlockTask marks a prog task as blocked and logs the failure reason.
func (o *Orchestrator) progBlockTask(internalID, reason string) {
	if o.progClient == nil {
		return
	}
	progID := o.progTaskID(internalID)
	if progID == "" {
		return
	}

	if err := o.progClient.AddLog(progID, fmt.Sprintf("Task failed: %s", reason)); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task failure %s: %v", progID, err)
	}
	if err := o.progClient.Block(progID); err != nil {
		log.Printf("[orchestrator] warning: failed to block prog task %s: %v", progID, err)
	}
}

// captureLearningOnCompletion extracts learnings from successful task completion
// and stores them via prog for cross-session knowledge retention.
// It analyzes task output for patterns worth learning and creates durable learnings
// with concepts for categorization, linked to the task for evidence.
func (o *Orchestrator) captureLearningOnCompletion(task *models.Task, result *agent.ExecutionResult) {
	// Skip if prog client or learning system is not configured
	if o.progClient == nil {
		return
	}

	progID := o.progTaskID(task.ID)

	// Extract learnable patterns from the task output
	learningCandidate := o.extractLearningCandidate(task, result)
	if learningCandidate == nil {
		return
	}

	// Derive concepts from the task context
	concepts := o.deriveLearningConcepts(task)

	// Create learning via prog client for cross-session durability
	learningID, err := o.progClient.AddLearning(learningCandidate.Summary, &prog.LearningOptions{
		TaskID:   progID,
		Detail:   learningCandidate.Detail,
		Concepts: concepts,
	})
	if err != nil {
		log.Printf("[orchestrator] warning: failed to capture learning for task %s: %v", task.ID, err)
		return
	}

	log.Printf("[orchestrator] captured learning %s for task %s: %s", learningID, task.ID, learningCandidate.Summary)

	// Also log to task for traceability
	if progID != "" {
		_ = o.progClient.AddLog(progID, fmt.Sprintf("Captured learning: %s", learningCandidate.Summary))
	}
}

// learningCandidate holds extracted learning information from task completion.
type learningCandidate struct {
	Summary string // Brief description of what was learned
	Detail  string // Extended details about the learning
}

// extractLearningCandidate analyzes task completion for patterns worth learning.
// It looks for:
// - Tasks that completed faster than expected
// - Tasks that used novel approaches visible in output
// - Tasks involving complex problem-solving
// - Tasks with high token efficiency
// Returns nil if no learnable pattern is detected.
func (o *Orchestrator) extractLearningCandidate(task *models.Task, result *agent.ExecutionResult) *learningCandidate {
	// Skip tasks with minimal output (likely trivial)
	if len(result.Output) < 100 {
		return nil
	}

	// Look for patterns indicating valuable learnings:
	// 1. Tasks that involved debugging or error resolution
	// 2. Tasks that modified configuration or setup
	// 3. Tasks that implemented significant features
	// 4. Tasks with high efficiency (low tokens for substantial output)

	// Check for debug/fix patterns in output
	output := strings.ToLower(result.Output)
	isDebugTask := strings.Contains(output, "fixed") ||
		strings.Contains(output, "resolved") ||
		strings.Contains(output, "debugging") ||
		strings.Contains(output, "error was")

	// Check for setup/config patterns
	isConfigTask := strings.Contains(output, "configured") ||
		strings.Contains(output, "setup") ||
		strings.Contains(output, "initialized")

	// Check for implementation patterns
	isImplTask := strings.Contains(output, "implemented") ||
		strings.Contains(output, "created") ||
		strings.Contains(output, "added")

	// Check for efficiency - successful completion with reasonable token usage
	isEfficient := result.TokensUsed > 0 && result.TokensUsed < 50000 && len(result.Output) > 500

	// Build learning candidate based on detected patterns
	var summary, detail string

	switch {
	case isDebugTask:
		summary = fmt.Sprintf("WHEN debugging %s DO check for similar patterns RESULT faster resolution", task.Title)
		detail = fmt.Sprintf("Task successfully resolved an issue. Output patterns indicate debugging approach used.\n\nTask: %s\nTokens used: %d\nDuration: %s",
			task.Title, result.TokensUsed, result.Duration)
	case isConfigTask:
		summary = fmt.Sprintf("WHEN setting up %s DO follow established configuration RESULT consistent setup", task.Title)
		detail = fmt.Sprintf("Task completed configuration or setup work.\n\nTask: %s\nTokens used: %d\nDuration: %s",
			task.Title, result.TokensUsed, result.Duration)
	case isImplTask && isEfficient:
		summary = fmt.Sprintf("WHEN implementing features like %s DO use efficient patterns RESULT reduced token usage", task.Title)
		detail = fmt.Sprintf("Task implemented functionality efficiently.\n\nTask: %s\nTokens used: %d\nOutput length: %d chars\nDuration: %s",
			task.Title, result.TokensUsed, len(result.Output), result.Duration)
	default:
		// No significant pattern detected
		return nil
	}

	return &learningCandidate{
		Summary: summary,
		Detail:  detail,
	}
}

// deriveLearningConcepts extracts concept names from task context for categorization.
// Concepts help organize learnings for retrieval in similar contexts.
func (o *Orchestrator) deriveLearningConcepts(task *models.Task) []string {
	var concepts []string

	// Add tier as a concept
	if o.tier != "" {
		concepts = append(concepts, string(o.tier))
	}

	// Extract keywords from task title for concepts
	title := strings.ToLower(task.Title)
	description := strings.ToLower(task.Description)
	combined := title + " " + description

	// Common concept patterns
	conceptKeywords := map[string]string{
		"test":       "testing",
		"debug":      "debugging",
		"fix":        "bug-fix",
		"implement":  "implementation",
		"refactor":   "refactoring",
		"config":     "configuration",
		"setup":      "setup",
		"api":        "api",
		"database":   "database",
		"frontend":   "frontend",
		"backend":    "backend",
		"security":   "security",
		"performance":"performance",
		"doc":        "documentation",
	}

	for keyword, concept := range conceptKeywords {
		if strings.Contains(combined, keyword) {
			concepts = append(concepts, concept)
		}
	}

	// Limit concepts to avoid over-categorization
	if len(concepts) > 5 {
		concepts = concepts[:5]
	}

	return concepts
}

// GetProgEpicID returns the prog epic ID for cross-session tracking.
// Returns empty string if no epic is associated with this session.
func (o *Orchestrator) GetProgEpicID() string {
	return o.progEpicID
}
