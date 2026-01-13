package orchestrator

import (
	"testing"
	"time"

	"github.com/shayc/alphie/pkg/models"
)

func TestApprovalManager_Create(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}

	approval := manager.Create(task, "abc123", "diff content", "user")

	if approval == nil {
		t.Fatal("expected non-nil approval")
	}
	if approval.TaskID != "task-001" {
		t.Errorf("expected TaskID 'task-001', got %q", approval.TaskID)
	}
	if approval.BaseCommit != "abc123" {
		t.Errorf("expected BaseCommit 'abc123', got %q", approval.BaseCommit)
	}
	if approval.ApprovedBy != "user" {
		t.Errorf("expected ApprovedBy 'user', got %q", approval.ApprovedBy)
	}
	if approval.DiffHash == "" {
		t.Error("expected non-empty DiffHash")
	}
	if approval.ApprovedAt.IsZero() {
		t.Error("expected ApprovedAt to be set")
	}
}

func TestApprovalManager_SnapshotBinding(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}
	baseCommit := "abc123def"
	diffContent := "diff --git a/file.go b/file.go\n-old\n+new"

	// Create approval
	manager.Create(task, baseCommit, diffContent, "user")

	// Valid with same base commit and diff
	if !manager.IsValid("task-001", baseCommit, diffContent) {
		t.Error("expected approval to be valid with same snapshot")
	}
}

func TestApprovalManager_BaseCommitChange_InvalidatesApproval(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}
	diffContent := "diff content"

	// Create approval with original base commit
	manager.Create(task, "original-commit", diffContent, "user")

	// Should be valid with original commit
	if !manager.IsValid("task-001", "original-commit", diffContent) {
		t.Error("expected approval to be valid with original commit")
	}

	// Should be invalid with different base commit
	if manager.IsValid("task-001", "new-commit", diffContent) {
		t.Error("expected approval to be invalid after base commit change")
	}
}

func TestApprovalManager_DiffChange_InvalidatesApproval(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}
	baseCommit := "abc123"
	originalDiff := "original diff content"
	modifiedDiff := "modified diff content"

	// Create approval with original diff
	manager.Create(task, baseCommit, originalDiff, "user")

	// Should be valid with original diff
	if !manager.IsValid("task-001", baseCommit, originalDiff) {
		t.Error("expected approval to be valid with original diff")
	}

	// Should be invalid with different diff (even same base commit)
	if manager.IsValid("task-001", baseCommit, modifiedDiff) {
		t.Error("expected approval to be invalid after diff change")
	}
}

func TestApprovalManager_NonExistentTask(t *testing.T) {
	manager := NewApprovalManager()

	// No approval exists for this task
	if manager.IsValid("nonexistent-task", "any-commit", "any-diff") {
		t.Error("expected IsValid to return false for nonexistent task")
	}
}

func TestApprovalManager_Get(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}

	// Initially nil
	if manager.Get("task-001") != nil {
		t.Error("expected nil before creating approval")
	}

	// Create approval
	manager.Create(task, "commit", "diff", "user")

	// Now should return approval
	approval := manager.Get("task-001")
	if approval == nil {
		t.Fatal("expected non-nil approval after create")
	}
	if approval.TaskID != "task-001" {
		t.Errorf("expected TaskID 'task-001', got %q", approval.TaskID)
	}
}

func TestApprovalManager_Expire(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}
	baseCommit := "abc123"
	diff := "diff content"

	// Create approval
	manager.Create(task, baseCommit, diff, "user")

	// Verify it's valid
	if !manager.IsValid("task-001", baseCommit, diff) {
		t.Error("expected approval to be valid before expiration")
	}

	// Expire the approval
	manager.Expire("task-001")

	// Should no longer be valid
	if manager.IsValid("task-001", baseCommit, diff) {
		t.Error("expected approval to be invalid after expiration")
	}

	// Get should return nil
	if manager.Get("task-001") != nil {
		t.Error("expected Get to return nil after expiration")
	}
}

func TestApprovalManager_GetDiffHash(t *testing.T) {
	manager := NewApprovalManager()

	tests := []struct {
		name     string
		diff1    string
		diff2    string
		sameHash bool
	}{
		{
			name:     "identical diffs have same hash",
			diff1:    "diff content",
			diff2:    "diff content",
			sameHash: true,
		},
		{
			name:     "different diffs have different hash",
			diff1:    "diff content A",
			diff2:    "diff content B",
			sameHash: false,
		},
		{
			name:     "empty diff has hash",
			diff1:    "",
			diff2:    "",
			sameHash: true,
		},
		{
			name:     "whitespace matters",
			diff1:    "diff content",
			diff2:    "diff content ",
			sameHash: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hash1 := manager.GetDiffHash(tc.diff1)
			hash2 := manager.GetDiffHash(tc.diff2)

			if tc.sameHash && hash1 != hash2 {
				t.Errorf("expected same hash, got %q and %q", hash1, hash2)
			}
			if !tc.sameHash && hash1 == hash2 {
				t.Errorf("expected different hashes, got same: %q", hash1)
			}
		})
	}
}

func TestApprovalManager_ApprovedByValues(t *testing.T) {
	manager := NewApprovalManager()

	tests := []struct {
		name       string
		approvedBy string
	}{
		{
			name:       "user approval",
			approvedBy: "user",
		},
		{
			name:       "auto approval",
			approvedBy: "auto",
		},
		{
			name:       "custom approver",
			approvedBy: "system-bot",
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := &models.Task{ID: "task-" + string(rune('A'+i))}
			approval := manager.Create(task, "commit", "diff", tc.approvedBy)

			if approval.ApprovedBy != tc.approvedBy {
				t.Errorf("expected ApprovedBy %q, got %q", tc.approvedBy, approval.ApprovedBy)
			}
		})
	}
}

func TestApprovalManager_MultipleTasks(t *testing.T) {
	manager := NewApprovalManager()

	task1 := &models.Task{ID: "task-001"}
	task2 := &models.Task{ID: "task-002"}

	// Create approvals for different tasks
	manager.Create(task1, "commit1", "diff1", "user")
	manager.Create(task2, "commit2", "diff2", "auto")

	// Each task should have its own approval
	if !manager.IsValid("task-001", "commit1", "diff1") {
		t.Error("expected task-001 approval to be valid")
	}
	if !manager.IsValid("task-002", "commit2", "diff2") {
		t.Error("expected task-002 approval to be valid")
	}

	// Cross-checking should fail
	if manager.IsValid("task-001", "commit2", "diff2") {
		t.Error("expected task-001 to be invalid with task-002's snapshot")
	}

	// Expiring one should not affect the other
	manager.Expire("task-001")

	if manager.IsValid("task-001", "commit1", "diff1") {
		t.Error("expected task-001 to be invalid after expiration")
	}
	if !manager.IsValid("task-002", "commit2", "diff2") {
		t.Error("expected task-002 to still be valid after task-001 expiration")
	}
}

func TestApprovalManager_ApprovalTimestamp(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}

	before := time.Now()
	approval := manager.Create(task, "commit", "diff", "user")
	after := time.Now()

	if approval.ApprovedAt.Before(before) {
		t.Error("ApprovedAt should not be before creation time")
	}
	if approval.ApprovedAt.After(after) {
		t.Error("ApprovedAt should not be after creation time")
	}
}

func TestNewApprovalManager(t *testing.T) {
	manager := NewApprovalManager()

	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
	if manager.approvals == nil {
		t.Error("expected approvals map to be initialized")
	}
}

func TestApprovalManager_OverwriteExistingApproval(t *testing.T) {
	manager := NewApprovalManager()
	task := &models.Task{ID: "task-001"}

	// Create first approval
	approval1 := manager.Create(task, "commit1", "diff1", "user")

	// Create second approval for same task (should overwrite)
	approval2 := manager.Create(task, "commit2", "diff2", "auto")

	// Old approval should no longer be valid
	if manager.IsValid("task-001", "commit1", "diff1") {
		t.Error("expected old approval to be invalid after overwrite")
	}

	// New approval should be valid
	if !manager.IsValid("task-001", "commit2", "diff2") {
		t.Error("expected new approval to be valid")
	}

	// Verify different hashes
	if approval1.DiffHash == approval2.DiffHash {
		t.Error("expected different diff hashes for different diffs")
	}
}
