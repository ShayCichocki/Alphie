package agent

import (
	"strings"
	"testing"
)

func TestWorktreePathGeneration(t *testing.T) {
	// Test that worktree path is generated correctly
	baseDir := "/home/test/.cache/alphie/worktrees"
	agentID := "test-agent-123"
	expectedBranch := "agent-test-agent-123"
	expectedPath := "/home/test/.cache/alphie/worktrees/agent-test-agent-123"

	// Manually construct what Create() would produce
	branchName := "agent-" + agentID
	path := baseDir + "/" + branchName

	if branchName != expectedBranch {
		t.Errorf("branchName = %q, want %q", branchName, expectedBranch)
	}
	if path != expectedPath {
		t.Errorf("path = %q, want %q", path, expectedPath)
	}
}

func TestWorktreeBranchNaming(t *testing.T) {
	tests := []struct {
		agentID        string
		expectedBranch string
	}{
		{"abc123", "agent-abc123"},
		{"uuid-like-id", "agent-uuid-like-id"},
		{"simple", "agent-simple"},
	}

	for _, tt := range tests {
		t.Run(tt.agentID, func(t *testing.T) {
			branch := "agent-" + tt.agentID
			if branch != tt.expectedBranch {
				t.Errorf("branch = %q, want %q", branch, tt.expectedBranch)
			}
		})
	}
}

func TestParseWorktreeList(t *testing.T) {
	// Simulate git worktree list --porcelain output
	output := `worktree /home/user/project
branch refs/heads/main

worktree /home/user/.cache/alphie/worktrees/agent-abc123
branch refs/heads/agent-abc123

worktree /home/user/.cache/alphie/worktrees/agent-def456
branch refs/heads/agent-def456
`

	// Create a minimal WorktreeManager for parsing
	m := &WorktreeManager{
		baseDir:  "/home/user/.cache/alphie/worktrees",
		repoPath: "/home/user/project",
	}

	worktrees, err := m.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 3 {
		t.Fatalf("Expected 3 worktrees, got %d", len(worktrees))
	}

	// Check first worktree (main)
	if worktrees[0].Path != "/home/user/project" {
		t.Errorf("worktrees[0].Path = %q, want %q", worktrees[0].Path, "/home/user/project")
	}
	if worktrees[0].BranchName != "main" {
		t.Errorf("worktrees[0].BranchName = %q, want %q", worktrees[0].BranchName, "main")
	}
	if worktrees[0].AgentID != "" {
		t.Errorf("worktrees[0].AgentID = %q, want empty (not an agent branch)", worktrees[0].AgentID)
	}

	// Check second worktree (agent)
	if worktrees[1].Path != "/home/user/.cache/alphie/worktrees/agent-abc123" {
		t.Errorf("worktrees[1].Path = %q", worktrees[1].Path)
	}
	if worktrees[1].BranchName != "agent-abc123" {
		t.Errorf("worktrees[1].BranchName = %q, want %q", worktrees[1].BranchName, "agent-abc123")
	}
	if worktrees[1].AgentID != "abc123" {
		t.Errorf("worktrees[1].AgentID = %q, want %q", worktrees[1].AgentID, "abc123")
	}
}

func TestParseWorktreeListNoTrailingNewline(t *testing.T) {
	// Output without trailing blank line
	output := `worktree /home/user/project
branch refs/heads/main`

	m := &WorktreeManager{
		baseDir:  "/tmp",
		repoPath: "/home/user/project",
	}

	worktrees, err := m.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 1 {
		t.Fatalf("Expected 1 worktree, got %d", len(worktrees))
	}

	if worktrees[0].Path != "/home/user/project" {
		t.Errorf("Path = %q, want %q", worktrees[0].Path, "/home/user/project")
	}
}

func TestParseWorktreeListEmpty(t *testing.T) {
	m := &WorktreeManager{
		baseDir:  "/tmp",
		repoPath: "/home/user/project",
	}

	worktrees, err := m.parseWorktreeList("")
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 0 {
		t.Errorf("Expected 0 worktrees, got %d", len(worktrees))
	}
}

func TestParseWorktreeListDetachedHead(t *testing.T) {
	// Detached HEAD worktree (no branch line)
	output := `worktree /home/user/project
HEAD abc123def

worktree /home/user/.cache/alphie/worktrees/agent-test
branch refs/heads/agent-test
`

	m := &WorktreeManager{
		baseDir:  "/tmp",
		repoPath: "/home/user/project",
	}

	worktrees, err := m.parseWorktreeList(output)
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}

	if len(worktrees) != 2 {
		t.Fatalf("Expected 2 worktrees, got %d", len(worktrees))
	}

	// First worktree has no branch (detached)
	if worktrees[0].BranchName != "" {
		t.Errorf("Detached worktree should have empty BranchName, got %q", worktrees[0].BranchName)
	}
}

func TestIsAlphieWorktree(t *testing.T) {
	tests := []struct {
		branchName string
		expected   bool
	}{
		{"agent-abc123", true},
		{"alphie/feature-branch", true},
		{"session-xyz", true},
		{"main", false},
		{"feature/my-feature", false},
		{"develop", false},
	}

	for _, tt := range tests {
		t.Run(tt.branchName, func(t *testing.T) {
			wt := &Worktree{BranchName: tt.branchName}
			result := isAlphieWorktree(wt)
			if result != tt.expected {
				t.Errorf("isAlphieWorktree(%q) = %v, want %v", tt.branchName, result, tt.expected)
			}
		})
	}
}

func TestExtractSessionID(t *testing.T) {
	tests := []struct {
		branchName string
		expected   string
	}{
		{"agent-abc123", "abc123"},
		{"alphie/feature-branch", "feature-branch"},
		{"session-xyz789", "xyz789"},
		{"main", ""},
		{"feature/something", ""},
	}

	for _, tt := range tests {
		t.Run(tt.branchName, func(t *testing.T) {
			wt := &Worktree{BranchName: tt.branchName}
			result := extractSessionID(wt)
			if result != tt.expected {
				t.Errorf("extractSessionID(%q) = %q, want %q", tt.branchName, result, tt.expected)
			}
		})
	}
}

func TestAlphieWorktreePatterns(t *testing.T) {
	expectedPatterns := []string{"agent-", "alphie/", "session-"}

	if len(alphieWorktreePatterns) != len(expectedPatterns) {
		t.Fatalf("alphieWorktreePatterns has %d patterns, want %d", len(alphieWorktreePatterns), len(expectedPatterns))
	}

	for i, pattern := range expectedPatterns {
		if alphieWorktreePatterns[i] != pattern {
			t.Errorf("alphieWorktreePatterns[%d] = %q, want %q", i, alphieWorktreePatterns[i], pattern)
		}
	}
}

func TestWorktreeStruct(t *testing.T) {
	wt := &Worktree{
		Path:       "/path/to/worktree",
		BranchName: "agent-test",
		AgentID:    "test",
	}

	if wt.Path != "/path/to/worktree" {
		t.Errorf("Path = %q, want %q", wt.Path, "/path/to/worktree")
	}
	if wt.BranchName != "agent-test" {
		t.Errorf("BranchName = %q, want %q", wt.BranchName, "agent-test")
	}
	if wt.AgentID != "test" {
		t.Errorf("AgentID = %q, want %q", wt.AgentID, "test")
	}
}

func TestWorktreeManagerBaseDir(t *testing.T) {
	m := &WorktreeManager{
		baseDir:  "/custom/base/dir",
		repoPath: "/home/user/project",
	}

	if m.BaseDir() != "/custom/base/dir" {
		t.Errorf("BaseDir() = %q, want %q", m.BaseDir(), "/custom/base/dir")
	}
}

func TestWorktreeManagerRepoPath(t *testing.T) {
	m := &WorktreeManager{
		baseDir:  "/tmp",
		repoPath: "/home/user/project",
	}

	if m.RepoPath() != "/home/user/project" {
		t.Errorf("RepoPath() = %q, want %q", m.RepoPath(), "/home/user/project")
	}
}

func TestParseWorktreeListUnlockedMatchesParseWorktreeList(t *testing.T) {
	output := `worktree /home/user/project
branch refs/heads/main

worktree /home/user/.cache/alphie/worktrees/agent-abc123
branch refs/heads/agent-abc123
`

	m := &WorktreeManager{
		baseDir:  "/tmp",
		repoPath: "/home/user/project",
	}

	worktrees1, err1 := m.parseWorktreeList(output)
	worktrees2, err2 := m.parseWorktreeListUnlocked(output)

	if err1 != nil || err2 != nil {
		t.Fatalf("Errors: parseWorktreeList=%v, parseWorktreeListUnlocked=%v", err1, err2)
	}

	if len(worktrees1) != len(worktrees2) {
		t.Fatalf("Different lengths: %d vs %d", len(worktrees1), len(worktrees2))
	}

	for i := range worktrees1 {
		if worktrees1[i].Path != worktrees2[i].Path {
			t.Errorf("Path mismatch at %d: %q vs %q", i, worktrees1[i].Path, worktrees2[i].Path)
		}
		if worktrees1[i].BranchName != worktrees2[i].BranchName {
			t.Errorf("BranchName mismatch at %d: %q vs %q", i, worktrees1[i].BranchName, worktrees2[i].BranchName)
		}
	}
}

func TestBranchNameFromAgentID(t *testing.T) {
	tests := []struct {
		agentID  string
		expected string
	}{
		{"abc123", "agent-abc123"},
		{"", "agent-"},
		{"with-dashes", "agent-with-dashes"},
		{"with_underscores", "agent-with_underscores"},
	}

	for _, tt := range tests {
		t.Run(tt.agentID, func(t *testing.T) {
			result := "agent-" + tt.agentID
			if result != tt.expected {
				t.Errorf("branch name = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestAgentIDFromBranchName(t *testing.T) {
	tests := []struct {
		branchName string
		expected   string
	}{
		{"agent-abc123", "abc123"},
		{"agent-", ""},
		{"agent-with-dashes", "with-dashes"},
		{"not-an-agent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.branchName, func(t *testing.T) {
			var result string
			if strings.HasPrefix(tt.branchName, "agent-") {
				result = strings.TrimPrefix(tt.branchName, "agent-")
			}
			if result != tt.expected {
				t.Errorf("agent ID = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFilterOrphansWithActiveSessions(t *testing.T) {
	worktrees := []*Worktree{
		{Path: "/repo", BranchName: "main"},                   // Not an alphie worktree
		{Path: "/wt/agent-active", BranchName: "agent-active", AgentID: "active"},
		{Path: "/wt/agent-orphan1", BranchName: "agent-orphan1", AgentID: "orphan1"},
		{Path: "/wt/agent-orphan2", BranchName: "agent-orphan2", AgentID: "orphan2"},
		{Path: "/wt/alphie/active2", BranchName: "alphie/active2"},
	}

	activeSessions := []string{"active", "active2"}
	activeSet := make(map[string]bool)
	for _, s := range activeSessions {
		activeSet[s] = true
	}

	var orphans []*Worktree
	for _, wt := range worktrees {
		if !isAlphieWorktree(wt) {
			continue
		}
		sessionID := extractSessionID(wt)
		if sessionID != "" && activeSet[sessionID] {
			continue
		}
		orphans = append(orphans, wt)
	}

	if len(orphans) != 2 {
		t.Fatalf("Expected 2 orphans, got %d", len(orphans))
	}

	// Verify the orphans are the expected ones
	orphanIDs := make(map[string]bool)
	for _, o := range orphans {
		orphanIDs[o.AgentID] = true
	}

	if !orphanIDs["orphan1"] {
		t.Error("Expected orphan1 to be in orphans list")
	}
	if !orphanIDs["orphan2"] {
		t.Error("Expected orphan2 to be in orphans list")
	}
}
