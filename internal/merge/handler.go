// Package merge provides git merge operations with smart conflict handling.
package merge

import (
	"fmt"
	"path/filepath"

	"github.com/ShayCichocki/alphie/internal/git"
)

// Result represents the outcome of a merge operation.
type Result struct {
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
	Diff string
	// ChangedFiles lists the files that were changed in the merge.
	ChangedFiles []string
}

// Handler manages merge operations between agent branches and the session branch.
type Handler struct {
	sessionBranch string
	repoPath      string
	git           git.Runner
	debugLog      func(format string, args ...interface{})
}

// NewHandler creates a new Handler with the given session branch and repository path.
func NewHandler(sessionBranch, repoPath string) *Handler {
	return &Handler{
		sessionBranch: sessionBranch,
		repoPath:      repoPath,
		git:           git.NewRunner(repoPath),
		debugLog:      func(format string, args ...interface{}) {}, // no-op by default
	}
}

// NewHandlerWithRunner creates a new Handler with a custom git runner (for testing).
func NewHandlerWithRunner(sessionBranch, repoPath string, runner git.Runner) *Handler {
	return &Handler{
		sessionBranch: sessionBranch,
		repoPath:      repoPath,
		git:           runner,
		debugLog:      func(format string, args ...interface{}) {},
	}
}

// SetDebugLog sets the debug logging function.
func (m *Handler) SetDebugLog(fn func(format string, args ...interface{})) {
	if fn != nil {
		m.debugLog = fn
	}
}

// RepoPath returns the repository path for this merger.
func (m *Handler) RepoPath() string {
	return m.repoPath
}

// SessionBranch returns the session branch name.
func (m *Handler) SessionBranch() string {
	return m.sessionBranch
}

// GitRunner returns the git runner for direct git operations.
func (m *Handler) GitRunner() git.Runner {
	return m.git
}

// StageFiles stages the given files for commit.
func (m *Handler) StageFiles(paths ...string) error {
	return m.git.Add(paths...)
}

// CommitMerge commits the current staged changes with the given message.
func (m *Handler) CommitMerge(message string) error {
	return m.git.Commit(message)
}

// CheckoutOurs resolves a conflict by choosing the "ours" version.
func (m *Handler) CheckoutOurs(path string) error {
	return m.git.CheckoutOurs(path)
}

// CheckoutTheirs resolves a conflict by choosing the "theirs" version.
func (m *Handler) CheckoutTheirs(path string) error {
	return m.git.CheckoutTheirs(path)
}

// Merge attempts to merge the agent branch into the session branch.
func (m *Handler) Merge(agentBranch string) (*Result, error) {
	if err := m.git.CheckoutBranch(m.sessionBranch); err != nil {
		return &Result{
			Success: false,
			Error:   fmt.Errorf("checkout session branch: %w", err),
		}, nil
	}

	if err := m.git.MergeNoFF(agentBranch); err == nil {
		diff, _ := m.getMergeDiff()
		changedFiles, _ := m.getMergeChangedFiles()
		return &Result{
			Success:      true,
			Diff:         diff,
			ChangedFiles: changedFiles,
		}, nil
	}

	conflictFiles, _ := m.GetConflictedFiles()

	if err := m.AbortMerge(); err != nil {
		return &Result{
			Success:       false,
			ConflictFiles: conflictFiles,
			Error:         fmt.Errorf("abort merge: %w", err),
		}, nil
	}

	if err := m.git.CheckoutBranch(agentBranch); err != nil {
		return &Result{
			Success:       false,
			ConflictFiles: conflictFiles,
			Error:         fmt.Errorf("checkout agent branch for rebase: %w", err),
		}, nil
	}

	if err := m.git.Rebase(m.sessionBranch); err != nil {
		_ = m.git.RebaseAbort()
		_ = m.git.CheckoutBranch(m.sessionBranch)

		return &Result{
			Success:            false,
			ConflictFiles:      conflictFiles,
			NeedsSemanticMerge: true,
			Error:              fmt.Errorf("rebase failed: %w", err),
		}, nil
	}

	if err := m.git.CheckoutBranch(m.sessionBranch); err != nil {
		return &Result{
			Success:       false,
			ConflictFiles: conflictFiles,
			Error:         fmt.Errorf("checkout session branch after rebase: %w", err),
		}, nil
	}

	if err := m.git.MergeNoFF(agentBranch); err != nil {
		newConflictFiles, _ := m.GetConflictedFiles()
		_ = m.AbortMerge()

		return &Result{
			Success:            false,
			ConflictFiles:      newConflictFiles,
			NeedsSemanticMerge: true,
			Error:              fmt.Errorf("merge failed after rebase: %w", err),
		}, nil
	}

	diff, _ := m.getMergeDiff()
	changedFiles, _ := m.getMergeChangedFiles()
	return &Result{
		Success:      true,
		Diff:         diff,
		ChangedFiles: changedFiles,
	}, nil
}

// AbortMerge aborts an in-progress merge operation.
func (m *Handler) AbortMerge() error {
	return m.git.MergeAbort()
}

// GetConflictedFiles returns a list of files with merge conflicts.
func (m *Handler) GetConflictedFiles() ([]string, error) {
	return m.git.ConflictedFiles()
}

func (m *Handler) getMergeDiff() (string, error) {
	return m.git.DiffBetween("HEAD^", "HEAD")
}

func (m *Handler) getMergeChangedFiles() ([]string, error) {
	return m.git.ChangedFilesBetween("HEAD^", "HEAD")
}

// MergeWithRetry attempts to merge with multiple intelligent retry attempts.
func (m *Handler) MergeWithRetry(agentBranch string, maxRetries int) (*Result, error) {
	if maxRetries < 1 {
		maxRetries = 3
	}

	var lastConflictFiles []string
	var lastError error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if err := m.git.CheckoutBranch(m.sessionBranch); err != nil {
			return &Result{
				Success: false,
				Error:   fmt.Errorf("checkout session branch (attempt %d): %w", attempt, err),
			}, nil
		}

		_ = m.git.PullFFOnly()

		if err := m.git.MergeNoFF(agentBranch); err == nil {
			diff, _ := m.getMergeDiff()
			changedFiles, _ := m.getMergeChangedFiles()
			return &Result{
				Success:      true,
				Diff:         diff,
				ChangedFiles: changedFiles,
			}, nil
		}

		lastConflictFiles, _ = m.GetConflictedFiles()
		_ = m.AbortMerge()

		if err := m.git.CheckoutBranch(agentBranch); err != nil {
			lastError = fmt.Errorf("checkout agent branch for rebase (attempt %d): %w", attempt, err)
			_ = m.git.CheckoutBranch(m.sessionBranch)
			continue
		}

		if err := m.git.Rebase(m.sessionBranch); err != nil {
			_ = m.git.RebaseAbort()
			_ = m.git.CheckoutBranch(m.sessionBranch)
			lastError = fmt.Errorf("rebase failed (attempt %d): %w", attempt, err)
			continue
		}

		if err := m.git.CheckoutBranch(m.sessionBranch); err != nil {
			lastError = fmt.Errorf("checkout session after rebase (attempt %d): %w", attempt, err)
			continue
		}
	}

	return &Result{
		Success:            false,
		ConflictFiles:      lastConflictFiles,
		NeedsSemanticMerge: true,
		Error:              fmt.Errorf("merge failed after %d attempts: %w", maxRetries, lastError),
	}, nil
}

// DeleteBranch deletes the specified branch.
func (m *Handler) DeleteBranch(branch string) error {
	return m.git.DeleteBranch(branch)
}

func (m *Handler) getChangedFiles(branch, relativeTo string) ([]string, error) {
	return m.git.ChangedFilesRelative(branch, relativeTo)
}

func (m *Handler) getMergeBase(branch1, branch2 string) (string, error) {
	return m.git.MergeBase(branch1, branch2)
}

func (m *Handler) detectCriticalFileConflict(agentBranch string) ([]string, bool) {
	mergeBase, err := m.getMergeBase(m.sessionBranch, agentBranch)
	if err != nil {
		return nil, false
	}

	agentFiles, err := m.getChangedFiles(agentBranch, mergeBase)
	if err != nil {
		return nil, false
	}

	sessionFiles, err := m.getChangedFiles(m.sessionBranch, mergeBase)
	if err != nil {
		return nil, false
	}

	if !HasCriticalFileOverlap(agentFiles, sessionFiles) {
		return nil, false
	}

	agentCritical := GetCriticalFilesFromList(agentFiles)
	sessionCritical := GetCriticalFilesFromList(sessionFiles)

	sessionSet := make(map[string]bool)
	for _, f := range sessionCritical {
		sessionSet[f] = true
	}

	var overlapping []string
	for _, f := range agentCritical {
		if sessionSet[f] {
			overlapping = append(overlapping, f)
		}
	}

	return overlapping, len(overlapping) > 0
}

// MergeWithSmartFallback attempts a normal merge, but uses smart merge for
// critical file conflicts before falling back to semantic merge.
func (m *Handler) MergeWithSmartFallback(agentBranch string) (*Result, error) {
	criticalFiles, hasCritical := m.detectCriticalFileConflict(agentBranch)

	if hasCritical {
		m.debugLog("[merger] detected critical file conflicts: %v", criticalFiles)

		smartResult, err := SmartMerge(m.repoPath, criticalFiles, m.sessionBranch, agentBranch)
		if err == nil && smartResult.Success {
			if err := ApplySmartMerge(m.repoPath, smartResult); err != nil {
				m.debugLog("[merger] failed to apply smart merge: %v", err)
			} else {
				for file := range smartResult.MergedFiles {
					_ = m.git.Add(file)
				}
				m.debugLog("[merger] applied smart merge for %d files", len(smartResult.MergedFiles))
			}
		} else if err != nil {
			m.debugLog("[merger] smart merge failed: %v", err)
		} else {
			m.debugLog("[merger] smart merge had conflicts: %v", smartResult.Conflicts)
		}
	}

	return m.Merge(agentBranch)
}

// SmartMergeForConflicts handles merge conflicts by using format-aware merge logic.
func (m *Handler) SmartMergeForConflicts(agentBranch string, conflictFiles []string) (*Result, error) {
	criticalConflicts := GetCriticalFilesFromList(conflictFiles)
	if len(criticalConflicts) == 0 {
		return &Result{
			Success:            false,
			ConflictFiles:      conflictFiles,
			NeedsSemanticMerge: true,
		}, nil
	}

	smartResult, err := SmartMerge(m.repoPath, criticalConflicts, m.sessionBranch, agentBranch)
	if err != nil {
		return &Result{
			Success:            false,
			ConflictFiles:      conflictFiles,
			NeedsSemanticMerge: true,
			Error:              fmt.Errorf("smart merge failed: %w", err),
		}, nil
	}

	if !smartResult.Success {
		return &Result{
			Success:            false,
			ConflictFiles:      smartResult.Conflicts,
			NeedsSemanticMerge: true,
		}, nil
	}

	if err := ApplySmartMerge(m.repoPath, smartResult); err != nil {
		return &Result{
			Success:            false,
			ConflictFiles:      conflictFiles,
			NeedsSemanticMerge: true,
			Error:              fmt.Errorf("apply smart merge: %w", err),
		}, nil
	}

	for file := range smartResult.MergedFiles {
		fullPath := filepath.Join(m.repoPath, file)
		if err := m.git.Add(fullPath); err != nil {
			m.debugLog("[merger] failed to stage %s: %v", file, err)
		}
	}

	var remainingConflicts []string
	for _, file := range conflictFiles {
		if _, merged := smartResult.MergedFiles[file]; !merged {
			if !IsLockFile(file) {
				remainingConflicts = append(remainingConflicts, file)
			}
		}
	}

	if len(remainingConflicts) > 0 {
		return &Result{
			Success:            false,
			ConflictFiles:      remainingConflicts,
			NeedsSemanticMerge: true,
		}, nil
	}

	if err := m.git.Commit("Smart merge: resolved critical file conflicts"); err != nil {
		return &Result{
			Success:            false,
			ConflictFiles:      conflictFiles,
			NeedsSemanticMerge: true,
			Error:              fmt.Errorf("commit smart merge: %w", err),
		}, nil
	}

	diff, _ := m.getMergeDiff()
	changedFiles, _ := m.getMergeChangedFiles()

	return &Result{
		Success:      true,
		Diff:         diff,
		ChangedFiles: changedFiles,
	}, nil
}
