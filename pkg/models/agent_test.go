package models

import (
	"testing"
	"time"
)

func TestAgentStatus_Valid(t *testing.T) {
	tests := []struct {
		name   string
		status AgentStatus
		want   bool
	}{
		{"pending is valid", AgentStatusPending, true},
		{"running is valid", AgentStatusRunning, true},
		{"paused is valid", AgentStatusPaused, true},
		{"waiting_approval is valid", AgentStatusWaitingApproval, true},
		{"done is valid", AgentStatusDone, true},
		{"failed is valid", AgentStatusFailed, true},
		{"empty string is invalid", AgentStatus(""), false},
		{"unknown status is invalid", AgentStatus("unknown"), false},
		{"typo status is invalid", AgentStatus("runnning"), false},
		{"similar to task status is invalid", AgentStatus("in_progress"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("AgentStatus(%q).Valid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestAgentStatus_StringValues(t *testing.T) {
	// Verify the string values are as expected
	tests := []struct {
		status AgentStatus
		want   string
	}{
		{AgentStatusPending, "pending"},
		{AgentStatusRunning, "running"},
		{AgentStatusPaused, "paused"},
		{AgentStatusWaitingApproval, "waiting_approval"},
		{AgentStatusDone, "done"},
		{AgentStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := string(tt.status); got != tt.want {
				t.Errorf("string(AgentStatus) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgent_DefaultValues(t *testing.T) {
	agent := Agent{}

	if agent.ID != "" {
		t.Errorf("Agent.ID default should be empty string, got %q", agent.ID)
	}
	if agent.TaskID != "" {
		t.Errorf("Agent.TaskID default should be empty string, got %q", agent.TaskID)
	}
	if agent.Status != "" {
		t.Errorf("Agent.Status default should be empty string, got %q", agent.Status)
	}
	if agent.WorktreePath != "" {
		t.Errorf("Agent.WorktreePath default should be empty string, got %q", agent.WorktreePath)
	}
	if agent.PID != 0 {
		t.Errorf("Agent.PID default should be 0, got %d", agent.PID)
	}
	if !agent.StartedAt.IsZero() {
		t.Errorf("Agent.StartedAt default should be zero time, got %v", agent.StartedAt)
	}
	if agent.TokensUsed != 0 {
		t.Errorf("Agent.TokensUsed default should be 0, got %d", agent.TokensUsed)
	}
	if agent.Cost != 0.0 {
		t.Errorf("Agent.Cost default should be 0.0, got %f", agent.Cost)
	}
	if agent.RalphIter != 0 {
		t.Errorf("Agent.RalphIter default should be 0, got %d", agent.RalphIter)
	}
	if agent.RalphScore != nil {
		t.Errorf("Agent.RalphScore default should be nil, got %v", agent.RalphScore)
	}
}

func TestAgent_Fields(t *testing.T) {
	now := time.Now()
	score := &RubricScore{3, 2, 3}

	agent := Agent{
		ID:           "agent-123",
		TaskID:       "task-456",
		Status:       AgentStatusRunning,
		WorktreePath: "/path/to/worktree",
		PID:          12345,
		StartedAt:    now,
		TokensUsed:   50000,
		Cost:         1.25,
		RalphIter:    2,
		RalphScore:   score,
	}

	if agent.ID != "agent-123" {
		t.Errorf("Agent.ID = %q, want %q", agent.ID, "agent-123")
	}
	if agent.TaskID != "task-456" {
		t.Errorf("Agent.TaskID = %q, want %q", agent.TaskID, "task-456")
	}
	if agent.Status != AgentStatusRunning {
		t.Errorf("Agent.Status = %q, want %q", agent.Status, AgentStatusRunning)
	}
	if agent.WorktreePath != "/path/to/worktree" {
		t.Errorf("Agent.WorktreePath = %q, want %q", agent.WorktreePath, "/path/to/worktree")
	}
	if agent.PID != 12345 {
		t.Errorf("Agent.PID = %d, want %d", agent.PID, 12345)
	}
	if !agent.StartedAt.Equal(now) {
		t.Errorf("Agent.StartedAt = %v, want %v", agent.StartedAt, now)
	}
	if agent.TokensUsed != 50000 {
		t.Errorf("Agent.TokensUsed = %d, want %d", agent.TokensUsed, 50000)
	}
	if agent.Cost != 1.25 {
		t.Errorf("Agent.Cost = %f, want %f", agent.Cost, 1.25)
	}
	if agent.RalphIter != 2 {
		t.Errorf("Agent.RalphIter = %d, want %d", agent.RalphIter, 2)
	}
	if agent.RalphScore == nil || agent.RalphScore.Total() != 8 {
		t.Errorf("Agent.RalphScore.Total() = %d, want %d", agent.RalphScore.Total(), 8)
	}
}

func TestAgent_WithRubricScore(t *testing.T) {
	tests := []struct {
		name      string
		score     *RubricScore
		wantTotal int
		wantValid bool
	}{
		{
			name:      "nil score",
			score:     nil,
			wantTotal: 0,
			wantValid: false,
		},
		{
			name:      "minimum valid score",
			score:     &RubricScore{1, 1, 1},
			wantTotal: 3,
			wantValid: true,
		},
		{
			name:      "maximum valid score",
			score:     &RubricScore{3, 3, 3},
			wantTotal: 9,
			wantValid: true,
		},
		{
			name:      "mixed valid score",
			score:     &RubricScore{2, 3, 1},
			wantTotal: 6,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := Agent{
				ID:         "test-agent",
				RalphScore: tt.score,
			}

			if tt.score == nil {
				if agent.RalphScore != nil {
					t.Errorf("Agent.RalphScore should be nil")
				}
				return
			}

			if got := agent.RalphScore.Total(); got != tt.wantTotal {
				t.Errorf("Agent.RalphScore.Total() = %d, want %d", got, tt.wantTotal)
			}
			if got := agent.RalphScore.Valid(); got != tt.wantValid {
				t.Errorf("Agent.RalphScore.Valid() = %v, want %v", got, tt.wantValid)
			}
		})
	}
}

func TestAgentStatus_AllStatusesAreDistinct(t *testing.T) {
	statuses := []AgentStatus{
		AgentStatusPending,
		AgentStatusRunning,
		AgentStatusPaused,
		AgentStatusWaitingApproval,
		AgentStatusDone,
		AgentStatusFailed,
	}

	seen := make(map[AgentStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Errorf("Duplicate AgentStatus: %q", s)
		}
		seen[s] = true
	}

	if len(seen) != 6 {
		t.Errorf("Expected 6 distinct AgentStatus values, got %d", len(seen))
	}
}
