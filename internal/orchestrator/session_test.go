package orchestrator

import (
	"testing"
)

func TestSessionBranchManager_BranchNaming(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		greenfield bool
		expected   string
	}{
		{
			name:       "standard session branch naming",
			sessionID:  "abc123",
			greenfield: false,
			expected:   "session-abc123",
		},
		{
			name:       "session with hyphen in ID",
			sessionID:  "task-001",
			greenfield: false,
			expected:   "session-task-001",
		},
		{
			name:       "session with underscore",
			sessionID:  "ts_8b7a01",
			greenfield: false,
			expected:   "session-ts_8b7a01",
		},
		{
			name:       "greenfield mode - no branch",
			sessionID:  "abc123",
			greenfield: true,
			expected:   "",
		},
		{
			name:       "empty session ID",
			sessionID:  "",
			greenfield: false,
			expected:   "session-",
		},
		{
			name:       "long session ID",
			sessionID:  "very-long-session-identifier-12345",
			greenfield: false,
			expected:   "session-very-long-session-identifier-12345",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewSessionBranchManager(tc.sessionID, "/tmp/repo", tc.greenfield)
			result := manager.GetBranchName()
			if result != tc.expected {
				t.Errorf("expected branch name %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestSessionBranchManager_ProtectedBranchDetection(t *testing.T) {
	manager := NewSessionBranchManager("test", "/tmp/repo", false)

	tests := []struct {
		name     string
		branch   string
		expected bool
	}{
		{
			name:     "main is protected",
			branch:   "main",
			expected: true,
		},
		{
			name:     "master is protected",
			branch:   "master",
			expected: true,
		},
		{
			name:     "dev is protected",
			branch:   "dev",
			expected: true,
		},
		{
			name:     "feature branch not protected",
			branch:   "feature/my-feature",
			expected: false,
		},
		{
			name:     "session branch not protected",
			branch:   "session-abc123",
			expected: false,
		},
		{
			name:     "case insensitive - MAIN",
			branch:   "MAIN",
			expected: true,
		},
		{
			name:     "case insensitive - Master",
			branch:   "Master",
			expected: true,
		},
		{
			name:     "case insensitive - DEV",
			branch:   "DEV",
			expected: true,
		},
		{
			name:     "whitespace trimming",
			branch:   "  main  ",
			expected: true,
		},
		{
			name:     "develop not protected",
			branch:   "develop",
			expected: false,
		},
		{
			name:     "release branch not protected",
			branch:   "release/v1.0",
			expected: false,
		},
		{
			name:     "empty branch",
			branch:   "",
			expected: false,
		},
		{
			name:     "main-backup not protected",
			branch:   "main-backup",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := manager.IsProtected(tc.branch)
			if result != tc.expected {
				t.Errorf("IsProtected(%q) = %v, expected %v", tc.branch, result, tc.expected)
			}
		})
	}
}

func TestNewSessionBranchManager(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		repoPath   string
		greenfield bool
	}{
		{
			name:       "normal initialization",
			sessionID:  "test-session",
			repoPath:   "/path/to/repo",
			greenfield: false,
		},
		{
			name:       "greenfield initialization",
			sessionID:  "test-session",
			repoPath:   "/path/to/repo",
			greenfield: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewSessionBranchManager(tc.sessionID, tc.repoPath, tc.greenfield)
			if manager == nil {
				t.Fatal("expected non-nil manager")
			}
			if manager.sessionID != tc.sessionID {
				t.Errorf("expected sessionID %q, got %q", tc.sessionID, manager.sessionID)
			}
			if manager.repoPath != tc.repoPath {
				t.Errorf("expected repoPath %q, got %q", tc.repoPath, manager.repoPath)
			}
			if manager.greenfield != tc.greenfield {
				t.Errorf("expected greenfield %v, got %v", tc.greenfield, manager.greenfield)
			}
		})
	}
}

func TestSessionBranchManager_GreenfieldMode(t *testing.T) {
	// In greenfield mode, CreateBranch and Cleanup should be no-ops
	manager := NewSessionBranchManager("test", "/tmp/fake-repo", true)

	// CreateBranch should return nil in greenfield mode
	err := manager.CreateBranch()
	if err != nil {
		t.Errorf("expected nil error for greenfield CreateBranch, got %v", err)
	}

	// Cleanup should return nil in greenfield mode
	err = manager.Cleanup()
	if err != nil {
		t.Errorf("expected nil error for greenfield Cleanup, got %v", err)
	}

	// Branch name should be empty in greenfield mode
	if manager.GetBranchName() != "" {
		t.Errorf("expected empty branch name in greenfield mode, got %q", manager.GetBranchName())
	}
}
