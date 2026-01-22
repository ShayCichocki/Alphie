// Package learning provides learning and context management capabilities.
package learning

// LearningProvider defines the interface for learning system integration.
// This interface allows the orchestrator to work with any learning backend
// without depending on the concrete implementation.
type LearningProvider interface {
	// OnTaskStart is called at the beginning of a task to retrieve relevant learnings.
	// It retrieves learnings based on task description and file paths,
	// records triggers for matched learnings, and returns them for injection
	// into the agent context.
	OnTaskStart(taskDescription string, filePaths []string) ([]*Learning, error)

	// OnTaskComplete is called when a task finishes to handle learning opportunities.
	// If the task failed with an unknown pattern, it returns information suggesting
	// a candidate learning should be created.
	OnTaskComplete(taskID string, success bool) error

	// OnFailure is called when an error occurs to check for existing learnings
	// that match the error pattern. If found, it returns the learnings and
	// records their triggers. If no matching learning exists, it returns nil
	// to signal the caller may want to create a new learning.
	OnFailure(errorMessage string) ([]*Learning, error)

	// Close closes the learning system and releases all resources.
	Close() error
}

// Verify LearningSystem implements LearningProvider at compile time.
var _ LearningProvider = (*LearningSystem)(nil)
