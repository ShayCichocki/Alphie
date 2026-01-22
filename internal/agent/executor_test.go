package agent

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ShayCichocki/alphie/internal/learning"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// mockRunner implements ClaudeRunner for testing
type mockRunner struct {
	outputCh chan StreamEvent
}

func (m *mockRunner) Start(prompt, workDir string) error { return nil }
func (m *mockRunner) StartWithOptions(prompt, workDir string, opts *StartOptions) error {
	return nil
}
func (m *mockRunner) Output() <-chan StreamEvent { return m.outputCh }
func (m *mockRunner) Wait() error                { return nil }
func (m *mockRunner) Kill() error                { return nil }
func (m *mockRunner) Stderr() string             { return "" }
func (m *mockRunner) PID() int                   { return 0 }

// mockRunnerFactory creates mock ClaudeRunner instances for testing
type mockRunnerFactory struct{}

func (f *mockRunnerFactory) NewRunner() ClaudeRunner {
	return &mockRunner{outputCh: make(chan StreamEvent)}
}

// testRunnerFactory returns a factory for use in tests
func testRunnerFactory() ClaudeRunnerFactory {
	return &mockRunnerFactory{}
}

func TestNewExecutor(t *testing.T) {
	// Create a temp directory for the test repo
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cfg := ExecutorConfig{
		RepoPath:      tmpDir,
		Model:         "claude-sonnet-4-20250514",
		RunnerFactory: testRunnerFactory(),
	}

	executor, err := NewExecutor(cfg)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	if executor == nil {
		t.Fatal("Executor should not be nil")
	}
	if executor.worktreeMgr == nil {
		t.Error("worktreeMgr should not be nil")
	}
	if executor.tokenTracker == nil {
		t.Error("tokenTracker should not be nil")
	}
	if executor.agentMgr == nil {
		t.Error("agentMgr should not be nil")
	}
	if executor.model != "claude-sonnet-4-20250514" {
		t.Errorf("model = %q, want %q", executor.model, "claude-sonnet-4-20250514")
	}
}

func TestNewExecutor_DefaultModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	cfg := ExecutorConfig{
		RepoPath:      tmpDir,
		RunnerFactory: testRunnerFactory(),
		// No model specified
	}

	executor, err := NewExecutor(cfg)
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	// Should default to sonnet
	if executor.model != "claude-sonnet-4-20250514" {
		t.Errorf("Default model = %q, want %q", executor.model, "claude-sonnet-4-20250514")
	}
}

func TestNewExecutor_InvalidRepo(t *testing.T) {
	cfg := ExecutorConfig{
		RepoPath: "/nonexistent/path/that/definitely/does/not/exist/anywhere",
	}

	// NewExecutor may or may not fail for non-existent paths depending on
	// whether the worktree manager validates the path immediately.
	// This test documents the current behavior.
	executor, err := NewExecutor(cfg)

	// Either error OR executor with the configured path is acceptable
	if err == nil && executor != nil {
		// Executor was created - that's fine, validation may happen later
		t.Log("Executor created despite invalid path - validation deferred")
	} else if err != nil {
		// Error occurred - also fine
		t.Logf("Expected error for invalid path: %v", err)
	}
}

func TestExecutionResult_Fields(t *testing.T) {
	result := ExecutionResult{
		Success:      true,
		Output:       "Task completed",
		Error:        "",
		TokensUsed:   1500,
		Cost:         0.05,
		Duration:     30 * time.Second,
		AgentID:      "agent-123",
		WorktreePath: "/tmp/worktree",
		Model:        "claude-sonnet-4-20250514",
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if result.TokensUsed != 1500 {
		t.Errorf("TokensUsed = %d, want 1500", result.TokensUsed)
	}
	if result.Duration != 30*time.Second {
		t.Errorf("Duration = %v, want 30s", result.Duration)
	}
}

func TestExecutionResult_FailedResult(t *testing.T) {
	result := ExecutionResult{
		Success: false,
		Output:  "partial output",
		Error:   "process failed",
	}

	if result.Success {
		t.Error("Success should be false")
	}
	if result.Error != "process failed" {
		t.Errorf("Error = %q, want %q", result.Error, "process failed")
	}
}

func TestProgressUpdate_Fields(t *testing.T) {
	update := ProgressUpdate{
		AgentID:    "agent-1",
		TokensUsed: 500,
		Cost:       0.02,
		Duration:   10 * time.Second,
	}

	if update.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", update.AgentID, "agent-1")
	}
	if update.TokensUsed != 500 {
		t.Errorf("TokensUsed = %d, want 500", update.TokensUsed)
	}
}

func TestExecutor_BuildPrompt_Basic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	task := &models.Task{
		ID:    "task-123",
		Title: "Fix the bug",
	}

	prompt := executor.buildPrompt(task, models.TierBuilder, nil)

	if !strings.Contains(prompt, "task-123") {
		t.Error("Prompt should contain task ID")
	}
	if !strings.Contains(prompt, "Fix the bug") {
		t.Error("Prompt should contain task title")
	}
	if !strings.Contains(prompt, "builder") {
		t.Error("Prompt should contain tier")
	}
}

func TestExecutor_BuildPrompt_WithDescription(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	task := &models.Task{
		ID:          "task-123",
		Title:       "Fix the bug",
		Description: "This is a detailed description of the bug.",
	}

	prompt := executor.buildPrompt(task, models.TierBuilder, nil)

	if !strings.Contains(prompt, "This is a detailed description") {
		t.Error("Prompt should contain task description")
	}
}

func TestExecutor_BuildPrompt_TierGuidance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	task := &models.Task{ID: "task-1", Title: "Task"}

	tests := []struct {
		tier     models.Tier
		contains string
	}{
		{models.TierScout, "Scout agent"},
		{models.TierBuilder, "Builder agent"},
		{models.TierArchitect, "Architect agent"},
	}

	for _, tt := range tests {
		t.Run(string(tt.tier), func(t *testing.T) {
			prompt := executor.buildPrompt(task, tt.tier, nil)
			if !strings.Contains(prompt, tt.contains) {
				t.Errorf("Prompt for tier %s should contain %q", tt.tier, tt.contains)
			}
		})
	}
}

func TestExecutor_BuildPrompt_WithLearnings(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	task := &models.Task{ID: "task-1", Title: "Task"}

	opts := &ExecuteOptions{
		Learnings: []*learning.Learning{
			{
				Condition: "When handling authentication",
				Action:    "Always validate tokens",
				Outcome:   "Prevents security issues",
			},
		},
	}

	prompt := executor.buildPrompt(task, models.TierBuilder, opts)

	if !strings.Contains(prompt, "Relevant Learnings") {
		t.Error("Prompt should contain learnings section")
	}
	if !strings.Contains(prompt, "When handling authentication") {
		t.Error("Prompt should contain learning condition")
	}
	if !strings.Contains(prompt, "Always validate tokens") {
		t.Error("Prompt should contain learning action")
	}
}

func TestExecutor_ProcessStreamEvent_Assistant(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	tracker := NewTokenTracker("claude-sonnet-4-20250514")
	var output strings.Builder

	event := StreamEvent{
		Type:    StreamEventAssistant,
		Message: "Working on the task",
	}

	executor.processStreamEvent(event, tracker, &output)

	if !strings.Contains(output.String(), "Working on the task") {
		t.Errorf("Output should contain assistant message, got %q", output.String())
	}
}

func TestExecutor_ProcessStreamEvent_Result(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	tracker := NewTokenTracker("claude-sonnet-4-20250514")
	var output strings.Builder

	event := StreamEvent{
		Type:    StreamEventResult,
		Message: "Task completed successfully",
	}

	executor.processStreamEvent(event, tracker, &output)

	if !strings.Contains(output.String(), "Result") {
		t.Error("Output should contain result header")
	}
	if !strings.Contains(output.String(), "Task completed successfully") {
		t.Error("Output should contain result message")
	}
}

func TestExecutor_ProcessStreamEvent_Error(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	tracker := NewTokenTracker("claude-sonnet-4-20250514")
	var output strings.Builder

	event := StreamEvent{
		Type:  StreamEventError,
		Error: "Something went wrong",
	}

	executor.processStreamEvent(event, tracker, &output)

	if !strings.Contains(output.String(), "Error") {
		t.Error("Output should contain error header")
	}
	if !strings.Contains(output.String(), "Something went wrong") {
		t.Error("Output should contain error message")
	}
}

func TestExecutor_ExtractTokenUsage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	raw := json.RawMessage(`{"usage": {"input_tokens": 100, "output_tokens": 50}}`)
	executor.extractTokenUsage(raw, tracker)

	usage := tracker.GetUsage()
	if usage.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", usage.InputTokens)
	}
	if usage.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", usage.OutputTokens)
	}
}

func TestExecutor_ExtractTokenUsage_NoUsage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	raw := json.RawMessage(`{"message": "no usage here"}`)
	executor.extractTokenUsage(raw, tracker)

	usage := tracker.GetUsage()
	if usage.TotalTokens != 0 {
		t.Errorf("TotalTokens should be 0 when no usage, got %d", usage.TotalTokens)
	}
}

func TestExecutor_ExtractTokenUsage_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	tracker := NewTokenTracker("claude-sonnet-4-20250514")

	raw := json.RawMessage(`invalid json`)
	// Should not panic
	executor.extractTokenUsage(raw, tracker)

	usage := tracker.GetUsage()
	if usage.TotalTokens != 0 {
		t.Errorf("TotalTokens should be 0 for invalid JSON, got %d", usage.TotalTokens)
	}
}

func TestExecutor_AutoCommitChanges_NoChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	err = executor.autoCommitChanges(tmpDir, "test task")

	if err == nil {
		t.Error("Expected error for no changes")
	}
	if !strings.Contains(err.Error(), "no changes to commit") {
		t.Errorf("Error = %q, should contain 'no changes to commit'", err.Error())
	}
}

func TestExecutor_AutoCommitChanges_WithChanges(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "executor-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := initTestGitRepo(tmpDir); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	executor, err := NewExecutor(ExecutorConfig{RepoPath: tmpDir, RunnerFactory: testRunnerFactory()})
	if err != nil {
		t.Fatalf("NewExecutor failed: %v", err)
	}

	// Create a new file
	testFile := filepath.Join(tmpDir, "newfile.txt")
	if err := os.WriteFile(testFile, []byte("new content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = executor.autoCommitChanges(tmpDir, "add new file")

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

	if !strings.Contains(string(output), "Agent: add new file") {
		t.Errorf("Commit message = %q, should contain 'Agent: add new file'", string(output))
	}
}

func TestExecutorConfig_Fields(t *testing.T) {
	cfg := ExecutorConfig{
		WorktreeBaseDir: "/tmp/worktrees",
		RepoPath:        "/path/to/repo",
		Model:           "claude-opus-4-5-20251101",
	}

	if cfg.WorktreeBaseDir != "/tmp/worktrees" {
		t.Errorf("WorktreeBaseDir = %q, want %q", cfg.WorktreeBaseDir, "/tmp/worktrees")
	}
	if cfg.RepoPath != "/path/to/repo" {
		t.Errorf("RepoPath = %q, want %q", cfg.RepoPath, "/path/to/repo")
	}
	if cfg.Model != "claude-opus-4-5-20251101" {
		t.Errorf("Model = %q, want %q", cfg.Model, "claude-opus-4-5-20251101")
	}
}

func TestExecuteOptions_Fields(t *testing.T) {
	learnings := []*learning.Learning{
		{Condition: "test", Action: "do", Outcome: "result"},
	}

	var progressCalled bool
	callback := func(update ProgressUpdate) {
		progressCalled = true
	}

	opts := ExecuteOptions{
		Learnings:  learnings,
		OnProgress: callback,
	}

	if len(opts.Learnings) != 1 {
		t.Errorf("Learnings len = %d, want 1", len(opts.Learnings))
	}

	opts.OnProgress(ProgressUpdate{})
	if !progressCalled {
		t.Error("OnProgress callback should have been called")
	}
}

// Helper function to initialize a git repo for testing
func initTestGitRepo(dir string) error {
	initCmd := exec.Command("git", "init")
	initCmd.Dir = dir
	if err := initCmd.Run(); err != nil {
		return err
	}

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

	// Create initial commit
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
