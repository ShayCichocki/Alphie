// Package merge provides git merge operations with checkpoints for rollback.
package merge

import (
	"fmt"
	"sync"
	"time"

	"github.com/ShayCichocki/alphie/internal/git"
)

// CheckpointStatus represents the status of a checkpoint.
type CheckpointStatus int

const (
	// CheckpointGood indicates a successful merge at this checkpoint.
	CheckpointGood CheckpointStatus = iota
	// CheckpointBad indicates a failed merge at this checkpoint.
	CheckpointBad
	// CheckpointUnknown indicates the checkpoint status is not yet determined.
	CheckpointUnknown
)

// Checkpoint represents a git tag marking a point in merge history.
type Checkpoint struct {
	AgentID   string
	TaskID    string
	CommitSHA string
	TagName   string
	CreatedAt time.Time
	Status    CheckpointStatus
}

// CheckpointManager manages git checkpoints for transactional merge rollback.
// It creates lightweight git tags before each merge, tracks their status,
// and provides rollback functionality.
type CheckpointManager struct {
	sessionID   string
	repo        git.Runner
	mu          sync.RWMutex
	checkpoints map[string]*Checkpoint // agentID -> Checkpoint
}

// NewCheckpointManager creates a new checkpoint manager for a session.
func NewCheckpointManager(sessionID string, repo git.Runner) *CheckpointManager {
	return &CheckpointManager{
		sessionID:   sessionID,
		repo:        repo,
		checkpoints: make(map[string]*Checkpoint),
	}
}

// CreateCheckpoint creates a checkpoint before merging an agent's work.
// It creates a lightweight git tag at the current HEAD of the session branch.
func (cm *CheckpointManager) CreateCheckpoint(agentID string, taskID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Get current HEAD SHA
	output, err := cm.repo.Run("rev-parse", "HEAD")
	if err != nil {
		return fmt.Errorf("get HEAD sha: %w", err)
	}
	commitSHA := output

	// Create tag name
	tagName := fmt.Sprintf("alphie-checkpoint-%s-%s", cm.sessionID, agentID)

	// Create lightweight tag
	if _, err := cm.repo.Run("tag", tagName, commitSHA); err != nil {
		return fmt.Errorf("create checkpoint tag: %w", err)
	}

	// Record checkpoint
	checkpoint := &Checkpoint{
		AgentID:   agentID,
		TaskID:    taskID,
		CommitSHA: commitSHA,
		TagName:   tagName,
		CreatedAt: time.Now(),
		Status:    CheckpointUnknown,
	}
	cm.checkpoints[agentID] = checkpoint

	return nil
}

// MarkGood marks a checkpoint as successful after merge completes.
func (cm *CheckpointManager) MarkGood(agentID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	checkpoint, exists := cm.checkpoints[agentID]
	if !exists {
		return fmt.Errorf("checkpoint not found for agent %s", agentID)
	}

	checkpoint.Status = CheckpointGood
	return nil
}

// MarkBad marks a checkpoint as failed after merge fails.
func (cm *CheckpointManager) MarkBad(agentID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	checkpoint, exists := cm.checkpoints[agentID]
	if !exists {
		return fmt.Errorf("checkpoint not found for agent %s", agentID)
	}

	checkpoint.Status = CheckpointBad
	return nil
}

// GetCheckpoint retrieves a checkpoint by agent ID.
func (cm *CheckpointManager) GetCheckpoint(agentID string) (*Checkpoint, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	checkpoint, exists := cm.checkpoints[agentID]
	if !exists {
		return nil, fmt.Errorf("checkpoint not found for agent %s", agentID)
	}

	// Return a copy to avoid race conditions
	cp := *checkpoint
	return &cp, nil
}

// GetLastGoodCheckpoint returns the most recent good checkpoint.
// Returns nil if no good checkpoints exist.
func (cm *CheckpointManager) GetLastGoodCheckpoint() *Checkpoint {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	var lastGood *Checkpoint
	for _, cp := range cm.checkpoints {
		if cp.Status == CheckpointGood {
			if lastGood == nil || cp.CreatedAt.After(lastGood.CreatedAt) {
				lastGood = cp
			}
		}
	}

	if lastGood == nil {
		return nil
	}

	// Return a copy
	cp := *lastGood
	return &cp
}

// GetAllCheckpoints returns all checkpoints in chronological order.
func (cm *CheckpointManager) GetAllCheckpoints() []*Checkpoint {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	checkpoints := make([]*Checkpoint, 0, len(cm.checkpoints))
	for _, cp := range cm.checkpoints {
		cpCopy := *cp
		checkpoints = append(checkpoints, &cpCopy)
	}

	// Sort by creation time
	for i := 0; i < len(checkpoints)-1; i++ {
		for j := i + 1; j < len(checkpoints); j++ {
			if checkpoints[i].CreatedAt.After(checkpoints[j].CreatedAt) {
				checkpoints[i], checkpoints[j] = checkpoints[j], checkpoints[i]
			}
		}
	}

	return checkpoints
}

// ListGoodCheckpoints returns all good checkpoints in chronological order.
func (cm *CheckpointManager) ListGoodCheckpoints() []*Checkpoint {
	allCheckpoints := cm.GetAllCheckpoints()
	goodCheckpoints := make([]*Checkpoint, 0)

	for _, cp := range allCheckpoints {
		if cp.Status == CheckpointGood {
			goodCheckpoints = append(goodCheckpoints, cp)
		}
	}

	return goodCheckpoints
}

// Cleanup removes all checkpoint tags for this session.
// Should be called on successful session completion.
func (cm *CheckpointManager) Cleanup() error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	var errs []error
	for _, checkpoint := range cm.checkpoints {
		if _, err := cm.repo.Run("tag", "-d", checkpoint.TagName); err != nil {
			errs = append(errs, fmt.Errorf("delete tag %s: %w", checkpoint.TagName, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	return nil
}

// DeleteCheckpoint removes a specific checkpoint tag.
func (cm *CheckpointManager) DeleteCheckpoint(agentID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	checkpoint, exists := cm.checkpoints[agentID]
	if !exists {
		return fmt.Errorf("checkpoint not found for agent %s", agentID)
	}

	if _, err := cm.repo.Run("tag", "-d", checkpoint.TagName); err != nil {
		return fmt.Errorf("delete checkpoint tag: %w", err)
	}

	delete(cm.checkpoints, agentID)
	return nil
}

// StatusString returns a human-readable status string.
func (s CheckpointStatus) String() string {
	switch s {
	case CheckpointGood:
		return "good"
	case CheckpointBad:
		return "bad"
	case CheckpointUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}
