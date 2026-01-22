// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/ShayCichocki/alphie/internal/prog"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// ProgCoordinator manages cross-session task tracking via the prog system.
// It handles epic and task creation, status synchronization, and resumption.
type ProgCoordinator struct {
	// client is the prog tracker for cross-session persistence.
	client prog.ProgTracker
	// epicID is the prog epic ID for the current request.
	epicID string
	// taskIDs maps internal task IDs to prog task IDs.
	taskIDs map[string]string
	// emitter sends orchestrator events.
	emitter *EventEmitter
	// originalTaskID is the task ID from the TUI for event linking.
	originalTaskID string
	// tier is the agent tier for task creation.
	tier models.Tier
}

// NewProgCoordinator creates a new ProgCoordinator.
// If client is nil, returns a no-op coordinator that skips all operations.
func NewProgCoordinator(client prog.ProgTracker, emitter *EventEmitter, originalTaskID string, tier models.Tier, resumeEpicID string) *ProgCoordinator {
	return &ProgCoordinator{
		client:         client,
		epicID:         resumeEpicID,
		taskIDs:        make(map[string]string),
		emitter:        emitter,
		originalTaskID: originalTaskID,
		tier:           tier,
	}
}

// IsConfigured returns true if the prog client is configured.
func (p *ProgCoordinator) IsConfigured() bool {
	return p.client != nil
}

// Client returns the underlying prog tracker for external use.
// Returns nil if prog is not configured.
func (p *ProgCoordinator) Client() prog.ProgTracker {
	return p.client
}

// EpicID returns the prog epic ID for the current request.
func (p *ProgCoordinator) EpicID() string {
	return p.epicID
}

// HasResumeEpic returns true if resuming from an existing epic.
func (p *ProgCoordinator) HasResumeEpic() bool {
	return p.epicID != ""
}

// CreateEpicAndTasks creates a prog epic for the request and prog tasks for each subtask.
// This enables cross-session task tracking and continuity.
func (p *ProgCoordinator) CreateEpicAndTasks(request string, tasks []*models.Task) error {
	if p.client == nil {
		return nil // No-op if prog client not configured
	}

	// Truncate request for epic title (prog titles should be concise)
	epicTitle := request
	if len(epicTitle) > 100 {
		epicTitle = epicTitle[:97] + "..."
	}

	// Create the epic
	epicID, err := p.client.CreateEpic(epicTitle, &prog.EpicOptions{
		Description: request,
	})
	if err != nil {
		return fmt.Errorf("create epic: %w", err)
	}

	log.Printf("[orchestrator] created prog epic %s for request", epicID)

	// Store epic ID for later reference
	p.epicID = epicID

	// Emit epic created event so TUI can track the parent
	p.emitter.Emit(OrchestratorEvent{
		Type:           EventEpicCreated,
		TaskID:         epicID,
		TaskTitle:      epicTitle,
		Message:        fmt.Sprintf("Created epic: %s", epicTitle),
		Timestamp:      time.Now(),
		OriginalTaskID: p.originalTaskID,
	})

	// Map internal task IDs to prog task IDs for dependency resolution
	internalToProgID := make(map[string]string)

	// First pass: create all tasks (without dependencies)
	for _, task := range tasks {
		// Set ParentID on the internal task for event propagation
		task.ParentID = epicID

		progTaskID, err := p.client.CreateTask(task.Title, &prog.TaskOptions{
			Description: task.Description,
			ParentID:    epicID,
		})
		if err != nil {
			return fmt.Errorf("create task %q: %w", task.Title, err)
		}
		internalToProgID[task.ID] = progTaskID
	}

	// Second pass: add dependencies
	for _, task := range tasks {
		if len(task.DependsOn) == 0 {
			continue
		}

		progTaskID := internalToProgID[task.ID]
		for _, depID := range task.DependsOn {
			progDepID, ok := internalToProgID[depID]
			if !ok {
				log.Printf("[orchestrator] warning: prog dependency %s not found for task %s", depID, task.ID)
				continue
			}
			if err := p.client.AddDependency(progTaskID, progDepID); err != nil {
				return fmt.Errorf("add dependency %s -> %s: %w", progTaskID, progDepID, err)
			}
		}
	}

	// Store the mapping for later status updates
	p.taskIDs = internalToProgID

	log.Printf("[orchestrator] created %d prog tasks under epic %s", len(tasks), epicID)
	return nil
}

// TaskID returns the prog task ID for an internal task ID.
// Returns empty string if no mapping exists or prog is not configured.
func (p *ProgCoordinator) TaskID(internalID string) string {
	if p.taskIDs == nil {
		return ""
	}
	return p.taskIDs[internalID]
}

// StartTask marks a prog task as in_progress and logs the start event.
func (p *ProgCoordinator) StartTask(internalID string) {
	if p.client == nil {
		return
	}
	progID := p.TaskID(internalID)
	if progID == "" {
		return
	}

	if err := p.client.Start(progID); err != nil {
		log.Printf("[orchestrator] warning: failed to start prog task %s: %v", progID, err)
		return
	}
	if err := p.client.AddLog(progID, "Task execution started"); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task start %s: %v", progID, err)
	}
}

// LogTask adds a log entry to a prog task.
func (p *ProgCoordinator) LogTask(internalID, message string) {
	if p.client == nil {
		return
	}
	progID := p.TaskID(internalID)
	if progID == "" {
		return
	}

	if err := p.client.AddLog(progID, message); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task %s: %v", progID, err)
	}
}

// CompleteTask marks a prog task as done and logs the completion.
func (p *ProgCoordinator) CompleteTask(internalID string) {
	if p.client == nil {
		return
	}
	progID := p.TaskID(internalID)
	if progID == "" {
		return
	}

	if err := p.client.AddLog(progID, "Task completed successfully"); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task completion %s: %v", progID, err)
	}
	if err := p.client.Done(progID); err != nil {
		log.Printf("[orchestrator] warning: failed to complete prog task %s: %v", progID, err)
	}
}

// BlockTask marks a prog task as blocked and logs the failure reason.
func (p *ProgCoordinator) BlockTask(internalID, reason string) {
	if p.client == nil {
		return
	}
	progID := p.TaskID(internalID)
	if progID == "" {
		return
	}

	if err := p.client.AddLog(progID, fmt.Sprintf("Task failed: %s", reason)); err != nil {
		log.Printf("[orchestrator] warning: failed to log prog task failure %s: %v", progID, err)
	}
	if err := p.client.Block(progID); err != nil {
		log.Printf("[orchestrator] warning: failed to block prog task %s: %v", progID, err)
	}
}

// LoadTasksFromEpic loads tasks from an existing prog epic for resumption.
// Completed tasks are loaded with status Done so they will be skipped.
// In-progress tasks are reset to Pending for re-execution.
func (p *ProgCoordinator) LoadTasksFromEpic(ctx context.Context) ([]*models.Task, error) {
	if p.client == nil {
		return nil, fmt.Errorf("prog client not configured")
	}

	// Verify epic exists
	epic, err := p.client.GetEpic(p.epicID)
	if err != nil {
		return nil, fmt.Errorf("get epic: %w", err)
	}

	// Mark epic as in-progress if it's open
	if epic.Status == prog.StatusOpen {
		if err := p.client.Start(p.epicID); err != nil {
			log.Printf("[orchestrator] warning: failed to mark epic as in-progress: %v", err)
		}
	}

	// Get child tasks
	progTasks, err := p.client.GetChildTasks(p.epicID)
	if err != nil {
		return nil, fmt.Errorf("get child tasks: %w", err)
	}

	if len(progTasks) == 0 {
		return nil, fmt.Errorf("epic %s has no tasks", p.epicID)
	}

	// Convert prog tasks to internal tasks
	tasks := make([]*models.Task, 0, len(progTasks))
	for _, pt := range progTasks {
		// Map prog task ID to internal task ID for status sync
		internalID := uuid.New().String()[:8]
		p.taskIDs[internalID] = pt.ID

		// Convert status
		var status models.TaskStatus
		switch pt.Status {
		case prog.StatusDone:
			status = models.TaskStatusDone
		case prog.StatusCanceled:
			// Skip canceled tasks entirely
			continue
		default:
			// Open, in_progress, blocked all become pending for (re-)execution
			status = models.TaskStatusPending
		}

		task := &models.Task{
			ID:          internalID,
			Title:       pt.Title,
			Description: pt.Description,
			Status:      status,
			Tier:        p.tier,
			CreatedAt:   pt.CreatedAt,
		}
		// Set ParentID if the prog task has one
		if pt.ParentID != nil {
			task.ParentID = *pt.ParentID
		}

		tasks = append(tasks, task)
	}

	// Note: Dependencies from prog are not loaded here.
	// When resuming, tasks that were in-progress may have had their
	// dependencies already completed, so we execute them independently.
	// For more sophisticated dependency handling, we would need to map
	// prog dep IDs to internal task IDs.

	log.Printf("[orchestrator] loaded %d tasks from epic (skipped canceled, %d already done)",
		len(tasks), countDoneTasks(tasks))

	return tasks, nil
}

// countDoneTasks counts tasks with Done status.
func countDoneTasks(tasks []*models.Task) int {
	count := 0
	for _, t := range tasks {
		if t.Status == models.TaskStatusDone {
			count++
		}
	}
	return count
}
