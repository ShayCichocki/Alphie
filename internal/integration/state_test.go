//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/internal/state"
)

// TestAgentStateTokenTracking tests the integration of agent state with token tracking.
func TestAgentStateTokenTracking(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "alphie-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "state.db")
	db, err := state.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Create session
	session := &state.Session{
		ID:          "session-001",
		RootTask:    "task-root",
		Tier:        "standard",
		TokenBudget: 100000,
		TokensUsed:  0,
		StartedAt:   time.Now(),
		Status:      state.SessionActive,
	}

	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Create token tracker
	tracker := agent.NewTokenTracker("claude-sonnet-4-20250514")

	// Simulate token usage from API
	tracker.Update(agent.MessageDeltaUsage{
		InputTokens:  5000,
		OutputTokens: 2000,
	})

	// Create agent with token usage
	now := time.Now()
	agentState := &state.Agent{
		ID:         "agent-001",
		TaskID:     "task-001",
		Status:     state.AgentRunning,
		StartedAt:  &now,
		TokensUsed: int(tracker.GetUsage().TotalTokens),
		Cost:       tracker.GetCost(),
	}

	if err := db.CreateAgent(agentState); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	// Retrieve and verify agent state
	retrieved, err := db.GetAgent("agent-001")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetAgent() returned nil")
	}

	if retrieved.TokensUsed != 7000 {
		t.Errorf("TokensUsed = %d, want 7000", retrieved.TokensUsed)
	}

	// Verify cost calculation
	expectedCost := tracker.GetCost()
	if retrieved.Cost != expectedCost {
		t.Errorf("Cost = %f, want %f", retrieved.Cost, expectedCost)
	}

	// Update session with token usage
	session.TokensUsed = retrieved.TokensUsed
	if err := db.UpdateSession(session); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}

	// Verify session update
	updatedSession, err := db.GetSession("session-001")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if updatedSession.TokensUsed != 7000 {
		t.Errorf("Session TokensUsed = %d, want 7000", updatedSession.TokensUsed)
	}
}

// TestAggregateTokenTracking tests tracking tokens across multiple agents.
func TestAggregateTokenTracking(t *testing.T) {
	// Create aggregate tracker
	aggregate := agent.NewAggregateTracker()

	// Create individual trackers for different agents
	tracker1 := agent.NewTokenTracker("claude-sonnet-4-20250514")
	tracker1.Update(agent.MessageDeltaUsage{InputTokens: 1000, OutputTokens: 500})

	tracker2 := agent.NewTokenTracker("claude-sonnet-4-20250514")
	tracker2.Update(agent.MessageDeltaUsage{InputTokens: 2000, OutputTokens: 1000})

	tracker3 := agent.NewTokenTracker("claude-sonnet-4-20250514")
	tracker3.Update(agent.MessageDeltaUsage{InputTokens: 3000, OutputTokens: 1500})

	// Add to aggregate
	aggregate.Add("agent-1", tracker1)
	aggregate.Add("agent-2", tracker2)
	aggregate.Add("agent-3", tracker3)

	// Verify aggregate count
	if aggregate.Count() != 3 {
		t.Errorf("Count() = %d, want 3", aggregate.Count())
	}

	// Verify aggregate usage
	totalUsage := aggregate.GetUsage()
	expectedInput := int64(6000)  // 1000 + 2000 + 3000
	expectedOutput := int64(3000) // 500 + 1000 + 1500

	if totalUsage.InputTokens != expectedInput {
		t.Errorf("InputTokens = %d, want %d", totalUsage.InputTokens, expectedInput)
	}
	if totalUsage.OutputTokens != expectedOutput {
		t.Errorf("OutputTokens = %d, want %d", totalUsage.OutputTokens, expectedOutput)
	}

	// Verify aggregate cost
	individualCosts := tracker1.GetCost() + tracker2.GetCost() + tracker3.GetCost()
	aggregateCost := aggregate.GetCost()
	if aggregateCost != individualCosts {
		t.Errorf("Aggregate cost = %f, want %f", aggregateCost, individualCosts)
	}

	// Remove an agent and verify
	aggregate.Remove("agent-2")
	if aggregate.Count() != 2 {
		t.Errorf("After removal, Count() = %d, want 2", aggregate.Count())
	}

	// Verify new totals
	newUsage := aggregate.GetUsage()
	if newUsage.InputTokens != 4000 { // 1000 + 3000
		t.Errorf("After removal, InputTokens = %d, want 4000", newUsage.InputTokens)
	}
}

// TestTokenConfidenceTracking tests confidence tracking for hard vs soft tokens.
func TestTokenConfidenceTracking(t *testing.T) {
	tracker := agent.NewTokenTracker("claude-sonnet-4-20250514")

	// Initially confidence should be 1.0
	if tracker.GetConfidence() != 1.0 {
		t.Errorf("Initial confidence = %f, want 1.0", tracker.GetConfidence())
	}

	// Add hard tokens (from API)
	tracker.Update(agent.MessageDeltaUsage{InputTokens: 1000, OutputTokens: 500})

	// Confidence should still be 1.0 (all hard)
	if tracker.GetConfidence() != 1.0 {
		t.Errorf("After hard tokens, confidence = %f, want 1.0", tracker.GetConfidence())
	}

	// Add soft tokens (estimated)
	tracker.UpdateSoft(500, 250)

	// Confidence should be hard/(hard+soft) = 1500/2250 = 0.666...
	expectedConfidence := 1500.0 / 2250.0
	tolerance := 0.001
	actualConfidence := tracker.GetConfidence()
	if actualConfidence < expectedConfidence-tolerance || actualConfidence > expectedConfidence+tolerance {
		t.Errorf("After soft tokens, confidence = %f, want ~%f", actualConfidence, expectedConfidence)
	}
}

// TestSessionTaskWorkflow tests the workflow of session -> task -> agent -> completion.
func TestSessionTaskWorkflow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "alphie-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "state.db")
	db, err := state.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Step 1: Create session
	session := &state.Session{
		ID:          "workflow-session",
		RootTask:    "workflow-root",
		Tier:        "standard",
		TokenBudget: 50000,
		TokensUsed:  0,
		StartedAt:   time.Now(),
		Status:      state.SessionActive,
	}
	if err := db.CreateSession(session); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	// Step 2: Create task
	task := &state.Task{
		ID:          "workflow-task",
		ParentID:    "workflow-root",
		Title:       "Implement feature",
		Description: "Full feature implementation",
		Status:      state.TaskPending,
		DependsOn:   []string{},
		Tier:        "standard",
		CreatedAt:   time.Now(),
	}
	if err := db.CreateTask(task); err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}

	// Step 3: Create agent to work on task
	now := time.Now()
	agentState := &state.Agent{
		ID:           "workflow-agent",
		TaskID:       "workflow-task",
		Status:       state.AgentRunning,
		WorktreePath: "/tmp/worktree",
		StartedAt:    &now,
	}
	if err := db.CreateAgent(agentState); err != nil {
		t.Fatalf("CreateAgent() error = %v", err)
	}

	// Step 4: Update task to in progress
	task.Status = state.TaskInProgress
	task.AssignedTo = "workflow-agent"
	if err := db.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	// Step 5: Simulate work and token usage
	agentState.TokensUsed = 15000
	agentState.Cost = 0.06 // ~$0.06
	agentState.RalphIter = 2
	agentState.RalphScore = 8
	if err := db.UpdateAgent(agentState); err != nil {
		t.Fatalf("UpdateAgent() error = %v", err)
	}

	// Step 6: Complete agent and task
	agentState.Status = state.AgentDone
	if err := db.UpdateAgent(agentState); err != nil {
		t.Fatalf("UpdateAgent() error = %v", err)
	}

	completedAt := time.Now()
	task.Status = state.TaskDone
	task.CompletedAt = &completedAt
	if err := db.UpdateTask(task); err != nil {
		t.Fatalf("UpdateTask() error = %v", err)
	}

	// Step 7: Update session
	session.TokensUsed = agentState.TokensUsed
	session.Status = state.SessionCompleted
	if err := db.UpdateSession(session); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}

	// Verify final states
	finalSession, err := db.GetSession("workflow-session")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if finalSession.Status != state.SessionCompleted {
		t.Errorf("Session status = %s, want completed", finalSession.Status)
	}
	if finalSession.TokensUsed != 15000 {
		t.Errorf("Session TokensUsed = %d, want 15000", finalSession.TokensUsed)
	}

	finalTask, err := db.GetTask("workflow-task")
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if finalTask.Status != state.TaskDone {
		t.Errorf("Task status = %s, want done", finalTask.Status)
	}

	finalAgent, err := db.GetAgent("workflow-agent")
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if finalAgent.RalphScore != 8 {
		t.Errorf("Agent RalphScore = %d, want 8", finalAgent.RalphScore)
	}
}

// TestMultiAgentParallelTracking tests tracking multiple agents running in parallel.
func TestMultiAgentParallelTracking(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "alphie-integration-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "state.db")
	db, err := state.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	// Create multiple running agents
	now := time.Now()
	agents := []*state.Agent{
		{ID: "parallel-1", TaskID: "task-1", Status: state.AgentRunning, StartedAt: &now, TokensUsed: 5000},
		{ID: "parallel-2", TaskID: "task-2", Status: state.AgentRunning, StartedAt: &now, TokensUsed: 7000},
		{ID: "parallel-3", TaskID: "task-3", Status: state.AgentRunning, StartedAt: &now, TokensUsed: 3000},
	}

	for _, a := range agents {
		if err := db.CreateAgent(a); err != nil {
			t.Fatalf("CreateAgent(%s) error = %v", a.ID, err)
		}
	}

	// List running agents
	status := state.AgentRunning
	running, err := db.ListAgents(&status)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}

	if len(running) != 3 {
		t.Errorf("ListAgents(running) = %d, want 3", len(running))
	}

	// Calculate total tokens across agents
	var totalTokens int
	for _, a := range running {
		totalTokens += a.TokensUsed
	}

	if totalTokens != 15000 {
		t.Errorf("Total tokens = %d, want 15000", totalTokens)
	}

	// Complete one agent
	agents[0].Status = state.AgentDone
	if err := db.UpdateAgent(agents[0]); err != nil {
		t.Fatalf("UpdateAgent() error = %v", err)
	}

	// List running agents again
	running, err = db.ListAgents(&status)
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}

	if len(running) != 2 {
		t.Errorf("After completion, ListAgents(running) = %d, want 2", len(running))
	}
}
