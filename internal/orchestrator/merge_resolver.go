package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/git"
)

// MergeResolverAgent is a dedicated agent for resolving merge conflicts.
type MergeResolverAgent struct {
	claudeFactory agent.ClaudeRunnerFactory
	git           git.Runner
	repoPath      string
	orchestrator  *Orchestrator
}

// NewMergeResolverAgent creates a new merge resolver agent.
func NewMergeResolverAgent(
	claudeFactory agent.ClaudeRunnerFactory,
	git git.Runner,
	repoPath string,
	orchestrator *Orchestrator,
) *MergeResolverAgent {
	return &MergeResolverAgent{
		claudeFactory: claudeFactory,
		git:           git,
		repoPath:      repoPath,
		orchestrator:  orchestrator,
	}
}

// Resolve spawns a dedicated Claude agent to resolve merge conflicts.
func (mr *MergeResolverAgent) Resolve(ctx context.Context, req *MergeRequest, conflictFiles []string) error {
	targetBranch := mr.getTargetBranch()

	// Build merge resolution prompt
	prompt := mr.buildMergePrompt(targetBranch, req.AgentBranch, conflictFiles, req.TaskID)

	// Create fresh Claude runner for merge resolution
	claude := mr.claudeFactory.NewRunner()

	// Start Claude with merge resolution task
	if err := claude.Start(prompt, mr.repoPath); err != nil {
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

	// Clear merge conflict flag - resume scheduling
	if mr.orchestrator != nil {
		mr.orchestrator.ClearMergeConflict()
	}

	return nil
}

func (mr *MergeResolverAgent) buildMergePrompt(targetBranch, agentBranch string, conflicts []string, taskID string) string {
	return fmt.Sprintf(`# URGENT: Merge Conflict Resolution Required

You are a dedicated merge conflict resolver. The orchestrator has STOPPED all other work until you resolve these conflicts.

## Situation
- **Task ID**: %s
- **Target branch**: %s (integrated work from all completed agents)
- **Agent branch**: %s (new work that conflicts)
- **Conflicting files** (%d):
%s

## Your Mission
1. **Understand intent**: Read both versions of each conflicting file
2. **Analyze**: Determine what each branch is trying to accomplish
3. **Merge**: Create unified versions that preserve BOTH intents when compatible
4. **Resolve**: If changes are incompatible, choose the approach that maintains correctness
5. **Validate**: Ensure merged code compiles and tests pass
6. **Commit**: Stage all resolved files and commit the merge

## Critical Requirements
- **DO NOT** lose functionality from either branch unless truly contradictory
- **DO NOT** simply accept one side - merge the intents
- **DO** run tests after resolving to validate correctness
- **DO** commit once all conflicts resolved with message: "Merge conflict resolved for task %s"

## Commands Available
- Use Read tool to examine both versions
- Use Edit tool to create merged versions
- Use Bash to run tests/build: 'go test ./...' or 'npm test'
- When satisfied, stage files: 'git add <resolved-files>'
- Commit: 'git commit -m "Merge conflict resolved for task %s"'

IMPORTANT: The entire orchestrator is BLOCKED waiting for you. Resolve completely and correctly.
`, taskID, targetBranch, agentBranch, len(conflicts), strings.Join(conflicts, "\n"), taskID, taskID)
}

func (mr *MergeResolverAgent) validateResolution(conflictFiles []string) error {
	// Check that conflicting files are no longer in conflict state
	status, err := mr.git.Status()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	// Look for conflict markers
	if strings.Contains(status, "both modified") ||
		strings.Contains(status, "Unmerged paths") {
		return fmt.Errorf("conflicts still exist after resolution")
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
