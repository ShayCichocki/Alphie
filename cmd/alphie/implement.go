package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/architect"
	"github.com/ShayCichocki/alphie/internal/finalverify"
	"github.com/ShayCichocki/alphie/internal/orchestrator"
	"github.com/ShayCichocki/alphie/internal/tui"
	"github.com/ShayCichocki/alphie/internal/validation"
	"github.com/ShayCichocki/alphie/internal/verification"
	"github.com/spf13/cobra"
)

var (
	implementUseCLI bool
)

var implementCmd = &cobra.Command{
	Use:   "implement <spec.md|spec.xml>",
	Short: "Implement architecture specification to completion",
	Long: `Implement an architecture specification by decomposing into tasks,
orchestrating parallel agents through the DAG, and validating rigorously
at each step. Iterates until all features are 100% complete.

Core principle: Build it right, no matter how long it takes.

Process:
  1. Parse spec → Extract features and requirements
  2. Decompose spec → AI generates DAG of tasks
  3. Orchestrate → Execute tasks in parallel with validation
  4. Final verification → Audit + Build/Test + Semantic review
  5. Repeat if gaps found → Identify missing pieces and retry

Validation per task (all must pass):
  - Verification contracts (test commands)
  - Build + test suite passes
  - Semantic validation (Claude reviews intent)
  - Code review against acceptance criteria

Final verification (all must pass):
  - Architecture audit: 100% features COMPLETE
  - Build + full test suite passes
  - Comprehensive semantic review of entire implementation

Merge conflict handling:
  - When conflicts occur, orchestrator pauses
  - Opus merge agent spawns with full context
  - Conflicts resolved intelligently
  - Orchestration resumes

Escalation on persistent failures:
  - Tasks that fail 3x trigger user escalation
  - Options: Retry / Skip / Abort / Manual Fix
  - Orchestrator pauses until user decides

Examples:
  alphie implement docs/spec.md              # Implement spec
  alphie implement spec.xml --cli            # Use CLI subprocess
  alphie implement spec.md --greenfield      # Merge directly to main`,
	Args: cobra.ExactArgs(1),
	RunE: runImplement,
}

func init() {
	implementCmd.Flags().BoolVar(&implementUseCLI, "cli", false, "Use Claude CLI subprocess instead of API")
}

// implementConfig contains configuration extracted from global config and flags.
type implementConfig struct {
	specPath      string
	repoPath      string
	specName      string
	useCLI        bool
	model         string
	maxAgents     int
	runnerFactory agent.ClaudeRunnerFactory
}

func runImplement(cmd *cobra.Command, args []string) error {
	specPath := args[0]

	// Verify spec exists
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		return fmt.Errorf("spec not found: %s", specPath)
	}

	// Get current working directory as repo path
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Extract spec name for branch naming
	specName := extractSpecName(specPath)

	// Check for Claude CLI if needed
	if err := CheckClaudeCLI(); err != nil {
		return err
	}

	// Create runner factory (CLI subprocess or API)
	runnerFactory, err := createRunnerFactory(implementUseCLI)
	if err != nil {
		return fmt.Errorf("create runner factory: %w", err)
	}

	// Build config
	cfg := implementConfig{
		specPath:      specPath,
		repoPath:      repoPath,
		specName:      specName,
		useCLI:        implementUseCLI,
		model:         "sonnet",    // Default model
		maxAgents:     3,            // Default concurrent agents
		runnerFactory: runnerFactory,
	}

	// Display configuration
	fmt.Println("=== Alphie Implement ===")
	fmt.Println()
	fmt.Printf("Spec:         %s\n", specPath)
	fmt.Printf("Repository:   %s\n", repoPath)
	fmt.Printf("Model:        %s\n", cfg.model)
	fmt.Printf("Max agents:   %d\n", cfg.maxAgents)
	fmt.Printf("Backend:      %s\n", backendName(cfg.useCLI))
	fmt.Println()

	// Create TUI program for progress visualization
	tuiProgram, tuiApp := tui.NewImplementProgram()

	// Create escalation handler that will be wired to the orchestrator
	// The orchestrator reference will be updated on each iteration
	var currentOrch *orchestrator.Orchestrator
	var orchMu sync.RWMutex

	tuiApp.SetEscalationHandler(func(action string) error {
		orchMu.RLock()
		orch := currentOrch
		orchMu.RUnlock()

		if orch == nil {
			return fmt.Errorf("no orchestrator available")
		}

		response := &orchestrator.EscalationResponse{
			Action:    orchestrator.EscalationAction(action),
			Timestamp: time.Now(),
		}
		return orch.RespondToEscalation(response)
	})

	// Create context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run implementation loop in background
	go func() {
		err := runImplementationLoop(ctx, cfg, tuiProgram, &currentOrch, &orchMu)
		tuiProgram.Send(tui.ImplementDoneMsg{Err: err})
	}()

	// Run TUI (blocks until quit or completion)
	if _, err := tuiProgram.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

// runImplementationLoop executes the main implementation loop.
// This is the core orchestration logic that:
// 1. Parses spec
// 2. Decomposes to DAG
// 3. Orchestrates through tasks
// 4. Runs final verification
// 5. Repeats if gaps found
func runImplementationLoop(ctx context.Context, cfg implementConfig, tuiProgram interface{ Send(tea.Msg) }, currentOrch **orchestrator.Orchestrator, orchMu *sync.RWMutex) error {
	// Progress callback for TUI updates
	progressCallback := func(event architect.ProgressEvent) {
		phaseStr := string(event.Phase)

		// Convert architect.WorkerInfo to tui.WorkerInfo
		activeWorkers := make(map[string]tui.WorkerInfo)
		for k, v := range event.ActiveWorkers {
			activeWorkers[k] = tui.WorkerInfo{
				AgentID:   v.AgentID,
				TaskID:    v.TaskID,
				TaskTitle: v.TaskTitle,
				Status:    v.Status,
			}
		}

		tuiProgram.Send(tui.ImplementUpdateMsg{
			State: tui.ImplementState{
				Iteration:        event.Iteration,
				FeaturesComplete: event.FeaturesComplete,
				FeaturesTotal:    event.FeaturesTotal,
				Cost:             event.Cost,
				CurrentPhase:     phaseStr,
				WorkersRunning:   event.WorkersRunning,
				WorkersBlocked:   event.WorkersBlocked,
				ActiveWorkers:    activeWorkers,
			},
		})

		// Send log entry
		tuiProgram.Send(tui.ImplementLogMsg{
			Timestamp: event.Timestamp,
			Phase:     phaseStr,
			Message:   event.Message,
		})
	}

	// Main iteration loop
	iteration := 1
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 1. Parse spec
		progressCallback(architect.ProgressEvent{
			Phase:     architect.PhaseParsing,
			Iteration: iteration,
			Message:   fmt.Sprintf("Iteration %d: Parsing spec...", iteration),
		})

		parser := architect.NewParser()
		parseRunner := cfg.runnerFactory.NewRunner()
		spec, err := parser.Parse(ctx, cfg.specPath, parseRunner)
		if err != nil {
			return fmt.Errorf("parse spec (iteration %d): %w", iteration, err)
		}

		totalFeatures := len(spec.Features)

		// 2. Audit codebase
		progressCallback(architect.ProgressEvent{
			Phase:         architect.PhaseAuditing,
			Iteration:     iteration,
			FeaturesTotal: totalFeatures,
			Message:       fmt.Sprintf("Iteration %d: Auditing %d features...", iteration, totalFeatures),
		})

		auditor := architect.NewAuditor()
		auditRunner := cfg.runnerFactory.NewRunner()
		gapReport, err := auditor.Audit(ctx, spec, cfg.repoPath, auditRunner)
		if err != nil {
			return fmt.Errorf("audit codebase (iteration %d): %w", iteration, err)
		}

		// Calculate completion
		completedFeatures := 0
		for _, fs := range gapReport.Features {
			if fs.Status == architect.AuditStatusComplete {
				completedFeatures++
			}
		}
		gapsFound := len(gapReport.Gaps)

		progressCallback(architect.ProgressEvent{
			Phase:            architect.PhaseAuditing,
			Iteration:        iteration,
			FeaturesComplete: completedFeatures,
			FeaturesTotal:    totalFeatures,
			GapsFound:        gapsFound,
			Message:          fmt.Sprintf("Audit complete: %d/%d features, %d gaps", completedFeatures, totalFeatures, gapsFound),
		})

		// 3. Check if we're done (no gaps)
		if gapsFound == 0 {
			// Run final verification before declaring success
			progressCallback(architect.ProgressEvent{
				Phase:            architect.PhaseComplete,
				Iteration:        iteration,
				FeaturesComplete: totalFeatures,
				FeaturesTotal:    totalFeatures,
				Message:          "Running final verification...",
			})

			// Read spec text for final verification
			specTextBytes, err := os.ReadFile(cfg.specPath)
			if err != nil {
				return fmt.Errorf("read spec file for verification: %w", err)
			}
			specText := string(specTextBytes)

			verifyResult, verifyErr := runFinalVerification(ctx, cfg, spec, specText)
			if verifyErr != nil {
				return fmt.Errorf("final verification error: %w", verifyErr)
			}

			if verifyResult.AllPassed {
				// Success! All features complete and verified
				progressCallback(architect.ProgressEvent{
					Phase:            architect.PhaseComplete,
					Iteration:        iteration,
					FeaturesComplete: totalFeatures,
					FeaturesTotal:    totalFeatures,
					Message:          fmt.Sprintf("✓ Implementation complete! %s", verifyResult.Summary),
				})
				return nil
			}

			// Final verification failed - must have missed something
			// Continue to next iteration to identify what's missing
			progressCallback(architect.ProgressEvent{
				Phase:            architect.PhaseAuditing,
				Iteration:        iteration,
				FeaturesComplete: completedFeatures,
				FeaturesTotal:    totalFeatures,
				Message:          fmt.Sprintf("Final verification failed: %s", verifyResult.FailureReason),
			})
			iteration++
			continue
		}

		// 4. Orchestrate gap resolution
		progressCallback(architect.ProgressEvent{
			Phase:            architect.PhaseExecuting,
			Iteration:        iteration,
			FeaturesComplete: completedFeatures,
			FeaturesTotal:    totalFeatures,
			GapsFound:        gapsFound,
			Message:          fmt.Sprintf("Orchestrating %d gaps with %d agents...", gapsFound, cfg.maxAgents),
		})

		// Create orchestrator for this iteration
		orch, err := createOrchestrator(cfg, progressCallback)
		if err != nil {
			return fmt.Errorf("create orchestrator: %w", err)
		}

		// Update current orchestrator reference for escalation handler
		orchMu.Lock()
		*currentOrch = orch
		orchMu.Unlock()

		// Subscribe to orchestrator events for escalation handling
		eventsCh := orch.Events()
		eventsDone := make(chan struct{})
		go func() {
			defer close(eventsDone)
			for event := range eventsCh {
				// Route escalation events to TUI
				if event.Type == orchestrator.EventTaskEscalation {
					attempts := 0
					if event.Metadata != nil {
						if v, ok := event.Metadata["attempts"].(int); ok {
							attempts = v
						}
					}

					tuiProgram.Send(tui.EscalationMsg{
						TaskID:    event.TaskID,
						TaskTitle: event.TaskTitle,
						Reason:    event.Message,
						Attempts:  attempts,
						LogFile:   event.LogFile,
					})
				}
			}
		}()

		// Convert gaps to request string for decomposition
		request := buildRequestFromGaps(gapReport.Gaps, spec)

		// Run orchestrator with gap resolution request
		if err := orch.Run(ctx, request); err != nil {
			return fmt.Errorf("orchestrator execution (iteration %d): %w", iteration, err)
		}

		// Stop orchestrator
		orch.Stop()

		// Wait for event processing to complete
		<-eventsDone

		// Emit iteration complete
		progressCallback(architect.ProgressEvent{
			Phase:            architect.PhaseComplete,
			Iteration:        iteration,
			FeaturesComplete: completedFeatures,
			FeaturesTotal:    totalFeatures,
			Message:          fmt.Sprintf("Iteration %d complete - verifying progress...", iteration),
		})

		// Next iteration
		iteration++
	}
}

// mergeVerifierAdapter adapts orchestrator.MergeVerifier to finalverify.BuildTester interface.
type mergeVerifierAdapter struct {
	verifier *orchestrator.MergeVerifier
}

func (a *mergeVerifierAdapter) RunBuildAndTests(ctx context.Context, repoPath string) (bool, string, error) {
	result, err := a.verifier.VerifyMerge(ctx, "current")
	if err != nil {
		return false, "", err
	}
	return result.Passed, result.Output, nil
}

// runFinalVerification runs the 3-layer final verification using the finalverify package:
// 1. Architecture audit (must be 100% COMPLETE)
// 2. Build + test suite (must pass)
// 3. Comprehensive semantic review (must pass)
func runFinalVerification(ctx context.Context, cfg implementConfig, spec *architect.ArchSpec, specText string) (*finalverify.VerificationResult, error) {
	// Create auditor
	auditor := architect.NewAuditor()

	// Create build tester adapter
	projectInfo := orchestrator.GetProjectTypeInfo(cfg.repoPath)
	mergeVerifier := orchestrator.NewMergeVerifier(cfg.repoPath, projectInfo, 5*time.Minute)
	buildTester := &mergeVerifierAdapter{verifier: mergeVerifier}

	// Create final verifier
	verifier := finalverify.NewFinalVerifier(auditor, buildTester, cfg.runnerFactory)

	// Run verification
	result, err := verifier.Verify(ctx, finalverify.VerificationInput{
		RepoPath: cfg.repoPath,
		Spec:     spec,
		SpecText: specText,
	})

	return result, err
}

// createOrchestrator creates an orchestrator instance for the current iteration.
func createOrchestrator(cfg implementConfig, progressCallback architect.ProgressCallback) (*orchestrator.Orchestrator, error) {
	// Create 4-layer validator for task validation
	validator := createValidator(cfg.repoPath, cfg.runnerFactory)

	// Create executor
	executor, err := agent.NewExecutor(agent.ExecutorConfig{
		RepoPath:      cfg.repoPath,
		Model:         cfg.model,
		RunnerFactory: cfg.runnerFactory,
		Validator:     validator,
	})
	if err != nil {
		return nil, fmt.Errorf("create executor: %w", err)
	}

	// Create Claude runners for decomposer and merger
	decomposerClaude := cfg.runnerFactory.NewRunner()
	mergerClaude := cfg.runnerFactory.NewRunner()
	secondReviewerClaude := cfg.runnerFactory.NewRunner()

	// Determine if greenfield mode from global flag
	greenfieldMode := greenfieldEnabled

	// Create orchestrator with simplified config
	orch := orchestrator.New(
		orchestrator.RequiredConfig{
			RepoPath: cfg.repoPath,
			Executor: executor,
		},
		orchestrator.WithMaxAgents(cfg.maxAgents),
		orchestrator.WithGreenfield(greenfieldMode),
		orchestrator.WithSpecName(cfg.specName), // Branch naming: alphie-{spec-name}-{timestamp}
		orchestrator.WithDecomposerClaude(decomposerClaude),
		orchestrator.WithMergerClaude(mergerClaude),
		orchestrator.WithSecondReviewerClaude(secondReviewerClaude),
		orchestrator.WithRunnerFactory(cfg.runnerFactory),
	)

	return orch, nil
}

// createValidator creates a 4-layer validator for task validation.
func createValidator(repoPath string, runnerFactory agent.ClaudeRunnerFactory) agent.TaskValidator {
	// Create contract verifier (Layer 1)
	contractVerifier := verification.NewContractRunner(repoPath)

	// Create build tester with auto-detection (Layer 2)
	buildTester, err := validation.NewAutoBuildTester(repoPath, 5*time.Minute)
	if err != nil {
		// If build tester creation fails, use nil (validation will skip build tests)
		fmt.Fprintf(os.Stderr, "Warning: Failed to create build tester: %v\n", err)
		buildTester = nil
	}

	// Create validator with all 4 layers
	validator := validation.NewValidator(contractVerifier, buildTester, runnerFactory)

	// Wrap in adapter to implement agent.TaskValidator interface
	return validation.NewValidatorAdapter(validator)
}

// buildRequestFromGaps converts gap report into a request string for orchestrator.
func buildRequestFromGaps(gaps []architect.Gap, spec *architect.ArchSpec) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Implement the following gaps from the %s specification:\n\n", spec.Name))

	for i, gap := range gaps {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, gap.Description))
		if gap.FeatureID != "" {
			sb.WriteString(fmt.Sprintf("   Feature ID: %s\n", gap.FeatureID))
		}
		if gap.Status != "" {
			sb.WriteString(fmt.Sprintf("   Current status: %s\n", gap.Status))
		}
		if gap.SuggestedAction != "" {
			sb.WriteString(fmt.Sprintf("   Suggested action: %s\n", gap.SuggestedAction))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// extractSpecName extracts a clean name from the spec file path for branch naming.
func extractSpecName(specPath string) string {
	// Get base filename without extension
	base := filepath.Base(specPath)
	name := strings.TrimSuffix(base, filepath.Ext(base))

	// Clean up: lowercase, replace spaces/special chars with hyphens
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, name)

	// Remove consecutive hyphens and trim
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")

	// Truncate if too long
	if len(name) > 50 {
		name = name[:50]
	}

	return name
}

// backendName returns a human-readable backend name.
func backendName(useCLI bool) string {
	if useCLI {
		return "Claude CLI subprocess"
	}
	return "Anthropic API"
}
