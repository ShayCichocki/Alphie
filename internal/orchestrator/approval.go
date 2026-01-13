// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/shayc/alphie/pkg/models"
)

// ApprovalRequest represents a request for human approval of a task's output.
// This is sent from the orchestrator to the TUI when an Architect tier task
// completes and requires human review before merge.
type ApprovalRequest struct {
	// TaskID is the ID of the task requiring approval.
	TaskID string
	// AgentID is the ID of the agent that completed the task.
	AgentID string
	// Diff is the diff content to be reviewed.
	Diff string
	// TaskDescription describes what the task accomplished.
	TaskDescription string
	// BaseCommit is the git commit hash that was the base for this work.
	BaseCommit string
}

// ApprovalResponse represents the human's decision on an approval request.
type ApprovalResponse struct {
	// TaskID is the ID of the task being approved/rejected.
	TaskID string
	// Approved indicates whether the changes were approved.
	Approved bool
	// Reason provides context for rejections.
	Reason string
}

// Approval represents an approval bound to a specific state snapshot.
// An approval is tied to the combination of task ID, base commit, and diff content.
// If any of these change, the approval becomes invalid and re-approval is required.
type Approval struct {
	// TaskID is the ID of the approved task.
	TaskID string
	// BaseCommit is the git commit hash that was the base when approval was granted.
	BaseCommit string
	// DiffHash is the SHA256 hash of the diff content at approval time.
	DiffHash string
	// ApprovedAt is when the approval was granted.
	ApprovedAt time.Time
	// ApprovedBy indicates who granted approval ("user" or "auto").
	ApprovedBy string
}

// ApprovalManager tracks approval state for tasks and validates approval validity.
// It ensures that approvals are invalidated when the underlying state changes,
// requiring re-approval after code changes.
type ApprovalManager struct {
	// approvals maps task IDs to their approval state.
	approvals map[string]*Approval
	// pendingRequests maps task IDs to channels waiting for approval responses.
	pendingRequests map[string]chan ApprovalResponse
	// requestCh is used to send approval requests to the TUI.
	requestCh chan ApprovalRequest
	// mu protects concurrent access to the approvals map.
	mu sync.RWMutex
}

// NewApprovalManager creates a new ApprovalManager instance.
func NewApprovalManager() *ApprovalManager {
	return &ApprovalManager{
		approvals:       make(map[string]*Approval),
		pendingRequests: make(map[string]chan ApprovalResponse),
		requestCh:       make(chan ApprovalRequest, 10),
	}
}

// RequestCh returns a read-only channel for receiving approval requests.
// The TUI should listen on this channel to receive approval requests.
func (m *ApprovalManager) RequestCh() <-chan ApprovalRequest {
	return m.requestCh
}

// WaitForApproval blocks until the human approves or rejects the task.
// It sends an ApprovalRequest to the TUI and waits for a response.
// Returns the approval response or an error if the context is cancelled.
func (m *ApprovalManager) WaitForApproval(ctx context.Context, req ApprovalRequest) (ApprovalResponse, error) {
	// Create response channel for this task
	responseCh := make(chan ApprovalResponse, 1)

	m.mu.Lock()
	m.pendingRequests[req.TaskID] = responseCh
	m.mu.Unlock()

	// Cleanup on exit
	defer func() {
		m.mu.Lock()
		delete(m.pendingRequests, req.TaskID)
		m.mu.Unlock()
	}()

	// Send request to TUI
	select {
	case m.requestCh <- req:
	case <-ctx.Done():
		return ApprovalResponse{}, ctx.Err()
	}

	// Wait for response
	select {
	case resp := <-responseCh:
		return resp, nil
	case <-ctx.Done():
		return ApprovalResponse{}, ctx.Err()
	}
}

// SubmitResponse submits a human's approval response for a pending request.
// This is called by the TUI when the user approves or rejects.
func (m *ApprovalManager) SubmitResponse(resp ApprovalResponse) {
	m.mu.RLock()
	ch, exists := m.pendingRequests[resp.TaskID]
	m.mu.RUnlock()

	if exists {
		select {
		case ch <- resp:
		default:
			// Channel full or closed, response already submitted
		}
	}
}

// HasPendingRequest returns true if there is a pending approval request for the task.
func (m *ApprovalManager) HasPendingRequest(taskID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.pendingRequests[taskID]
	return exists
}

// Create creates a new approval for the given task and diff content.
// The approval is bound to the task ID, the base commit from the task context,
// and a hash of the diff content. The approvedBy parameter should be "user"
// for manual approval or "auto" for automated approval.
func (m *ApprovalManager) Create(task *models.Task, baseCommit, diff, approvedBy string) *Approval {
	approval := &Approval{
		TaskID:     task.ID,
		BaseCommit: baseCommit,
		DiffHash:   m.GetDiffHash(diff),
		ApprovedAt: time.Now(),
		ApprovedBy: approvedBy,
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.approvals[task.ID] = approval

	return approval
}

// IsValid checks if the approval for the given task is still valid.
// An approval is valid only if:
//   - An approval exists for the task
//   - The current base commit matches the approved base commit
//   - The current diff hash matches the approved diff hash
//
// If any of these conditions fail, the approval is considered expired
// and re-approval is required.
func (m *ApprovalManager) IsValid(taskID, currentBase, currentDiff string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	approval, exists := m.approvals[taskID]
	if !exists {
		return false
	}

	// Check if base commit matches
	if approval.BaseCommit != currentBase {
		return false
	}

	// Check if diff hash matches
	currentDiffHash := m.GetDiffHash(currentDiff)
	if approval.DiffHash != currentDiffHash {
		return false
	}

	return true
}

// Expire removes the approval for the given task, requiring re-approval.
func (m *ApprovalManager) Expire(taskID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.approvals, taskID)
}

// Get returns the approval for the given task, or nil if no approval exists.
func (m *ApprovalManager) Get(taskID string) *Approval {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.approvals[taskID]
}

// GetDiffHash computes the SHA256 hash of the diff content.
// This is used to detect changes in the diff that would invalidate an approval.
func (m *ApprovalManager) GetDiffHash(diff string) string {
	hash := sha256.Sum256([]byte(diff))
	return hex.EncodeToString(hash[:])
}
