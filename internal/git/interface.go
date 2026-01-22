// Package git provides an interface for git operations.
package git

// BranchOperations defines the interface for git branch operations.
type BranchOperations interface {
	// CurrentBranch returns the name of the current branch.
	CurrentBranch() (string, error)
	// CreateBranch creates a new branch with the given name.
	CreateBranch(name string) error
	// CreateAndCheckoutBranch creates and switches to a new branch (git checkout -b).
	CreateAndCheckoutBranch(name string) error
	// CheckoutBranch switches to the specified branch.
	CheckoutBranch(name string) error
	// BranchExists returns true if the branch exists.
	BranchExists(name string) (bool, error)
	// DeleteBranch deletes the specified branch (force delete).
	DeleteBranch(name string) error
}

// DiffOperations defines the interface for git diff and status operations.
type DiffOperations interface {
	// Status returns the output of git status --porcelain.
	Status() (string, error)
	// HasChanges returns true if there are uncommitted changes.
	HasChanges() (bool, error)
	// Diff returns the diff between the current state and the given base.
	Diff(base string) (string, error)
	// DiffBetween returns the diff between two refs.
	DiffBetween(ref1, ref2 string) (string, error)
	// ChangedFiles returns a list of files changed since the base ref.
	ChangedFiles(base string) ([]string, error)
	// ChangedFilesBetween returns files changed between two refs.
	ChangedFilesBetween(ref1, ref2 string) ([]string, error)
	// ChangedFilesRelative returns files changed on a branch relative to another.
	// Uses the triple-dot diff (branch1...branch2).
	ChangedFilesRelative(branch, relativeTo string) ([]string, error)
	// ConflictedFiles returns a list of files with unmerged changes.
	ConflictedFiles() ([]string, error)
}

// CommitOperations defines the interface for git commit operations.
type CommitOperations interface {
	// Add stages the specified files for commit.
	Add(paths ...string) error
	// Commit creates a new commit with the given message.
	Commit(message string) error
	// Reset resets the staging area to the specified ref.
	Reset(ref string) error
	// CheckoutPath discards changes to a specific path.
	CheckoutPath(path string) error
}

// MergeOperations defines the interface for git merge and rebase operations.
type MergeOperations interface {
	// Merge merges the specified branch into the current branch (fast-forward if possible).
	Merge(branch string) error
	// MergeNoFF merges the specified branch creating a merge commit (--no-ff).
	MergeNoFF(branch string) error
	// MergeNoFFMessage merges the specified branch with --no-ff and a custom message.
	MergeNoFFMessage(branch, message string) error
	// MergeAbort aborts an in-progress merge.
	MergeAbort() error
	// MergeBase returns the common ancestor of two branches.
	MergeBase(branch1, branch2 string) (string, error)
	// HasConflicts returns true if there are merge conflicts.
	HasConflicts() (bool, error)
	// Rebase rebases the current branch onto the specified base.
	Rebase(base string) error
	// RebaseAbort aborts an in-progress rebase.
	RebaseAbort() error
}

// WorktreeOperations defines the interface for git worktree operations.
type WorktreeOperations interface {
	// WorktreeAdd creates a new worktree at the given path for the branch.
	WorktreeAdd(path, branch string) error
	// WorktreeAddNewBranch creates a new worktree with a new branch (git worktree add -b).
	WorktreeAddNewBranch(path, branch string) error
	// WorktreeRemove removes the worktree at the given path.
	WorktreeRemove(path string) error
	// WorktreeRemoveOptionalForce removes the worktree, optionally with force.
	WorktreeRemoveOptionalForce(path string, force bool) error
	// WorktreeUnlock unlocks a locked worktree.
	WorktreeUnlock(path string) error
	// WorktreeList returns a list of worktree paths.
	WorktreeList() ([]string, error)
	// WorktreeListPorcelain returns the raw porcelain output for detailed parsing.
	WorktreeListPorcelain() (string, error)
	// WorktreePrune removes stale worktree entries.
	WorktreePrune() error
	// WorktreePruneExpireNow prunes worktrees with --expire now.
	WorktreePruneExpireNow() error
}

// RemoteOperations defines the interface for git remote operations.
type RemoteOperations interface {
	// PullFFOnly pulls from remote with fast-forward only.
	// Returns nil if no remote is configured.
	PullFFOnly() error
}

// FileOperations defines the interface for git file operations.
type FileOperations interface {
	// ShowFile returns the contents of a file at a specific ref.
	ShowFile(ref, path string) (string, error)
	// CheckoutOurs checks out the "ours" version of a conflicted file.
	CheckoutOurs(path string) error
	// CheckoutTheirs checks out the "theirs" version of a conflicted file.
	CheckoutTheirs(path string) error
}

// Runner defines the complete interface for git operations.
// This interface embeds all focused interfaces for full functionality.
// Consumers should prefer using focused interfaces when possible.
type Runner interface {
	BranchOperations
	DiffOperations
	CommitOperations
	MergeOperations
	WorktreeOperations
	RemoteOperations
	FileOperations
	// Run executes an arbitrary git command with the given arguments.
	// Returns the command output and an error if the command fails.
	Run(args ...string) (string, error)
}
