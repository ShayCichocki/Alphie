// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// MergeResult represents the outcome of a merge operation.
type MergeResult struct {
	// Success indicates whether the merge completed without conflicts.
	Success bool
	// ConflictFiles lists the files that have conflicts.
	ConflictFiles []string
	// NeedsSemanticMerge indicates that automatic merge/rebase failed
	// and the conflict requires semantic resolution (AI-assisted merge).
	NeedsSemanticMerge bool
	// Error contains any error that occurred during the merge.
	Error error
	// Diff contains the unified diff of the merged changes.
	// Populated only on successful merge.
	Diff string
	// ChangedFiles lists the files that were changed in the merge.
	// Populated only on successful merge.
	ChangedFiles []string
}

// MergeHandler manages merge operations between agent branches and the session branch.
type MergeHandler struct {
	// sessionBranch is the target branch for merging agent work.
	sessionBranch string
	// repoPath is the path to the git repository.
	repoPath string
}

// NewMergeHandler creates a new MergeHandler with the given session branch and repository path.
func NewMergeHandler(sessionBranch, repoPath string) *MergeHandler {
	return &MergeHandler{
		sessionBranch: sessionBranch,
		repoPath:      repoPath,
	}
}

// Merge attempts to merge the agent branch into the session branch.
// The workflow is:
//  1. Checkout session branch
//  2. Attempt git merge with --no-ff
//  3. If conflict: abort merge, rebase agent on session, retry merge
//  4. If still fails: return NeedsSemanticMerge = true
func (m *MergeHandler) Merge(agentBranch string) (*MergeResult, error) {
	// Step 1: Checkout session branch.
	if err := m.runGit("checkout", m.sessionBranch); err != nil {
		return &MergeResult{
			Success: false,
			Error:   fmt.Errorf("checkout session branch: %w", err),
		}, nil
	}

	// Step 2: Attempt merge with --no-ff.
	if err := m.runGit("merge", agentBranch, "--no-ff"); err == nil {
		// Merge succeeded - get diff and changed files
		diff, _ := m.getMergeDiff()
		changedFiles, _ := m.getMergeChangedFiles()
		return &MergeResult{
			Success:      true,
			Diff:         diff,
			ChangedFiles: changedFiles,
		}, nil
	}

	// Merge failed - get conflict files before aborting.
	conflictFiles, _ := m.GetConflictedFiles()

	// Abort the failed merge.
	if err := m.AbortMerge(); err != nil {
		return &MergeResult{
			Success:       false,
			ConflictFiles: conflictFiles,
			Error:         fmt.Errorf("abort merge: %w", err),
		}, nil
	}

	// Step 3: Rebase agent branch onto session branch.
	// First checkout the agent branch.
	if err := m.runGit("checkout", agentBranch); err != nil {
		return &MergeResult{
			Success:       false,
			ConflictFiles: conflictFiles,
			Error:         fmt.Errorf("checkout agent branch for rebase: %w", err),
		}, nil
	}

	// Attempt rebase.
	if err := m.runGit("rebase", m.sessionBranch); err != nil {
		// Rebase failed - abort it.
		_ = m.runGit("rebase", "--abort")

		// CRITICAL: Checkout back to session branch before returning.
		// Otherwise we leave the repo on the agent branch.
		_ = m.runGit("checkout", m.sessionBranch)

		// Step 4: Return NeedsSemanticMerge.
		return &MergeResult{
			Success:            false,
			ConflictFiles:      conflictFiles,
			NeedsSemanticMerge: true,
			Error:              fmt.Errorf("rebase failed: %w", err),
		}, nil
	}

	// Rebase succeeded - checkout session branch and retry merge.
	if err := m.runGit("checkout", m.sessionBranch); err != nil {
		return &MergeResult{
			Success:       false,
			ConflictFiles: conflictFiles,
			Error:         fmt.Errorf("checkout session branch after rebase: %w", err),
		}, nil
	}

	// Retry merge after rebase.
	if err := m.runGit("merge", agentBranch, "--no-ff"); err != nil {
		// Still failing - get new conflict files.
		newConflictFiles, _ := m.GetConflictedFiles()
		_ = m.AbortMerge()

		// Step 4: Return NeedsSemanticMerge.
		return &MergeResult{
			Success:            false,
			ConflictFiles:      newConflictFiles,
			NeedsSemanticMerge: true,
			Error:              fmt.Errorf("merge failed after rebase: %w", err),
		}, nil
	}

	// Merge succeeded after rebase - get diff and changed files
	diff, _ := m.getMergeDiff()
	changedFiles, _ := m.getMergeChangedFiles()
	return &MergeResult{
		Success:      true,
		Diff:         diff,
		ChangedFiles: changedFiles,
	}, nil
}

// AbortMerge aborts an in-progress merge operation.
func (m *MergeHandler) AbortMerge() error {
	return m.runGit("merge", "--abort")
}

// GetConflictedFiles returns a list of files with merge conflicts.
func (m *MergeHandler) GetConflictedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	cmd.Dir = m.repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get conflicted files: %w", err)
	}

	var files []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan conflicted files: %w", err)
	}

	return files, nil
}

// getMergeDiff returns the diff of the last merge commit compared to its first parent.
func (m *MergeHandler) getMergeDiff() (string, error) {
	cmd := exec.Command("git", "diff", "HEAD^", "HEAD")
	cmd.Dir = m.repoPath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get merge diff: %w", err)
	}

	return string(output), nil
}

// getMergeChangedFiles returns the list of files changed in the last merge commit.
func (m *MergeHandler) getMergeChangedFiles() ([]string, error) {
	cmd := exec.Command("git", "diff", "--name-only", "HEAD^", "HEAD")
	cmd.Dir = m.repoPath

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("get merge changed files: %w", err)
	}

	var files []string
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			files = append(files, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan changed files: %w", err)
	}

	return files, nil
}

// MergeWithRetry attempts to merge with multiple intelligent retry attempts.
// It will try merge, and on conflict: abort, pull latest, rebase, and retry.
// After maxRetries failures, it returns NeedsSemanticMerge = true.
func (m *MergeHandler) MergeWithRetry(agentBranch string, maxRetries int) (*MergeResult, error) {
	if maxRetries < 1 {
		maxRetries = 3
	}

	var lastConflictFiles []string
	var lastError error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		// Step 1: Checkout session branch and pull latest
		if err := m.runGit("checkout", m.sessionBranch); err != nil {
			return &MergeResult{
				Success: false,
				Error:   fmt.Errorf("checkout session branch (attempt %d): %w", attempt, err),
			}, nil
		}

		// Pull latest changes (in case other agents have merged)
		_ = m.runGit("pull", "--ff-only")

		// Step 2: Attempt merge
		if err := m.runGit("merge", agentBranch, "--no-ff"); err == nil {
			// Merge succeeded
			diff, _ := m.getMergeDiff()
			changedFiles, _ := m.getMergeChangedFiles()
			return &MergeResult{
				Success:      true,
				Diff:         diff,
				ChangedFiles: changedFiles,
			}, nil
		}

		// Merge failed - get conflict files
		lastConflictFiles, _ = m.GetConflictedFiles()

		// Abort the failed merge
		_ = m.AbortMerge()

		// Step 3: Rebase agent branch onto session branch
		if err := m.runGit("checkout", agentBranch); err != nil {
			lastError = fmt.Errorf("checkout agent branch for rebase (attempt %d): %w", attempt, err)
			_ = m.runGit("checkout", m.sessionBranch)
			continue
		}

		if err := m.runGit("rebase", m.sessionBranch); err != nil {
			// Rebase failed - abort and try again if we have retries left
			_ = m.runGit("rebase", "--abort")
			_ = m.runGit("checkout", m.sessionBranch)
			lastError = fmt.Errorf("rebase failed (attempt %d): %w", attempt, err)
			continue
		}

		// Rebase succeeded - checkout session branch for next merge attempt
		if err := m.runGit("checkout", m.sessionBranch); err != nil {
			lastError = fmt.Errorf("checkout session after rebase (attempt %d): %w", attempt, err)
			continue
		}
	}

	// All retries exhausted
	return &MergeResult{
		Success:            false,
		ConflictFiles:      lastConflictFiles,
		NeedsSemanticMerge: true,
		Error:              fmt.Errorf("merge failed after %d attempts: %w", maxRetries, lastError),
	}, nil
}

// DeleteBranch deletes the specified branch.
func (m *MergeHandler) DeleteBranch(branch string) error {
	return m.runGit("branch", "-D", branch)
}

// runGit executes a git command in the repository.
func (m *MergeHandler) runGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = m.repoPath

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(output))
	}

	return nil
}
