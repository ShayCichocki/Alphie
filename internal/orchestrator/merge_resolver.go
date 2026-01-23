package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/git"
)

const (
	// MaxMergeResolverRetries is the maximum number of retries for merge resolution.
	// Merge resolution gets more retries than regular tasks due to complexity.
	MaxMergeResolverRetries = 5
)

// MergeResolverAgent is a dedicated agent for resolving merge conflicts.
// It uses the Opus model (highest intelligence) with extended iteration budget.
type MergeResolverAgent struct {
	claudeFactory agent.ClaudeRunnerFactory
	git           git.Runner
	repoPath      string
	orchestrator  *Orchestrator
	contextBuilder *MergeContextBuilder
}

// NewMergeResolverAgent creates a new merge resolver agent.
func NewMergeResolverAgent(
	claudeFactory agent.ClaudeRunnerFactory,
	git git.Runner,
	repoPath string,
	orchestrator *Orchestrator,
) *MergeResolverAgent {
	// Create context builder for comprehensive merge context
	var contextBuilder *MergeContextBuilder
	if orchestrator != nil && orchestrator.graph != nil {
		contextBuilder = NewMergeContextBuilder(repoPath, git, orchestrator.graph)
	}

	return &MergeResolverAgent{
		claudeFactory:  claudeFactory,
		git:            git,
		repoPath:       repoPath,
		orchestrator:   orchestrator,
		contextBuilder: contextBuilder,
	}
}

// Resolve spawns a dedicated Claude agent to resolve merge conflicts.
// Uses Opus model (highest intelligence) with comprehensive context and extended retries.
func (mr *MergeResolverAgent) Resolve(ctx context.Context, req *MergeRequest, conflictFiles []string) error {
	targetBranch := mr.getTargetBranch()

	// Build comprehensive merge context if available
	var mergeContext *MergeContext
	if mr.contextBuilder != nil {
		var err error
		mergeContext, err = mr.contextBuilder.Build(ctx, targetBranch, req, conflictFiles)
		if err != nil {
			// Non-fatal - continue with basic prompt
			if mr.orchestrator != nil && mr.orchestrator.logger != nil {
				mr.orchestrator.logger.Log("merge_resolver", "Failed to build comprehensive context: %v", err)
			}
		}
	}

	// Build merge resolution prompt
	prompt := mr.buildMergePrompt(targetBranch, req.AgentBranch, conflictFiles, req.TaskID, mergeContext)

	// Create fresh Claude runner for merge resolution
	claude := mr.claudeFactory.NewRunner()

	// Start Claude with Opus model and extended timeout
	// Opus is the most capable model for complex merge resolution
	opts := &agent.StartOptions{
		Model: agent.ModelOpus, // Use highest intelligence model
	}

	if err := claude.StartWithOptions(prompt, mr.repoPath, opts); err != nil {
		return fmt.Errorf("start merge resolver: %w", err)
	}

	// Wait for resolution (with timeout)
	// Note: We can't pass context to Wait() - it will complete when Claude finishes
	// The ctx timeout is handled at a higher level
	err := claude.Wait()
	if err != nil {
		return fmt.Errorf("merge resolver failed: %w", err)
	}

	// Validate that conflicts are actually resolved
	if err := mr.validateResolution(conflictFiles); err != nil {
		return fmt.Errorf("merge resolution validation failed: %w", err)
	}

	// Ensure no conflict markers remain in resolved files
	if err := mr.validateNoConflictMarkers(conflictFiles); err != nil {
		return fmt.Errorf("conflict markers still present: %w", err)
	}

	// Clear merge conflict flag - resume scheduling
	if mr.orchestrator != nil {
		mr.orchestrator.ClearMergeConflict()
	}

	return nil
}

func (mr *MergeResolverAgent) buildMergePrompt(targetBranch, agentBranch string, conflicts []string, taskID string, mergeContext *MergeContext) string {
	var sb strings.Builder

	sb.WriteString("# URGENT: Merge Conflict Resolution Required\n\n")
	sb.WriteString("You are a dedicated merge conflict resolver using the Opus model (highest intelligence). ")
	sb.WriteString("The orchestrator has STOPPED all other work until you resolve these conflicts.\n\n")

	// Include comprehensive context if available
	if mergeContext != nil {
		sb.WriteString(mergeContext.FormatForPrompt())
		sb.WriteString("\n")
	} else {
		// Fallback to basic context
		sb.WriteString("## Situation\n\n")
		sb.WriteString(fmt.Sprintf("- **Task ID**: %s\n", taskID))
		sb.WriteString(fmt.Sprintf("- **Target branch**: %s (integrated work from all completed agents)\n", targetBranch))
		sb.WriteString(fmt.Sprintf("- **Agent branch**: %s (new work that conflicts)\n", agentBranch))
		sb.WriteString(fmt.Sprintf("- **Conflicting files** (%d):\n", len(conflicts)))
		for _, file := range conflicts {
			sb.WriteString(fmt.Sprintf("  - %s\n", file))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Your Mission\n\n")
	sb.WriteString("1. **Understand Context**: Review the task history above to understand what each completed task accomplished\n")
	sb.WriteString("2. **Analyze Intent**: Read both conflicting versions to understand what each branch is trying to accomplish\n")
	sb.WriteString("3. **Develop Strategy**: EXPLAIN your resolution strategy before implementing:\n")
	sb.WriteString("   - What does the target branch accomplish?\n")
	sb.WriteString("   - What does the agent branch accomplish?\n")
	sb.WriteString("   - Are these changes compatible or contradictory?\n")
	sb.WriteString("   - How will you merge them to preserve both intents?\n")
	sb.WriteString("4. **Merge Intelligently**: Create unified versions that preserve BOTH intents when compatible\n")
	sb.WriteString("5. **Resolve Conflicts**: If changes are incompatible, choose the approach that maintains correctness\n")
	sb.WriteString("6. **Validate**: Ensure merged code compiles and tests pass\n")
	sb.WriteString("7. **Commit**: Stage all resolved files and commit the merge\n\n")

	sb.WriteString("## Critical Requirements\n\n")
	sb.WriteString("- **DO NOT** lose functionality from either branch unless truly contradictory\n")
	sb.WriteString("- **DO NOT** simply accept one side - merge the intents intelligently\n")
	sb.WriteString("- **DO** explain your resolution strategy before implementing\n")
	sb.WriteString("- **DO** consider the context of all completed tasks\n")
	sb.WriteString("- **DO** run tests after resolving to validate correctness\n")
	sb.WriteString("- **DO** ensure no conflict markers (<<<<<<, ======, >>>>>>) remain in any file\n")
	sb.WriteString(fmt.Sprintf("- **DO** commit once all conflicts resolved with message: \"Merge conflict resolved for task %s\"\n\n", taskID))

	sb.WriteString("## Commands Available\n\n")
	sb.WriteString("- Use **Read** tool to examine file contents from both branches\n")
	sb.WriteString("- Use **Edit** tool to create merged versions of conflicting files\n")
	sb.WriteString("- Use **Bash** to run tests/build: `go test ./...` or `npm test`\n")
	sb.WriteString("- Use **Bash** to check git status: `git status`\n")
	sb.WriteString("- When satisfied, stage files: `git add <resolved-files>`\n")
	sb.WriteString(fmt.Sprintf("- Commit: `git commit -m \"Merge conflict resolved for task %s\"`\n\n", taskID))

	sb.WriteString("## Validation Checklist\n\n")
	sb.WriteString("Before committing, ensure:\n")
	sb.WriteString("- [ ] All conflict markers removed from all files\n")
	sb.WriteString("- [ ] Both intents preserved (if compatible)\n")
	sb.WriteString("- [ ] Code compiles without errors\n")
	sb.WriteString("- [ ] Tests pass\n")
	sb.WriteString("- [ ] No functionality lost unintentionally\n\n")

	sb.WriteString("**IMPORTANT**: The entire orchestrator is BLOCKED waiting for you. Resolve completely and correctly.\n")
	sb.WriteString("You have extended retries (5 attempts) due to the complexity of merge resolution.\n")

	return sb.String()
}

func (mr *MergeResolverAgent) validateResolution(conflictFiles []string) error {
	// Check that conflicting files are no longer in conflict state
	status, err := mr.git.Status()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	// Look for conflict markers in git status
	if strings.Contains(status, "both modified") ||
		strings.Contains(status, "Unmerged paths") {
		return fmt.Errorf("conflicts still exist after resolution")
	}

	return nil
}

// validateNoConflictMarkers checks that no conflict markers remain in resolved files.
func (mr *MergeResolverAgent) validateNoConflictMarkers(conflictFiles []string) error {
	conflictMarkers := []string{"<<<<<<<", "=======", ">>>>>>>"}

	for _, file := range conflictFiles {
		// Read the file content using git show for the working tree
		content, err := mr.git.ShowFile("HEAD", file)
		if err != nil {
			// File might not be in HEAD yet if it's a new file being added
			// In that case, we can't check it via git show, skip it
			continue
		}

		// Check for conflict markers
		for _, marker := range conflictMarkers {
			if strings.Contains(content, marker) {
				return fmt.Errorf("conflict marker '%s' found in %s", marker, file)
			}
		}
	}

	return nil
}

func (mr *MergeResolverAgent) getTargetBranch() string {
	// Check orchestrator's greenfield flag
	if mr.orchestrator != nil && mr.orchestrator.config.Greenfield {
		return "main"
	}
	if mr.orchestrator != nil && mr.orchestrator.sessionMgr != nil {
		return mr.orchestrator.sessionMgr.GetBranchName()
	}
	return "main"
}
