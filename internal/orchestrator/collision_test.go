package orchestrator

import (
	"sort"
	"testing"

	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestNewCollisionChecker(t *testing.T) {
	cc := NewCollisionChecker()
	if cc == nil {
		t.Fatal("expected non-nil collision checker")
	}
}

func TestCollisionCheckerRegisterUnregister(t *testing.T) {
	cc := NewCollisionChecker()

	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	// Unregister should not panic
	cc.UnregisterAgent("agent-1")
	cc.UnregisterAgent("non-existent") // Should not panic
}

func TestCollisionCheckerRecordTouch(t *testing.T) {
	cc := NewCollisionChecker()

	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	// Touch a file 4 times (exceeds hotspotThreshold of 3)
	for i := 0; i < 4; i++ {
		cc.RecordTouch("agent-1", "internal/auth/auth.go")
	}

	// File should now be a hotspot
	hotspots := cc.GetHotspots()
	if len(hotspots) != 1 {
		t.Errorf("expected 1 hotspot, got %d", len(hotspots))
	}
	if len(hotspots) > 0 && hotspots[0] != "internal/auth/auth.go" {
		t.Errorf("expected internal/auth/auth.go as hotspot, got %s", hotspots[0])
	}
}

func TestCollisionCheckerHotspotThreshold(t *testing.T) {
	cc := NewCollisionChecker()

	cc.RegisterAgent("agent-1", []string{"internal/"}, nil)

	// Touch exactly at threshold (3 times) - should not be hotspot yet
	for i := 0; i < 3; i++ {
		cc.RecordTouch("agent-1", "internal/config.go")
	}

	hotspots := cc.GetHotspots()
	if len(hotspots) != 0 {
		t.Errorf("expected no hotspots at threshold, got %d", len(hotspots))
	}

	// One more touch should trigger hotspot
	cc.RecordTouch("agent-1", "internal/config.go")
	hotspots = cc.GetHotspots()
	if len(hotspots) != 1 {
		t.Errorf("expected 1 hotspot after exceeding threshold, got %d", len(hotspots))
	}
}

func TestCollisionCheckerPathPrefixCollision(t *testing.T) {
	cc := NewCollisionChecker()

	// Agent working in internal/auth/
	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
	}

	// Task trying to work in same prefix should be blocked
	task := &models.Task{
		ID:          "task-2",
		Title:       "Work on internal/auth/ module",
		Description: "Modify internal/auth/handler.go",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if canSchedule {
		t.Error("expected task to be blocked due to path prefix collision")
	}
}

func TestCollisionCheckerNoPathPrefixCollision(t *testing.T) {
	cc := NewCollisionChecker()

	// Agent working in internal/auth/
	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
	}

	// Task working in different prefix should be allowed
	task := &models.Task{
		ID:          "task-2",
		Title:       "Work on pkg/utils/",
		Description: "Modify pkg/utils/helper.go",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if !canSchedule {
		t.Error("expected task to be allowed (different prefix)")
	}
}

func TestCollisionCheckerPrefixContainment(t *testing.T) {
	cc := NewCollisionChecker()

	// Agent working in broader internal/ prefix
	cc.RegisterAgent("agent-1", []string{"internal/"}, nil)

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
	}

	// Task trying to work in sub-prefix should be blocked
	task := &models.Task{
		ID:          "task-2",
		Title:       "Work on internal/auth/ module",
		Description: "Modify internal/auth/handler.go",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if canSchedule {
		t.Error("expected task to be blocked (sub-prefix of running agent)")
	}
}

func TestCollisionCheckerReversePrefixContainment(t *testing.T) {
	cc := NewCollisionChecker()

	// Agent working in narrow internal/auth/ prefix
	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
	}

	// Task trying to work in broader internal/ should be blocked
	task := &models.Task{
		ID:          "task-2",
		Title:       "Work on internal/ package",
		Description: "Refactor internal/ directory structure",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if canSchedule {
		t.Error("expected task to be blocked (broader prefix than running agent)")
	}
}

func TestCollisionCheckerHotspotCollision(t *testing.T) {
	cc := NewCollisionChecker()

	// Agent has a hotspot file
	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, []string{"internal/auth/auth.go"})

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
	}

	// Task trying to work in prefix containing hotspot should be blocked
	task := &models.Task{
		ID:          "task-2",
		Title:       "Work on internal/auth/ module",
		Description: "Modify internal/auth/ files",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if canSchedule {
		t.Error("expected task to be blocked due to hotspot collision")
	}
}

func TestCollisionCheckerTopLevelLimit(t *testing.T) {
	cc := NewCollisionChecker()

	// Two agents already working in internal/
	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)
	cc.RegisterAgent("agent-2", []string{"internal/config/"}, nil)

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
		{ID: "agent-2", Status: models.AgentStatusRunning},
	}

	// Third task in internal/ should be blocked (max 2 per top-level)
	task := &models.Task{
		ID:          "task-3",
		Title:       "Work on internal/utils/",
		Description: "Add internal/utils/helper.go",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if canSchedule {
		t.Error("expected task to be blocked due to top-level directory limit")
	}
}

func TestCollisionCheckerTopLevelLimitDifferentDirs(t *testing.T) {
	cc := NewCollisionChecker()

	// Two agents in different top-level directories
	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)
	cc.RegisterAgent("agent-2", []string{"pkg/utils/"}, nil)

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
		{ID: "agent-2", Status: models.AgentStatusRunning},
	}

	// Task in cmd/ should be allowed (different top-level)
	task := &models.Task{
		ID:          "task-3",
		Title:       "Work on cmd/server/",
		Description: "Update cmd/server/main.go",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if !canSchedule {
		t.Error("expected task to be allowed (different top-level directory)")
	}
}

func TestCollisionCheckerNoPathInfo(t *testing.T) {
	cc := NewCollisionChecker()

	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
	}

	// Task with no path information should be allowed
	task := &models.Task{
		ID:          "task-2",
		Title:       "Generic task",
		Description: "Do something without file paths",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if !canSchedule {
		t.Error("expected task with no path info to be allowed")
	}
}

func TestCollisionCheckerNonRunningAgentsIgnored(t *testing.T) {
	cc := NewCollisionChecker()

	cc.RegisterAgent("agent-1", []string{"internal/auth/"}, nil)

	// Agent is not running
	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusPaused},
	}

	// Task in same prefix should be allowed since agent is not running
	task := &models.Task{
		ID:          "task-2",
		Title:       "Work on internal/auth/",
		Description: "Modify internal/auth/handler.go",
	}

	canSchedule := cc.CanSchedule(task, runningAgents)
	if !canSchedule {
		t.Error("expected task to be allowed (agent not running)")
	}
}

func TestCollisionCheckerMultipleHotspots(t *testing.T) {
	cc := NewCollisionChecker()

	cc.RegisterAgent("agent-1", []string{"internal/"}, nil)

	// Touch multiple files to make them hotspots
	files := []string{
		"internal/auth/auth.go",
		"internal/config/config.go",
		"internal/utils/helper.go",
	}

	for _, f := range files {
		for i := 0; i < 4; i++ {
			cc.RecordTouch("agent-1", f)
		}
	}

	hotspots := cc.GetHotspots()
	if len(hotspots) != 3 {
		t.Errorf("expected 3 hotspots, got %d", len(hotspots))
	}

	// Verify all files are hotspots
	sort.Strings(hotspots)
	sort.Strings(files)
	for i, h := range hotspots {
		if h != files[i] {
			t.Errorf("expected hotspot %s, got %s", files[i], h)
		}
	}
}

func TestCollisionCheckerGetTopLevelDir(t *testing.T) {
	cc := NewCollisionChecker()

	tests := []struct {
		prefix   string
		expected string
	}{
		{"internal/auth/", "internal"},
		{"pkg/utils/", "pkg"},
		{"cmd/server/main.go", "cmd"},
		{"/internal/auth/", "internal"},
		{"src", "src"},
		{"", ""},
	}

	for _, tc := range tests {
		got := cc.getTopLevelDir(tc.prefix)
		if got != tc.expected {
			t.Errorf("getTopLevelDir(%q) = %q, expected %q", tc.prefix, got, tc.expected)
		}
	}
}

func TestCollisionCheckerExtractPathPrefixes(t *testing.T) {
	cc := NewCollisionChecker()

	tests := []struct {
		title       string
		description string
		wantPaths   bool
	}{
		{
			title:       "Update internal/auth handler",
			description: "Fix bug in internal/auth/handler.go",
			wantPaths:   true,
		},
		{
			title:       "Add pkg/utils helper",
			description: "Create new utility functions",
			wantPaths:   true,
		},
		{
			title:       "Generic task",
			description: "Do something without paths",
			wantPaths:   false,
		},
	}

	for _, tc := range tests {
		task := &models.Task{
			ID:          "test",
			Title:       tc.title,
			Description: tc.description,
		}
		prefixes := cc.ExtractPathPrefixes(task)
		hasPaths := len(prefixes) > 0
		if hasPaths != tc.wantPaths {
			t.Errorf("extractPathPrefixes for %q: got paths=%v, want paths=%v", tc.title, hasPaths, tc.wantPaths)
		}
	}
}

func TestCollisionCheckerHotspotDeduplication(t *testing.T) {
	cc := NewCollisionChecker()

	cc.RegisterAgent("agent-1", []string{"internal/"}, nil)

	// Touch same file many times (should only appear once in hotspots)
	for i := 0; i < 10; i++ {
		cc.RecordTouch("agent-1", "internal/config.go")
	}

	// Check that the file only appears once in agent's hints
	cc.mu.RLock()
	agentHints := cc.hints["agent-1"]
	cc.mu.RUnlock()

	count := 0
	for _, h := range agentHints.Hotspots {
		if h == "internal/config.go" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("expected hotspot to appear once in hints, appeared %d times", count)
	}
}

func TestCollisionCheckerEmptyRunningAgents(t *testing.T) {
	cc := NewCollisionChecker()

	task := &models.Task{
		ID:          "task-1",
		Title:       "Work on internal/auth/",
		Description: "Update internal/auth/handler.go",
	}

	// No running agents should allow any task
	canSchedule := cc.CanSchedule(task, []*models.Agent{})
	if !canSchedule {
		t.Error("expected task to be allowed with no running agents")
	}
}

func TestCollisionCheckerAgentWithoutHints(t *testing.T) {
	cc := NewCollisionChecker()

	// Don't register agent hints
	runningAgents := []*models.Agent{
		{ID: "agent-1", Status: models.AgentStatusRunning},
	}

	task := &models.Task{
		ID:          "task-2",
		Title:       "Work on internal/auth/",
		Description: "Update internal/auth/handler.go",
	}

	// Agent without hints should not block anything
	canSchedule := cc.CanSchedule(task, runningAgents)
	if !canSchedule {
		t.Error("expected task to be allowed (agent has no hints)")
	}
}

func TestCollisionCheckerConcurrentAccess(t *testing.T) {
	cc := NewCollisionChecker()

	// Run concurrent operations to test thread safety
	done := make(chan bool)

	// Goroutine 1: Register and unregister agents
	go func() {
		for i := 0; i < 100; i++ {
			cc.RegisterAgent("agent-concurrent", []string{"internal/"}, nil)
			cc.UnregisterAgent("agent-concurrent")
		}
		done <- true
	}()

	// Goroutine 2: Record touches
	go func() {
		cc.RegisterAgent("agent-touch", []string{"pkg/"}, nil)
		for i := 0; i < 100; i++ {
			cc.RecordTouch("agent-touch", "pkg/test.go")
		}
		done <- true
	}()

	// Goroutine 3: Check scheduling
	go func() {
		task := &models.Task{
			ID:          "task-test",
			Title:       "Test task internal/",
			Description: "Work on internal/test.go",
		}
		agents := []*models.Agent{
			{ID: "agent-concurrent", Status: models.AgentStatusRunning},
		}
		for i := 0; i < 100; i++ {
			cc.CanSchedule(task, agents)
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}
