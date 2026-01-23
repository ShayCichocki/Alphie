// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/decompose"
	iexec "github.com/ShayCichocki/alphie/internal/exec"
	"github.com/ShayCichocki/alphie/internal/git"
	"github.com/ShayCichocki/alphie/internal/graph"
	"github.com/ShayCichocki/alphie/internal/merge"
	"github.com/ShayCichocki/alphie/internal/orchestrator/policy"
	"github.com/ShayCichocki/alphie/internal/protect"
	"github.com/ShayCichocki/alphie/internal/structure"
)

// OrchestratorConfig contains configuration options for the Orchestrator.
type OrchestratorConfig struct {
	// RepoPath is the path to the git repository.
	RepoPath string
	// MaxAgents is the maximum number of concurrent agents.
	// If 0, defaults to 4.
	MaxAgents int
	// Policy contains configurable policy parameters.
	// If nil, policy.Default() is used.
	Policy *policy.Config
	// Greenfield indicates if this is a new project (no session branch needed).
	Greenfield bool
	// DecomposerClaude is the Claude runner for task decomposition.
	DecomposerClaude agent.ClaudeRunner
	// MergerClaude is the Claude runner for semantic merge operations.
	MergerClaude agent.ClaudeRunner
	// SecondReviewerClaude is the Claude runner for second review of high-risk diffs.
	// If nil, second review is disabled.
	SecondReviewerClaude agent.ClaudeRunner
	// ClaudeRunnerFactory creates new ClaudeRunner instances for dynamic operations.
	// Required for semantic merge factory.
	ClaudeRunnerFactory agent.ClaudeRunnerFactory
	// Executor is the agent executor for running tasks.
	Executor agent.TaskExecutor
	// Logger provides debug logging for orchestrator operations.
	// If nil, a default logger is created writing to .alphie/logs/orchestrator-debug.log
	Logger *DebugLogger
	// GitRunner provides git operations. If nil, git.NewRunner(RepoPath) is used.
	// Inject a custom implementation for testing.
	GitRunner git.Runner
	// ExecRunner provides command execution. If nil, exec.NewRunner() is used.
	// Inject a custom implementation for testing.
	ExecRunner iexec.CommandRunner
	// OriginalTaskID is the task ID from the TUI's task_entered event.
	// Used to link epic_created events back to the original task for deduplication.
	OriginalTaskID string

	// Verification options
	// EnablePostMergeVerification enables build verification after merge.
	// When enabled, the orchestrator runs the project's build command after each merge
	// and rolls back the merge if the build fails.
	// Use a pointer to distinguish between "not set" (nil = use default true) and "explicitly disabled" (false).
	EnablePostMergeVerification *bool
	// VerificationTimeout is the maximum time to wait for build verification.
	// Default: 2 minutes if not set or zero.
	VerificationTimeout time.Duration

	// Structure guidance options
	// EnableStructureGuidance enables directory structure guidance for agents.
	// When enabled, agents receive information about common directory patterns in prompts.
	// Use a pointer to distinguish between "not set" (nil = use default true) and "explicitly disabled" (false).
	EnableStructureGuidance *bool

	// Injectable dependencies (nil = use defaults)
	// Decomposer decomposes user requests into tasks. If nil, NewDecomposer is used.
	Decomposer *decompose.Decomposer
	// Graph is the dependency graph. If nil, NewDependencyGraph is used.
	Graph *graph.DependencyGraph
	// CollisionChecker detects file collisions. If nil, NewCollisionChecker is used.
	CollisionChecker *CollisionChecker
	// ProtectedAreaChecker detects protected areas. If nil, NewProtectedAreaDetector is used.
	ProtectedAreaChecker *protect.Detector
	// OverrideGate controls Scout question overrides. If nil, NewScoutOverrideGate is used.
	OverrideGate *ScoutOverrideGate
	// MergeStrategy defines how merge operations are configured.
	// If nil, automatically selected based on Greenfield flag.
	MergeStrategy *MergeStrategy
}

// Orchestrator coordinates the entire workflow from request to completion.
// It wires together: decomposer -> graph -> scheduler -> agents -> merger.
type Orchestrator struct {
	// config holds immutable runtime configuration.
	config *OrchestratorRunConfig

	// Core workflow components
	decomposer     *decompose.Decomposer
	graph          *graph.DependencyGraph
	scheduler      *Scheduler
	spawner        *DefaultAgentSpawner
	merger         *merge.Handler
	semanticMerger *SemanticMerger
	secondReviewer *SecondReviewer
	sessionMgr     *SessionBranchManager
	mergeQueue     *MergeQueue
	mergeVerifier  *MergeVerifier

	// Support components
	collision         *CollisionChecker
	protected         *protect.Detector
	overrideGate      *ScoutOverrideGate
	structureAnalyzer *structure.StructureAnalyzer

	// External dependencies
	runnerFactory agent.ClaudeRunnerFactory
	logger        *DebugLogger

	// Runtime state
	emitter   *EventEmitter
	stopCh    chan struct{}
	wg        sync.WaitGroup
	registry  *AgentRegistry
	pauseCtrl *PauseController

	// Merge conflict blocking state
	mergeConflictMu      sync.RWMutex
	hasMergeConflict     bool
	mergeConflictTask    string   // Task ID that triggered conflict
	mergeConflictFiles   []string // Files with conflicts
	mergeResolverRunning bool     // Is resolver agent active
}

// New creates an Orchestrator with the given required config and options.
// This is the preferred constructor using functional options pattern.
//
// Example:
//
//	orch := orchestrator.New(
//	    orchestrator.RequiredConfig{
//	        RepoPath: "/path/to/repo",
//	        Tier:     nil,
//	        Executor: executor,
//	    },
//	    orchestrator.WithGreenfield(true),
//	    orchestrator.WithMaxAgents(4),
//	    orchestrator.WithProgClient(progClient),
//	)
func New(req RequiredConfig, opts ...Option) *Orchestrator {
	// Apply options to defaults
	o := &orchestratorOptions{}
	for _, opt := range opts {
		opt(o)
	}

	// Convert to legacy config and delegate
	cfg := toOrchestratorConfig(req, o)
	return NewOrchestrator(cfg)
}

// NewOrchestrator creates a new Orchestrator with the given configuration.
// Prefer using New() with functional options for cleaner API.
func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator {
	sessionID := uuid.New().String()[:8]

	// Use provided policy or default
	policyConfig := cfg.Policy
	if policyConfig == nil {
		policyConfig = policy.Default()
	}
	_ = policyConfig.Validate() // Normalize values

	// Use injected dependencies or create defaults
	decomposer := cfg.Decomposer
	if decomposer == nil {
		decomposer = decompose.New(cfg.DecomposerClaude)
	}

	g := cfg.Graph
	if g == nil {
		g = graph.New()
	}

	collision := cfg.CollisionChecker
	if collision == nil {
		collision = NewCollisionChecker()
	}

	protected := cfg.ProtectedAreaChecker
	if protected == nil {
		protected = protect.New()
	}

	// Create scout override gate - use injected or create from protected area checker
	overrideGate := cfg.OverrideGate
	if overrideGate == nil {
		overridePolicy := &policy.Default().Override
		// TODO: TierConfigs removed - reinstate if needed
		// if cfg.TierConfigs != nil && cfg.TierConfigs.Scout != nil && cfg.TierConfigs.Scout.OverrideGates != nil {
		// 	og := cfg.TierConfigs.Scout.OverrideGates
		// 	overridePolicy = &policy.OverridePolicy{
		// 		BlockedAfterNAttempts: og.BlockedAfterNAttempts,
		// 		ProtectedAreaDetected: og.ProtectedAreaDetected,
		// 	}
		// }
		overrideGate = NewScoutOverrideGateWithPolicy(protected, overridePolicy, nil)
	}

	// Create git runner - use provided or create default
	gitRunner := cfg.GitRunner
	if gitRunner == nil {
		gitRunner = git.NewRunner(cfg.RepoPath)
	}

	// Create exec runner - use provided or create default
	execRunner := cfg.ExecRunner
	if execRunner == nil {
		execRunner = iexec.NewRunner()
	}

	// Session branch manager
	sessionMgr := NewSessionBranchManagerWithRunner(sessionID, cfg.RepoPath, cfg.Greenfield, gitRunner)

	// Create or use injected merge strategy
	mergeStrategy := cfg.MergeStrategy
	if mergeStrategy == nil {
		mergeStrategy = NewMergeStrategy(MergeStrategyConfig{
			RepoPath:             cfg.RepoPath,
			SessionBranch:        sessionMgr.GetBranchName(),
			GitRunner:            gitRunner,
			MergerClaude:         cfg.MergerClaude,
			SecondReviewerClaude: cfg.SecondReviewerClaude,
			Protected:            protected,
			Greenfield:           cfg.Greenfield,
		})
	}

	// Determine maxAgents from config
	maxAgents := cfg.MaxAgents
	// TODO: TierConfigs removed - reinstate if needed
	// if maxAgents <= 0 && cfg.TierConfigs != nil {
	// 	tierCfg := cfg.TierConfigs.Get(cfg.Tier)
	// 	if tierCfg != nil && tierCfg.MaxAgents > 0 {
	// 		maxAgents = tierCfg.MaxAgents
	// 	}
	// }
	if maxAgents <= 0 {
		maxAgents = 4 // Default to 4 concurrent agents
	}

	// Initialize logger - use provided one or create default
	logger := cfg.Logger
	if logger == nil {
		logger = NewDebugLoggerForRepo(cfg.RepoPath)
	}
	// Set package-level logger for internal components
	setPackageLogger(logger)

	// Create event emitter with large buffer to prevent event loss
	// Buffer size of 1000 supports ~10 concurrent tasks with ~100 events each
	emitter := NewEventEmitter(1000)

	// TODO: prog and learning coordinators removed - reinstate if needed
	// progCoord := NewProgCoordinator(cfg.ProgClient, emitter, cfg.OriginalTaskID, cfg.Tier, cfg.ResumeEpicID)
	// learningCoord := NewLearningCoordinator(progCoord, cfg.Tier)

	// Create agent spawner (scheduler will be set later in Run)
	spawner := NewAgentSpawner(cfg.Executor, collision, nil, emitter.Channel(), cfg.RepoPath)

	// Create merge components from strategy
	merger := mergeStrategy.CreateMerger()
	semanticMerger := mergeStrategy.CreateSemanticMerger()
	secondReviewer := mergeStrategy.CreateSecondReviewer()

	// Apply configuration defaults
	// Verification defaults to enabled unless explicitly disabled
	enableVerification := true
	if cfg.EnablePostMergeVerification != nil {
		enableVerification = *cfg.EnablePostMergeVerification
	}

	verificationTimeout := cfg.VerificationTimeout
	if verificationTimeout == 0 {
		verificationTimeout = 2 * time.Minute
	}

	// Structure guidance defaults to enabled unless explicitly disabled
	enableStructure := true
	if cfg.EnableStructureGuidance != nil {
		enableStructure = *cfg.EnableStructureGuidance
	}

	// Create merge verifier for post-merge build verification (if enabled)
	var mergeVerifier *MergeVerifier
	if enableVerification {
		projectInfo := GetProjectTypeInfo(cfg.RepoPath)
		mergeVerifier = NewMergeVerifier(cfg.RepoPath, projectInfo, verificationTimeout)
		logger.Log("[orchestrator] post-merge verification enabled (timeout: %v, project type: %s)", verificationTimeout, projectInfo.Type)
	} else {
		logger.Log("[orchestrator] post-merge verification disabled")
	}

	// Create structure analyzer for directory pattern guidance (if enabled)
	var structureAnalyzer *structure.StructureAnalyzer
	if enableStructure {
		structureAnalyzer = structure.NewAnalyzer(cfg.RepoPath)
		if err := structureAnalyzer.AnalyzeRepository(); err != nil {
			logger.Log("[orchestrator] warning: structure analysis failed: %v", err)
			// Don't fail - structure guidance is optional
		} else {
			rules := structureAnalyzer.GetRules()
			if rules != nil {
				logger.Log("[orchestrator] structure guidance enabled (%d patterns detected)", len(rules.Rules))
			}
		}
	} else {
		logger.Log("[orchestrator] structure guidance disabled")
	}

	// Create immutable runtime config
	runConfig := &OrchestratorRunConfig{
		SessionID:      sessionID,
		RepoPath:       cfg.RepoPath,
		Tier:           nil, // TODO: tier removed - always use default
		MaxAgents:      maxAgents,
		Greenfield:     cfg.Greenfield,
		OriginalTaskID: cfg.OriginalTaskID,
		Policy:         policyConfig,
		// Baseline is set later in Run() after capture
	}

	o := &Orchestrator{
		config:            runConfig,
		decomposer:        decomposer,
		graph:             g,
		scheduler:         nil, // Created in Run after graph is built
		spawner:           spawner,
		merger:            merger,
		semanticMerger:    semanticMerger,
		secondReviewer:    secondReviewer,
		sessionMgr:        sessionMgr,
		mergeQueue:        nil, // Created in Run
		mergeVerifier:     mergeVerifier,
		collision:         collision,
		protected:         protected,
		overrideGate:      overrideGate,
		structureAnalyzer: structureAnalyzer,
		runnerFactory:     cfg.ClaudeRunnerFactory,
		logger:            logger,
		emitter:           emitter,
		stopCh:            make(chan struct{}),
		registry:          NewAgentRegistry(),
		pauseCtrl:         NewPauseController(),
	}

	// TODO: learning system removed - reinstate if needed
	// Initialize effectiveness tracker if learning system is available
	// if ls, ok := cfg.LearningSystem.(*learning.LearningSystem); ok {
	// 	o.effectivenessTracker = learning.NewEffectivenessTracker(ls.GetStore())
	// }

	return o
}

// Events returns a read-only channel of orchestrator events.
// This is used by the TUI to receive updates.
func (o *Orchestrator) Events() <-chan OrchestratorEvent {
	return o.emitter.Events()
}

// DroppedEventCount returns the number of events dropped due to full channel.
func (o *Orchestrator) DroppedEventCount() uint64 {
	return o.emitter.DroppedCount()
}

// emitEvent sends an event to the events channel.
func (o *Orchestrator) emitEvent(event OrchestratorEvent) {
	o.emitter.Emit(event)
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
// TODO: prog package removed - reinstate if needed
func (o *Orchestrator) GetProgClient() interface{} {
	return nil
}

// QuestionsAllowedForTask returns the number of questions allowed for the current task.
// This considers the tier and any active override conditions.
func (o *Orchestrator) QuestionsAllowedForTask(taskID string) int {
	return QuestionsAllowed(o.config.Tier, o.overrideGate, taskID)
}

// GetProgEpicID returns the prog epic ID for cross-session tracking.
// Returns empty string if no epic is associated with this session.
// TODO: prog package removed - reinstate if needed
func (o *Orchestrator) GetProgEpicID() string {
	return ""
}

// SetMergeConflict marks the orchestrator as having an active merge conflict.
// This blocks all task scheduling until the conflict is resolved.
func (o *Orchestrator) SetMergeConflict(taskID string, files []string) {
	o.mergeConflictMu.Lock()
	defer o.mergeConflictMu.Unlock()

	o.hasMergeConflict = true
	o.mergeConflictTask = taskID
	o.mergeConflictFiles = files

	o.logger.Log("MERGE_CONFLICT", "Blocking all scheduling - conflict in task %s (%d files)", taskID, len(files))
}

// HasMergeConflict returns true if there is an active merge conflict blocking scheduling.
func (o *Orchestrator) HasMergeConflict() bool {
	o.mergeConflictMu.RLock()
	defer o.mergeConflictMu.RUnlock()
	return o.hasMergeConflict
}

// ClearMergeConflict clears the merge conflict flag and resumes scheduling.
func (o *Orchestrator) ClearMergeConflict() {
	o.mergeConflictMu.Lock()
	defer o.mergeConflictMu.Unlock()

	if !o.hasMergeConflict {
		return
	}

	o.logger.Log("MERGE_RESOLVED", "Clearing merge conflict flag - resuming scheduling")
	o.hasMergeConflict = false
	o.mergeConflictTask = ""
	o.mergeConflictFiles = nil
	o.mergeResolverRunning = false

	// Trigger scheduling to resume work
	select {
	case o.scheduler.trigger <- struct{}{}:
	default:
	}
}
