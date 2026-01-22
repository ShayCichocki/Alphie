// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/config"
	"github.com/ShayCichocki/alphie/internal/decompose"
	iexec "github.com/ShayCichocki/alphie/internal/exec"
	"github.com/ShayCichocki/alphie/internal/git"
	"github.com/ShayCichocki/alphie/internal/graph"
	"github.com/ShayCichocki/alphie/internal/learning"
	"github.com/ShayCichocki/alphie/internal/orchestrator/policy"
	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/internal/protect"
	"github.com/ShayCichocki/alphie/internal/state"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// RequiredConfig contains the minimal required configuration for an Orchestrator.
// All fields are required and have no defaults.
type RequiredConfig struct {
	// RepoPath is the path to the git repository.
	RepoPath string
	// Tier is the agent tier for task execution.
	Tier models.Tier
	// Executor is the agent executor for running tasks.
	Executor agent.TaskExecutor
}

// Option configures an Orchestrator. Use With* functions to create Options.
type Option func(*orchestratorOptions)

// orchestratorOptions holds all optional configuration.
// These mirror OrchestratorConfig but are only used during construction.
type orchestratorOptions struct {
	maxAgents            int
	tierConfigs          *config.TierConfigs
	policyConfig         *policy.Config
	greenfield           bool
	decomposerClaude     agent.ClaudeRunner
	mergerClaude         agent.ClaudeRunner
	secondReviewerClaude agent.ClaudeRunner
	runnerFactory        agent.ClaudeRunnerFactory
	stateDB              state.StateStore
	learningSystem       learning.LearningProvider
	progClient           prog.ProgTracker
	logger               *DebugLogger
	gitRunner            git.Runner
	execRunner           iexec.CommandRunner
	resumeEpicID         string
	originalTaskID       string

	// Injectable dependencies for testing
	decomposer           *decompose.Decomposer
	graph                *graph.DependencyGraph
	collisionChecker     *CollisionChecker
	protectedAreaChecker *protect.Detector
	overrideGate         *ScoutOverrideGate
	mergeStrategy        *MergeStrategy
}

// WithMaxAgents sets the maximum number of concurrent agents.
func WithMaxAgents(n int) Option {
	return func(o *orchestratorOptions) { o.maxAgents = n }
}

// WithTierConfigs sets the tier configurations.
func WithTierConfigs(tc *config.TierConfigs) Option {
	return func(o *orchestratorOptions) { o.tierConfigs = tc }
}

// WithPolicy sets the policy configuration.
func WithPolicy(p *policy.Config) Option {
	return func(o *orchestratorOptions) { o.policyConfig = p }
}

// WithGreenfield sets greenfield mode (new project, no session branch).
func WithGreenfield(b bool) Option {
	return func(o *orchestratorOptions) { o.greenfield = b }
}

// WithDecomposerClaude sets the Claude runner for task decomposition.
func WithDecomposerClaude(r agent.ClaudeRunner) Option {
	return func(o *orchestratorOptions) { o.decomposerClaude = r }
}

// WithMergerClaude sets the Claude runner for semantic merge operations.
func WithMergerClaude(r agent.ClaudeRunner) Option {
	return func(o *orchestratorOptions) { o.mergerClaude = r }
}

// WithSecondReviewerClaude sets the Claude runner for second review.
func WithSecondReviewerClaude(r agent.ClaudeRunner) Option {
	return func(o *orchestratorOptions) { o.secondReviewerClaude = r }
}

// WithRunnerFactory sets the factory for creating ClaudeRunner instances.
func WithRunnerFactory(f agent.ClaudeRunnerFactory) Option {
	return func(o *orchestratorOptions) { o.runnerFactory = f }
}

// WithStateDB sets the state database.
func WithStateDB(db state.StateStore) Option {
	return func(o *orchestratorOptions) { o.stateDB = db }
}

// WithLearningSystem sets the learning system.
func WithLearningSystem(ls learning.LearningProvider) Option {
	return func(o *orchestratorOptions) { o.learningSystem = ls }
}

// WithProgClient sets the prog client for cross-session task management.
func WithProgClient(pc prog.ProgTracker) Option {
	return func(o *orchestratorOptions) { o.progClient = pc }
}

// WithLogger sets the debug logger.
func WithLogger(l *DebugLogger) Option {
	return func(o *orchestratorOptions) { o.logger = l }
}

// WithGitRunner sets the git runner.
func WithGitRunner(r git.Runner) Option {
	return func(o *orchestratorOptions) { o.gitRunner = r }
}

// WithExecRunner sets the command execution runner.
func WithExecRunner(r iexec.CommandRunner) Option {
	return func(o *orchestratorOptions) { o.execRunner = r }
}

// WithResumeEpicID sets the epic ID to resume.
func WithResumeEpicID(id string) Option {
	return func(o *orchestratorOptions) { o.resumeEpicID = id }
}

// WithOriginalTaskID sets the original task ID for event linking.
func WithOriginalTaskID(id string) Option {
	return func(o *orchestratorOptions) { o.originalTaskID = id }
}

// WithDecomposer sets a custom task decomposer (mainly for testing).
func WithDecomposer(d *decompose.Decomposer) Option {
	return func(o *orchestratorOptions) { o.decomposer = d }
}

// WithGraph sets a custom dependency graph (mainly for testing).
func WithGraph(g *graph.DependencyGraph) Option {
	return func(o *orchestratorOptions) { o.graph = g }
}

// WithCollisionChecker sets a custom collision checker (mainly for testing).
func WithCollisionChecker(c *CollisionChecker) Option {
	return func(o *orchestratorOptions) { o.collisionChecker = c }
}

// WithProtectedAreaChecker sets a custom protected area checker (mainly for testing).
func WithProtectedAreaChecker(p *protect.Detector) Option {
	return func(o *orchestratorOptions) { o.protectedAreaChecker = p }
}

// WithOverrideGate sets a custom override gate (mainly for testing).
func WithOverrideGate(g *ScoutOverrideGate) Option {
	return func(o *orchestratorOptions) { o.overrideGate = g }
}

// WithMergeStrategy sets a custom merge strategy.
func WithMergeStrategy(s *MergeStrategy) Option {
	return func(o *orchestratorOptions) { o.mergeStrategy = s }
}

// toOrchestratorConfig converts RequiredConfig + Options to the internal OrchestratorConfig.
// This bridges the new API to the existing implementation.
func toOrchestratorConfig(req RequiredConfig, opts *orchestratorOptions) OrchestratorConfig {
	return OrchestratorConfig{
		RepoPath:             req.RepoPath,
		Tier:                 req.Tier,
		Executor:             req.Executor,
		MaxAgents:            opts.maxAgents,
		TierConfigs:          opts.tierConfigs,
		Policy:               opts.policyConfig,
		Greenfield:           opts.greenfield,
		DecomposerClaude:     opts.decomposerClaude,
		MergerClaude:         opts.mergerClaude,
		SecondReviewerClaude: opts.secondReviewerClaude,
		ClaudeRunnerFactory:  opts.runnerFactory,
		StateDB:              opts.stateDB,
		LearningSystem:       opts.learningSystem,
		ProgClient:           opts.progClient,
		Logger:               opts.logger,
		GitRunner:            opts.gitRunner,
		ExecRunner:           opts.execRunner,
		ResumeEpicID:         opts.resumeEpicID,
		OriginalTaskID:       opts.originalTaskID,
		Decomposer:           opts.decomposer,
		Graph:                opts.graph,
		CollisionChecker:     opts.collisionChecker,
		ProtectedAreaChecker: opts.protectedAreaChecker,
		OverrideGate:         opts.overrideGate,
		MergeStrategy:        opts.mergeStrategy,
	}
}
