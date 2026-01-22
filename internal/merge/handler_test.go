package merge

import (
	"testing"
)

func TestResult_Success(t *testing.T) {
	result := Result{
		Success:      true,
		Diff:         "diff content",
		ChangedFiles: []string{"file1.go", "file2.go"},
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.NeedsSemanticMerge {
		t.Error("expected NeedsSemanticMerge to be false")
	}
	if result.Error != nil {
		t.Errorf("expected nil error, got %v", result.Error)
	}
	if len(result.ConflictFiles) != 0 {
		t.Errorf("expected empty ConflictFiles, got %v", result.ConflictFiles)
	}
	if len(result.ChangedFiles) != 2 {
		t.Errorf("expected 2 changed files, got %d", len(result.ChangedFiles))
	}
}

func TestResult_Conflict(t *testing.T) {
	result := Result{
		Success:       false,
		ConflictFiles: []string{"conflict1.go", "conflict2.go"},
		Error:         nil,
	}

	if result.Success {
		t.Error("expected Success to be false")
	}
	if len(result.ConflictFiles) != 2 {
		t.Errorf("expected 2 conflict files, got %d", len(result.ConflictFiles))
	}
	if result.ConflictFiles[0] != "conflict1.go" {
		t.Errorf("expected first conflict to be 'conflict1.go', got %q", result.ConflictFiles[0])
	}
}

func TestResult_NeedsSemanticMerge(t *testing.T) {
	result := Result{
		Success:            false,
		ConflictFiles:      []string{"complex.go"},
		NeedsSemanticMerge: true,
		Error:              nil,
	}

	if result.Success {
		t.Error("expected Success to be false")
	}
	if !result.NeedsSemanticMerge {
		t.Error("expected NeedsSemanticMerge to be true")
	}
}

func TestNewHandler(t *testing.T) {
	handler := NewHandler("session-abc", "/tmp/repo")

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.SessionBranch() != "session-abc" {
		t.Errorf("expected sessionBranch 'session-abc', got %q", handler.SessionBranch())
	}
	if handler.RepoPath() != "/tmp/repo" {
		t.Errorf("expected repoPath '/tmp/repo', got %q", handler.RepoPath())
	}
}

func TestResult_DiffAndChangedFiles(t *testing.T) {
	result := Result{
		Success: true,
		Diff: `diff --git a/file1.go b/file1.go
--- a/file1.go
+++ b/file1.go
@@ -1 +1 @@
-old
+new`,
		ChangedFiles: []string{"file1.go"},
	}

	if result.Diff == "" {
		t.Error("expected non-empty diff")
	}
	if len(result.ChangedFiles) != 1 {
		t.Errorf("expected 1 changed file, got %d", len(result.ChangedFiles))
	}
	if result.ChangedFiles[0] != "file1.go" {
		t.Errorf("expected changed file 'file1.go', got %q", result.ChangedFiles[0])
	}
}

func TestResult_MultipleConflictFiles(t *testing.T) {
	conflicts := []string{
		"internal/auth/login.go",
		"internal/auth/session.go",
		"pkg/models/user.go",
	}

	result := Result{
		Success:            false,
		ConflictFiles:      conflicts,
		NeedsSemanticMerge: true,
	}

	if len(result.ConflictFiles) != 3 {
		t.Errorf("expected 3 conflict files, got %d", len(result.ConflictFiles))
	}

	for i, expected := range conflicts {
		if result.ConflictFiles[i] != expected {
			t.Errorf("conflict[%d]: expected %q, got %q", i, expected, result.ConflictFiles[i])
		}
	}
}

func TestHandler_EmptyBranchName(t *testing.T) {
	handler := NewHandler("", "/tmp/repo")

	if handler.SessionBranch() != "" {
		t.Errorf("expected empty sessionBranch, got %q", handler.SessionBranch())
	}
}

func TestResult_ZeroValue(t *testing.T) {
	var result Result

	if result.Success {
		t.Error("expected zero value Success to be false")
	}
	if result.NeedsSemanticMerge {
		t.Error("expected zero value NeedsSemanticMerge to be false")
	}
	if result.Error != nil {
		t.Error("expected zero value Error to be nil")
	}
	if result.ConflictFiles != nil {
		t.Error("expected zero value ConflictFiles to be nil")
	}
	if result.ChangedFiles != nil {
		t.Error("expected zero value ChangedFiles to be nil")
	}
	if result.Diff != "" {
		t.Error("expected zero value Diff to be empty")
	}
}
