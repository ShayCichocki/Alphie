// Package prog provides the prog task management client for alphie.
package prog

import "io"

// EpicManager handles epic lifecycle operations.
type EpicManager interface {
	// CreateEpic creates a new epic and returns its ID.
	CreateEpic(title string, opts *EpicOptions) (string, error)

	// GetEpic retrieves an epic by ID. Returns an error if the item is not an epic.
	GetEpic(id string) (*Item, error)

	// FindInProgressEpic returns an in-progress epic for the client's project, if any.
	FindInProgressEpic() (*Item, error)

	// ComputeEpicProgress returns the number of completed and total tasks for an epic.
	ComputeEpicProgress(epicID string) (completed int, total int, err error)

	// UpdateEpicStatusIfComplete marks the epic as done if all child tasks are done.
	UpdateEpicStatusIfComplete(epicID string) (bool, error)
}

// TaskManager handles task lifecycle operations.
type TaskManager interface {
	// CreateTask creates a new task and returns its ID.
	CreateTask(title string, opts *TaskOptions) (string, error)

	// GetItem retrieves an item by ID.
	GetItem(id string) (*Item, error)

	// GetChildTasks returns all tasks under a given epic ID.
	GetChildTasks(epicID string) ([]Item, error)

	// GetIncompleteTasks returns tasks under an epic that are not yet done.
	GetIncompleteTasks(epicID string) ([]Item, error)
}

// StatusUpdater handles status mutations.
type StatusUpdater interface {
	// UpdateStatus changes an item's status.
	UpdateStatus(id string, status Status) error

	// Start marks an item as in progress.
	Start(id string) error

	// Done marks an item as completed.
	Done(id string) error

	// Block marks an item as blocked.
	Block(id string) error
}

// MetadataRecorder handles auxiliary operations.
type MetadataRecorder interface {
	// AddLog adds a timestamped log entry to an item.
	AddLog(itemID, message string) error

	// AddDependency adds a dependency between items.
	AddDependency(itemID, dependsOnID string) error

	// AddLearning creates a new learning entry and returns its ID.
	AddLearning(summary string, opts *LearningOptions) (string, error)
}

// ProgTracker defines the interface for cross-session task tracking.
// It composes focused interfaces for specific concerns.
type ProgTracker interface {
	io.Closer
	EpicManager
	TaskManager
	StatusUpdater
	MetadataRecorder
}

// Compile-time interface verification.
var (
	_ ProgTracker      = (*Client)(nil)
	_ EpicManager      = (*Client)(nil)
	_ TaskManager      = (*Client)(nil)
	_ StatusUpdater    = (*Client)(nil)
	_ MetadataRecorder = (*Client)(nil)
)
