// Package prog provides the prog task management client for alphie.
// This file wraps the vendored prog database operations with an alphie-specific interface.
package prog

import (
	"fmt"
	"time"
)

// Client provides an alphie-specific interface to prog operations.
// It wraps the low-level database operations with higher-level methods
// that handle both project-local and global scopes.
type Client struct {
	db      *DB
	project string // Default project for operations, empty for global scope
}

// NewClient creates a new prog client with the given database and optional project scope.
// If project is empty, operations default to global scope.
func NewClient(db *DB, project string) *Client {
	return &Client{
		db:      db,
		project: project,
	}
}

// NewClientDefault creates a new prog client using the default database path.
// Returns an error if the database cannot be opened or initialized.
func NewClientDefault(project string) (*Client, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, fmt.Errorf("failed to get default db path: %w", err)
	}

	db, err := Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Init(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return NewClient(db, project), nil
}

// Close closes the underlying database connection.
func (c *Client) Close() error {
	return c.db.Close()
}

// Project returns the client's default project scope.
func (c *Client) Project() string {
	return c.project
}

// SetProject changes the client's default project scope.
func (c *Client) SetProject(project string) {
	c.project = project
}

// resolveProject returns the effective project, using the default if not specified.
func (c *Client) resolveProject(project string) string {
	if project != "" {
		return project
	}
	return c.project
}

// EpicOptions contains optional parameters for creating an epic.
type EpicOptions struct {
	Project     string // Override client default project
	Description string
	Priority    int // 1=high, 2=medium (default), 3=low
}

// CreateEpic creates a new epic and returns its ID.
// Epics are containers for grouping related tasks.
func (c *Client) CreateEpic(title string, opts *EpicOptions) (string, error) {
	if title == "" {
		return "", fmt.Errorf("epic title cannot be empty")
	}

	if opts == nil {
		opts = &EpicOptions{}
	}

	project := c.resolveProject(opts.Project)
	priority := opts.Priority
	if priority == 0 {
		priority = 2 // Default priority
	}

	now := time.Now()
	item := &Item{
		ID:          GenerateID(ItemTypeEpic),
		Project:     project,
		Type:        ItemTypeEpic,
		Title:       title,
		Description: opts.Description,
		Status:      StatusOpen,
		Priority:    priority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := c.db.CreateItem(item); err != nil {
		return "", fmt.Errorf("failed to create epic: %w", err)
	}

	return item.ID, nil
}

// TaskOptions contains optional parameters for creating a task.
type TaskOptions struct {
	Project     string   // Override client default project
	Description string
	Priority    int      // 1=high, 2=medium (default), 3=low
	ParentID    string   // Parent epic ID
	DependsOn   []string // IDs of tasks this depends on
}

// CreateTask creates a new task and returns its ID.
func (c *Client) CreateTask(title string, opts *TaskOptions) (string, error) {
	if title == "" {
		return "", fmt.Errorf("task title cannot be empty")
	}

	if opts == nil {
		opts = &TaskOptions{}
	}

	project := c.resolveProject(opts.Project)
	priority := opts.Priority
	if priority == 0 {
		priority = 2 // Default priority
	}

	now := time.Now()
	item := &Item{
		ID:          GenerateID(ItemTypeTask),
		Project:     project,
		Type:        ItemTypeTask,
		Title:       title,
		Description: opts.Description,
		Status:      StatusOpen,
		Priority:    priority,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Set parent if specified
	if opts.ParentID != "" {
		item.ParentID = &opts.ParentID
	}

	if err := c.db.CreateItem(item); err != nil {
		return "", fmt.Errorf("failed to create task: %w", err)
	}

	// Add dependencies
	for _, depID := range opts.DependsOn {
		if err := c.db.AddDep(item.ID, depID); err != nil {
			return item.ID, fmt.Errorf("task created but failed to add dependency %s: %w", depID, err)
		}
	}

	return item.ID, nil
}

// UpdateStatus changes an item's status.
// Valid statuses: open, in_progress, blocked, done, canceled
func (c *Client) UpdateStatus(id string, status Status) error {
	if !status.IsValid() {
		return fmt.Errorf("invalid status: %s (valid: open, in_progress, blocked, done, canceled)", status)
	}
	return c.db.UpdateStatus(id, status)
}

// Start marks an item as in progress.
func (c *Client) Start(id string) error {
	return c.UpdateStatus(id, StatusInProgress)
}

// Done marks an item as completed.
func (c *Client) Done(id string) error {
	return c.UpdateStatus(id, StatusDone)
}

// Block marks an item as blocked.
func (c *Client) Block(id string) error {
	return c.UpdateStatus(id, StatusBlocked)
}

// Cancel marks an item as canceled.
func (c *Client) Cancel(id string) error {
	return c.UpdateStatus(id, StatusCanceled)
}

// Reopen marks an item as open (useful for re-opening closed items).
func (c *Client) Reopen(id string) error {
	return c.UpdateStatus(id, StatusOpen)
}

// AddLog adds a timestamped log entry to an item.
func (c *Client) AddLog(itemID, message string) error {
	if message == "" {
		return fmt.Errorf("log message cannot be empty")
	}
	return c.db.AddLog(itemID, message)
}

// LearningOptions contains optional parameters for creating a learning.
type LearningOptions struct {
	Project  string   // Override client default project
	TaskID   string   // Optional task association
	Detail   string   // Extended details
	Files    []string // Related file paths
	Concepts []string // Associated concept names
}

// AddLearning creates a new learning entry and returns its ID.
// Learnings capture knowledge discovered during work.
func (c *Client) AddLearning(summary string, opts *LearningOptions) (string, error) {
	if summary == "" {
		return "", fmt.Errorf("learning summary cannot be empty")
	}

	if opts == nil {
		opts = &LearningOptions{}
	}

	project := c.resolveProject(opts.Project)
	if project == "" {
		return "", fmt.Errorf("project is required for learnings")
	}

	now := time.Now()
	learning := &Learning{
		ID:        GenerateLearningID(),
		Project:   project,
		CreatedAt: now,
		UpdatedAt: now,
		Summary:   summary,
		Detail:    opts.Detail,
		Files:     opts.Files,
		Status:    LearningStatusActive,
		Concepts:  opts.Concepts,
	}

	// Set task ID if specified
	if opts.TaskID != "" {
		learning.TaskID = &opts.TaskID
	}

	if err := c.db.CreateLearning(learning); err != nil {
		return "", fmt.Errorf("failed to create learning: %w", err)
	}

	return learning.ID, nil
}

// GetItem retrieves an item by ID.
func (c *Client) GetItem(id string) (*Item, error) {
	return c.db.GetItem(id)
}

// GetLearning retrieves a learning by ID.
func (c *Client) GetLearning(id string) (*Learning, error) {
	return c.db.GetLearning(id)
}

// ListReadyTasks returns tasks that are open and have no unmet dependencies.
// If project is empty, uses the client's default project.
// If both are empty, returns ready tasks from all projects.
func (c *Client) ListReadyTasks(project string) ([]Item, error) {
	return c.db.ReadyItems(c.resolveProject(project))
}

// GetStatus returns an aggregated status report for a project.
// If project is empty, uses the client's default project.
func (c *Client) GetStatus(project string) (*StatusReport, error) {
	return c.db.ProjectStatus(c.resolveProject(project))
}

// AppendDescription appends text to an item's description.
func (c *Client) AppendDescription(id string, text string) error {
	if text == "" {
		return fmt.Errorf("text cannot be empty")
	}
	return c.db.AppendDescription(id, text)
}

// SetDescription replaces an item's description entirely.
func (c *Client) SetDescription(id string, description string) error {
	return c.db.SetDescription(id, description)
}

// SetTitle replaces an item's title.
func (c *Client) SetTitle(id string, title string) error {
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}
	return c.db.SetTitle(id, title)
}

// AddDependency adds a dependency between items.
// The item with itemID will be blocked until dependsOnID is done.
func (c *Client) AddDependency(itemID, dependsOnID string) error {
	return c.db.AddDep(itemID, dependsOnID)
}

// GetDependencies returns the IDs of items that the given item depends on.
func (c *Client) GetDependencies(itemID string) ([]string, error) {
	return c.db.GetDeps(itemID)
}

// GetLogs retrieves all logs for an item.
func (c *Client) GetLogs(itemID string) ([]Log, error) {
	return c.db.GetLogs(itemID)
}

// SearchLearnings performs full-text search on learnings.
func (c *Client) SearchLearnings(query string, includeStale bool) ([]Learning, error) {
	project := c.project
	if project == "" {
		return nil, fmt.Errorf("project is required for searching learnings")
	}
	return c.db.SearchLearnings(project, query, includeStale)
}

// ListLearnings returns all learnings for the client's project.
func (c *Client) ListLearnings(includeStale bool) ([]Learning, error) {
	project := c.project
	if project == "" {
		return nil, fmt.Errorf("project is required for listing learnings")
	}
	return c.db.GetAllLearnings(project, includeStale)
}

// GetChildTasks returns all tasks under a given epic ID.
// The tasks are returned in priority order (high to low, then by creation time).
func (c *Client) GetChildTasks(epicID string) ([]Item, error) {
	return c.db.ListItemsFiltered(ListFilter{
		Parent: epicID,
		Type:   string(ItemTypeTask),
	})
}

// GetEpic retrieves an epic by ID. Returns an error if the item is not an epic.
func (c *Client) GetEpic(id string) (*Item, error) {
	item, err := c.db.GetItem(id)
	if err != nil {
		return nil, err
	}
	if item.Type != ItemTypeEpic {
		return nil, fmt.Errorf("item %s is not an epic (type: %s)", id, item.Type)
	}
	return item, nil
}

// FindInProgressEpic returns an in-progress epic for the client's project, if any.
// This is used to detect epics that can be resumed across sessions.
func (c *Client) FindInProgressEpic() (*Item, error) {
	status := StatusInProgress
	items, err := c.db.ListItemsFiltered(ListFilter{
		Project: c.project,
		Status:  &status,
		Type:    string(ItemTypeEpic),
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	// Return the first in-progress epic (most recently updated would be ideal,
	// but for now just return the first by priority/creation order)
	return &items[0], nil
}

// ComputeEpicProgress returns the number of completed and total tasks for an epic.
func (c *Client) ComputeEpicProgress(epicID string) (completed int, total int, err error) {
	tasks, err := c.GetChildTasks(epicID)
	if err != nil {
		return 0, 0, err
	}
	total = len(tasks)
	for _, t := range tasks {
		if t.Status == StatusDone {
			completed++
		}
	}
	return completed, total, nil
}

// UpdateEpicStatusIfComplete marks the epic as done if all child tasks are done.
// Returns true if the epic was marked as done, false otherwise.
func (c *Client) UpdateEpicStatusIfComplete(epicID string) (bool, error) {
	completed, total, err := c.ComputeEpicProgress(epicID)
	if err != nil {
		return false, err
	}
	if total > 0 && completed == total {
		if err := c.Done(epicID); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

// ListInProgressEpics returns all in-progress epics for the client's project.
// This is used to detect sessions that can be resumed.
func (c *Client) ListInProgressEpics() ([]Item, error) {
	status := StatusInProgress
	return c.db.ListItemsFiltered(ListFilter{
		Project: c.project,
		Status:  &status,
		Type:    string(ItemTypeEpic),
	})
}

// ListOpenOrInProgressEpics returns all epics that are either open or in-progress
// for the client's project. These represent potentially resumable sessions.
func (c *Client) ListOpenOrInProgressEpics() ([]Item, error) {
	// Get in-progress epics
	inProgress, err := c.ListInProgressEpics()
	if err != nil {
		return nil, err
	}

	// Get open epics
	status := StatusOpen
	open, err := c.db.ListItemsFiltered(ListFilter{
		Project: c.project,
		Status:  &status,
		Type:    string(ItemTypeEpic),
	})
	if err != nil {
		return nil, err
	}

	// Combine results
	return append(inProgress, open...), nil
}

// GetIncompleteTasks returns tasks under an epic that are not yet done.
// This is useful for determining what work remains in a resumable session.
func (c *Client) GetIncompleteTasks(epicID string) ([]Item, error) {
	tasks, err := c.GetChildTasks(epicID)
	if err != nil {
		return nil, err
	}

	var incomplete []Item
	for _, task := range tasks {
		if task.Status != StatusDone && task.Status != StatusCanceled {
			incomplete = append(incomplete, task)
		}
	}
	return incomplete, nil
}
