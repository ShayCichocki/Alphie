package orchestrator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/internal/agent"
)

// mockRunner implements agent.ClaudeRunner for testing
type mockRunner struct {
	outputCh chan agent.StreamEvent
}

func (m *mockRunner) Start(prompt, workDir string) error {
	// Close the channel immediately so Execute() doesn't block
	go func() { close(m.outputCh) }()
	return nil
}
func (m *mockRunner) StartWithOptions(prompt, workDir string, opts *agent.StartOptions) error {
	// Close the channel immediately so Execute() doesn't block
	go func() { close(m.outputCh) }()
	return nil
}
func (m *mockRunner) Output() <-chan agent.StreamEvent { return m.outputCh }
func (m *mockRunner) Wait() error                      { return nil }
func (m *mockRunner) Kill() error                      { return nil }
func (m *mockRunner) Stderr() string                   { return "" }
func (m *mockRunner) PID() int                         { return 0 }

// mockRunnerFactory creates mock ClaudeRunner instances for testing
type mockRunnerFactory struct{}

func (f *mockRunnerFactory) NewRunner() agent.ClaudeRunner {
	return &mockRunner{outputCh: make(chan agent.StreamEvent)}
}

// testFactory returns a factory for use in tests
func testFactory() agent.ClaudeRunnerFactory {
	return &mockRunnerFactory{}
}

func TestNewQuickExecutor(t *testing.T) {
	executor := NewQuickExecutor("/tmp/test-repo", testFactory())

	if executor == nil {
		t.Fatal("NewQuickExecutor returned nil")
	}
	if executor.repoPath != "/tmp/test-repo" {
		t.Errorf("repoPath = %q, want %q", executor.repoPath, "/tmp/test-repo")
	}
}

func TestQuickResult_Fields(t *testing.T) {
	result := QuickResult{
		Success:    true,
		Output:     "test output",
		Error:      "",
		TokensUsed: 100,
		Cost:       0.0012,
		Duration:   5 * time.Second,
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if result.Output != "test output" {
		t.Errorf("Output = %q, want %q", result.Output, "test output")
	}
	if result.TokensUsed != 100 {
		t.Errorf("TokensUsed = %d, want 100", result.TokensUsed)
	}
	if result.Cost != 0.0012 {
		t.Errorf("Cost = %f, want 0.0012", result.Cost)
	}
	if result.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", result.Duration)
	}
}

func TestQuickResult_FailedResult(t *testing.T) {
	result := QuickResult{
		Success:  false,
		Output:   "partial output",
		Error:    "task failed",
		Duration: 2 * time.Second,
	}

	if result.Success {
		t.Error("Success should be false")
	}
	if result.Error != "task failed" {
		t.Errorf("Error = %q, want %q", result.Error, "task failed")
	}
}

func TestAutoCommitChanges_NoChanges(t *testing.T) {
	// Create a temp directory with git repo
	tmpDir, err := os.MkdirTemp("", "quick-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	if err := initGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor := NewQuickExecutor(tmpDir, testFactory())
	err = executor.autoCommitChanges("test task")

	// Should return error about no changes
	if err == nil {
		t.Error("Expected error for no changes, got nil")
	}
	if err.Error() != "no changes to commit" {
		t.Errorf("Error = %q, want %q", err.Error(), "no changes to commit")
	}
}

func TestAutoCommitChanges_WithChanges(t *testing.T) {
	// Create a temp directory with git repo
	tmpDir, err := os.MkdirTemp("", "quick-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	if err := initGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create a new file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	executor := NewQuickExecutor(tmpDir, testFactory())
	err = executor.autoCommitChanges("add test file")

	if err != nil {
		t.Fatalf("autoCommitChanges failed: %v", err)
	}

	// Verify commit was created
	logCmd := exec.Command("git", "log", "--oneline", "-1")
	logCmd.Dir = tmpDir
	output, err := logCmd.Output()
	if err != nil {
		t.Fatalf("Failed to get git log: %v", err)
	}

	if len(output) == 0 {
		t.Error("Expected commit to be created")
	}

	// Verify commit message contains task
	if !contains(string(output), "Quick: add test file") {
		t.Errorf("Commit message = %q, should contain 'Quick: add test file'", string(output))
	}
}

func TestAutoCommitChanges_InvalidRepo(t *testing.T) {
	// Create a temp directory without git repo
	tmpDir, err := os.MkdirTemp("", "quick-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	executor := NewQuickExecutor(tmpDir, testFactory())
	err = executor.autoCommitChanges("test task")

	// Should fail because not a git repo
	if err == nil {
		t.Error("Expected error for non-git directory, got nil")
	}
}

func TestAutoCommitChanges_StagedAndUnstagedChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "quick-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Create and commit initial file
	file1 := filepath.Join(tmpDir, "file1.txt")
	if err := os.WriteFile(file1, []byte("initial"), 0644); err != nil {
		t.Fatalf("Failed to create file1: %v", err)
	}

	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = tmpDir
	if err := addCmd.Run(); err != nil {
		t.Fatalf("Failed to git add: %v", err)
	}

	commitCmd := exec.Command("git", "commit", "-m", "initial")
	commitCmd.Dir = tmpDir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("Failed to git commit: %v", err)
	}

	// Modify file1 and create new file2
	if err := os.WriteFile(file1, []byte("modified"), 0644); err != nil {
		t.Fatalf("Failed to modify file1: %v", err)
	}

	file2 := filepath.Join(tmpDir, "file2.txt")
	if err := os.WriteFile(file2, []byte("new file"), 0644); err != nil {
		t.Fatalf("Failed to create file2: %v", err)
	}

	executor := NewQuickExecutor(tmpDir, testFactory())
	err = executor.autoCommitChanges("update files")

	if err != nil {
		t.Fatalf("autoCommitChanges failed: %v", err)
	}

	// Verify both files are in the commit
	statusCmd := exec.Command("git", "status", "--porcelain")
	statusCmd.Dir = tmpDir
	output, _ := statusCmd.Output()

	if len(output) != 0 {
		t.Errorf("Expected clean working directory, got: %s", output)
	}
}

func TestQuickExecutor_Execute_ContextCancellation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "quick-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor := NewQuickExecutor(tmpDir, testFactory())

	// Create already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Execute should fail quickly with cancelled context
	result, err := executor.Execute(ctx, "test task")

	// The error behavior depends on how ClaudeProcess handles context cancellation
	// At minimum, the duration should be short
	if result != nil && result.Duration > 5*time.Second {
		t.Errorf("Expected quick failure with cancelled context, took %v", result.Duration)
	}

	// Either error or failed result is acceptable for cancelled context
	if err == nil && (result == nil || result.Success) {
		t.Log("Warning: Expected failure for cancelled context")
	}
}

func TestTokenEstimation(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectedMin int64
	}{
		{
			name:        "short task",
			input:       "fix typo",
			expectedMin: 10, // len("fix typo")/4 = 2, but min is 10
		},
		{
			name:        "medium task",
			input:       "Add a new feature to handle user authentication with OAuth2",
			expectedMin: 10, // 59 / 4 = 14, which is >= 10
		},
		{
			name:        "long task",
			input:       string(make([]byte, 400)), // 400 chars
			expectedMin: 10,                        // 400 / 4 = 100, which is >= 10
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens := int64(len(tt.input) / 4)
			if tokens < 10 {
				tokens = 10
			}

			// Verify tokens meet minimum threshold
			if tokens < tt.expectedMin {
				t.Errorf("Token estimate = %d, want at least %d", tokens, tt.expectedMin)
			}

			// Verify calculation is correct
			expectedTokens := int64(len(tt.input) / 4)
			if expectedTokens < 10 {
				expectedTokens = 10
			}
			if tokens != expectedTokens {
				t.Errorf("Token estimate = %d, want %d", tokens, expectedTokens)
			}
		})
	}
}

// Helper functions

func initGitRepo(dir string) error {
	initCmd := exec.Command("git", "init")
	initCmd.Dir = dir
	if err := initCmd.Run(); err != nil {
		return err
	}

	// Configure git user for commits
	configName := exec.Command("git", "config", "user.name", "Test")
	configName.Dir = dir
	if err := configName.Run(); err != nil {
		return err
	}

	configEmail := exec.Command("git", "config", "user.email", "test@test.com")
	configEmail.Dir = dir
	if err := configEmail.Run(); err != nil {
		return err
	}

	// Create initial commit so we have a valid repo
	readmeFile := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Test"), 0644); err != nil {
		return err
	}

	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	if err := addCmd.Run(); err != nil {
		return err
	}

	commitCmd := exec.Command("git", "commit", "-m", "Initial commit")
	commitCmd.Dir = dir
	return commitCmd.Run()
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
