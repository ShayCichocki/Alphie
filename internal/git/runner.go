// Package git provides an interface for git operations.
package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// ExecRunner implements Runner using exec.Command.
type ExecRunner struct {
	repoPath string
}

// NewRunner creates a new git runner for the repository at the given path.
func NewRunner(repoPath string) *ExecRunner {
	return &ExecRunner{repoPath: repoPath}
}

// run executes a git command and returns its output.
func (r *ExecRunner) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// runSilent executes a git command and ignores output.
func (r *ExecRunner) runSilent(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, string(out))
	}
	return nil
}

// Run executes an arbitrary git command with the given arguments.
// This is the public version of run() for generic git operations.
func (r *ExecRunner) Run(args ...string) (string, error) {
	return r.run(args...)
}

// CurrentBranch returns the name of the current branch.
func (r *ExecRunner) CurrentBranch() (string, error) {
	return r.run("rev-parse", "--abbrev-ref", "HEAD")
}

// CreateBranch creates a new branch with the given name.
func (r *ExecRunner) CreateBranch(name string) error {
	return r.runSilent("branch", name)
}

// CreateAndCheckoutBranch creates and switches to a new branch (git checkout -b).
func (r *ExecRunner) CreateAndCheckoutBranch(name string) error {
	return r.runSilent("checkout", "-b", name)
}

// CheckoutBranch switches to the specified branch.
func (r *ExecRunner) CheckoutBranch(name string) error {
	return r.runSilent("checkout", name)
}

// BranchExists returns true if the branch exists.
func (r *ExecRunner) BranchExists(name string) (bool, error) {
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	cmd.Dir = r.repoPath
	err := cmd.Run()
	if err != nil {
		// Exit code 1 means branch doesn't exist (not an error)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("check branch exists: %w", err)
	}
	return true, nil
}

// DeleteBranch deletes the specified branch.
func (r *ExecRunner) DeleteBranch(name string) error {
	return r.runSilent("branch", "-D", name)
}

// Status returns the output of git status --porcelain.
func (r *ExecRunner) Status() (string, error) {
	return r.run("status", "--porcelain")
}

// HasChanges returns true if there are uncommitted changes.
func (r *ExecRunner) HasChanges() (bool, error) {
	status, err := r.Status()
	if err != nil {
		return false, err
	}
	return len(status) > 0, nil
}

// Diff returns the diff between the current state and the given base.
func (r *ExecRunner) Diff(base string) (string, error) {
	return r.run("diff", base)
}

// ChangedFiles returns a list of files changed since the base ref.
func (r *ExecRunner) ChangedFiles(base string) ([]string, error) {
	out, err := r.run("diff", "--name-only", base)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// Add stages the specified files for commit.
func (r *ExecRunner) Add(paths ...string) error {
	args := append([]string{"add"}, paths...)
	return r.runSilent(args...)
}

// Commit creates a new commit with the given message.
func (r *ExecRunner) Commit(message string) error {
	return r.runSilent("commit", "-m", message)
}

// Reset resets the staging area to the specified ref.
func (r *ExecRunner) Reset(ref string) error {
	return r.runSilent("reset", ref)
}

// CheckoutPath discards changes to a specific path.
func (r *ExecRunner) CheckoutPath(path string) error {
	return r.runSilent("checkout", path)
}

// Merge merges the specified branch into the current branch.
func (r *ExecRunner) Merge(branch string) error {
	return r.runSilent("merge", branch)
}

// MergeAbort aborts an in-progress merge.
func (r *ExecRunner) MergeAbort() error {
	return r.runSilent("merge", "--abort")
}

// HasConflicts returns true if there are merge conflicts.
func (r *ExecRunner) HasConflicts() (bool, error) {
	status, err := r.Status()
	if err != nil {
		return false, err
	}
	// Check for conflict markers (UU, AA, DD, etc.)
	for _, line := range strings.Split(status, "\n") {
		if len(line) >= 2 {
			prefix := line[:2]
			if prefix == "UU" || prefix == "AA" || prefix == "DD" ||
				prefix == "AU" || prefix == "UA" || prefix == "DU" || prefix == "UD" {
				return true, nil
			}
		}
	}
	return false, nil
}

// WorktreeAdd creates a new worktree at the given path for the branch.
func (r *ExecRunner) WorktreeAdd(path, branch string) error {
	return r.runSilent("worktree", "add", path, branch)
}

// WorktreeAddNewBranch creates a new worktree with a new branch (git worktree add -b).
func (r *ExecRunner) WorktreeAddNewBranch(path, branch string) error {
	return r.runSilent("worktree", "add", path, "-b", branch)
}

// WorktreeRemove removes the worktree at the given path.
func (r *ExecRunner) WorktreeRemove(path string) error {
	return r.runSilent("worktree", "remove", "--force", path)
}

// WorktreeRemoveOptionalForce removes the worktree, optionally with force.
func (r *ExecRunner) WorktreeRemoveOptionalForce(path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, path)
	return r.runSilent(args...)
}

// WorktreeUnlock unlocks a locked worktree.
func (r *ExecRunner) WorktreeUnlock(path string) error {
	return r.runSilent("worktree", "unlock", path)
}

// WorktreeList returns a list of worktree paths.
func (r *ExecRunner) WorktreeList() ([]string, error) {
	out, err := r.run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	var paths []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "worktree ") {
			paths = append(paths, strings.TrimPrefix(line, "worktree "))
		}
	}
	return paths, nil
}

// WorktreeListPorcelain returns the raw porcelain output for detailed parsing.
func (r *ExecRunner) WorktreeListPorcelain() (string, error) {
	return r.run("worktree", "list", "--porcelain")
}

// WorktreePrune removes stale worktree entries.
func (r *ExecRunner) WorktreePrune() error {
	return r.runSilent("worktree", "prune")
}

// WorktreePruneExpireNow prunes worktrees with --expire now.
func (r *ExecRunner) WorktreePruneExpireNow() error {
	return r.runSilent("worktree", "prune", "--expire", "now")
}

// ShowFile returns the contents of a file at a specific ref.
func (r *ExecRunner) ShowFile(ref, path string) (string, error) {
	return r.run("show", ref+":"+path)
}

// DiffBetween returns the diff between two refs.
func (r *ExecRunner) DiffBetween(ref1, ref2 string) (string, error) {
	return r.run("diff", ref1, ref2)
}

// ChangedFilesBetween returns files changed between two refs.
func (r *ExecRunner) ChangedFilesBetween(ref1, ref2 string) ([]string, error) {
	out, err := r.run("diff", "--name-only", ref1, ref2)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ChangedFilesRelative returns files changed on a branch relative to another.
func (r *ExecRunner) ChangedFilesRelative(branch, relativeTo string) ([]string, error) {
	out, err := r.run("diff", "--name-only", relativeTo+"..."+branch)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ConflictedFiles returns a list of files with unmerged changes.
func (r *ExecRunner) ConflictedFiles() ([]string, error) {
	out, err := r.run("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		// If there are no conflicts, git may exit with code 0 but empty output
		return nil, nil
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// MergeNoFF merges the specified branch creating a merge commit.
func (r *ExecRunner) MergeNoFF(branch string) error {
	return r.runSilent("merge", branch, "--no-ff")
}

// MergeNoFFMessage merges the specified branch with --no-ff and a custom message.
func (r *ExecRunner) MergeNoFFMessage(branch, message string) error {
	return r.runSilent("merge", "--no-ff", "-m", message, branch)
}

// MergeBase returns the common ancestor of two branches.
func (r *ExecRunner) MergeBase(branch1, branch2 string) (string, error) {
	return r.run("merge-base", branch1, branch2)
}

// Rebase rebases the current branch onto the specified base.
func (r *ExecRunner) Rebase(base string) error {
	return r.runSilent("rebase", base)
}

// RebaseAbort aborts an in-progress rebase.
func (r *ExecRunner) RebaseAbort() error {
	return r.runSilent("rebase", "--abort")
}

// PullFFOnly pulls from remote with fast-forward only.
// Returns nil if no remote is configured or pull fails (non-fatal for local repos).
func (r *ExecRunner) PullFFOnly() error {
	// Ignore errors - pull may fail if there's no remote, which is fine
	_ = r.runSilent("pull", "--ff-only")
	return nil
}

// CheckoutOurs checks out the "ours" version of a conflicted file.
func (r *ExecRunner) CheckoutOurs(path string) error {
	return r.runSilent("checkout", "--ours", path)
}

// CheckoutTheirs checks out the "theirs" version of a conflicted file.
func (r *ExecRunner) CheckoutTheirs(path string) error {
	return r.runSilent("checkout", "--theirs", path)
}

// Verify ExecRunner implements Runner at compile time.
var _ Runner = (*ExecRunner)(nil)
