package models

import (
	"testing"
	"time"
)

func TestTaskStatus_Valid(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{"pending is valid", TaskStatusPending, true},
		{"in_progress is valid", TaskStatusInProgress, true},
		{"blocked is valid", TaskStatusBlocked, true},
		{"done is valid", TaskStatusDone, true},
		{"failed is valid", TaskStatusFailed, true},
		{"empty string is invalid", TaskStatus(""), false},
		{"unknown status is invalid", TaskStatus("unknown"), false},
		{"typo status is invalid", TaskStatus("pendingg"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("TaskStatus(%q).Valid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestTaskStatus_StringValues(t *testing.T) {
	// Verify the string values are as expected
	tests := []struct {
		status TaskStatus
		want   string
	}{
		{TaskStatusPending, "pending"},
		{TaskStatusInProgress, "in_progress"},
		{TaskStatusBlocked, "blocked"},
		{TaskStatusDone, "done"},
		{TaskStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := string(tt.status); got != tt.want {
				t.Errorf("string(TaskStatus) = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTask_DefaultValues(t *testing.T) {
	task := Task{}

	if task.ID != "" {
		t.Errorf("Task.ID default should be empty string, got %q", task.ID)
	}
	if task.ParentID != "" {
		t.Errorf("Task.ParentID default should be empty string, got %q", task.ParentID)
	}
	if task.Title != "" {
		t.Errorf("Task.Title default should be empty string, got %q", task.Title)
	}
	if task.Status != "" {
		t.Errorf("Task.Status default should be empty string, got %q", task.Status)
	}
	if task.DependsOn != nil {
		t.Errorf("Task.DependsOn default should be nil, got %v", task.DependsOn)
	}
	if task.CompletedAt != nil {
		t.Errorf("Task.CompletedAt default should be nil, got %v", task.CompletedAt)
	}
	if !task.CreatedAt.IsZero() {
		t.Errorf("Task.CreatedAt default should be zero time, got %v", task.CreatedAt)
	}
}

func TestTask_Fields(t *testing.T) {
	now := time.Now()
	completedAt := now.Add(time.Hour)

	task := Task{
		ID:                 "task-123",
		ParentID:           "epic-456",
		Title:              "Test Task",
		Description:        "A test task description",
		AcceptanceCriteria: "Tests pass",
		Status:             TaskStatusInProgress,
		DependsOn:          []string{"task-100", "task-101"},
		AssignedTo:         "agent-789",
		Tier:               TierBuilder,
		CreatedAt:          now,
		CompletedAt:        &completedAt,
	}

	if task.ID != "task-123" {
		t.Errorf("Task.ID = %q, want %q", task.ID, "task-123")
	}
	if task.ParentID != "epic-456" {
		t.Errorf("Task.ParentID = %q, want %q", task.ParentID, "epic-456")
	}
	if task.Title != "Test Task" {
		t.Errorf("Task.Title = %q, want %q", task.Title, "Test Task")
	}
	if task.Description != "A test task description" {
		t.Errorf("Task.Description = %q, want %q", task.Description, "A test task description")
	}
	if task.AcceptanceCriteria != "Tests pass" {
		t.Errorf("Task.AcceptanceCriteria = %q, want %q", task.AcceptanceCriteria, "Tests pass")
	}
	if task.Status != TaskStatusInProgress {
		t.Errorf("Task.Status = %q, want %q", task.Status, TaskStatusInProgress)
	}
	if len(task.DependsOn) != 2 {
		t.Errorf("Task.DependsOn length = %d, want 2", len(task.DependsOn))
	}
	if task.AssignedTo != "agent-789" {
		t.Errorf("Task.AssignedTo = %q, want %q", task.AssignedTo, "agent-789")
	}
	if task.Tier != TierBuilder {
		t.Errorf("Task.Tier = %q, want %q", task.Tier, TierBuilder)
	}
	if !task.CreatedAt.Equal(now) {
		t.Errorf("Task.CreatedAt = %v, want %v", task.CreatedAt, now)
	}
	if task.CompletedAt == nil || !task.CompletedAt.Equal(completedAt) {
		t.Errorf("Task.CompletedAt = %v, want %v", task.CompletedAt, completedAt)
	}
}

func TestRubricScore_Valid(t *testing.T) {
	tests := []struct {
		name  string
		score RubricScore
		want  bool
	}{
		{"all minimum valid", RubricScore{1, 1, 1}, true},
		{"all maximum valid", RubricScore{3, 3, 3}, true},
		{"mixed valid", RubricScore{1, 2, 3}, true},
		{"correctness zero", RubricScore{0, 1, 1}, false},
		{"readability zero", RubricScore{1, 0, 1}, false},
		{"edge_cases zero", RubricScore{1, 1, 0}, false},
		{"correctness too high", RubricScore{4, 1, 1}, false},
		{"readability too high", RubricScore{1, 4, 1}, false},
		{"edge_cases too high", RubricScore{1, 1, 4}, false},
		{"all zeros", RubricScore{0, 0, 0}, false},
		{"all too high", RubricScore{4, 4, 4}, false},
		{"negative value", RubricScore{-1, 1, 1}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.score.Valid(); got != tt.want {
				t.Errorf("RubricScore%+v.Valid() = %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}

func TestRubricScore_Total(t *testing.T) {
	tests := []struct {
		name  string
		score RubricScore
		want  int
	}{
		{"all minimum", RubricScore{1, 1, 1}, 3},
		{"all maximum", RubricScore{3, 3, 3}, 9},
		{"mixed values", RubricScore{1, 2, 3}, 6},
		{"all twos", RubricScore{2, 2, 2}, 6},
		{"zeros", RubricScore{0, 0, 0}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.score.Total(); got != tt.want {
				t.Errorf("RubricScore%+v.Total() = %d, want %d", tt.score, got, tt.want)
			}
		})
	}
}

func TestRubricScore_DefaultValues(t *testing.T) {
	score := RubricScore{}

	if score.Correctness != 0 {
		t.Errorf("RubricScore.Correctness default should be 0, got %d", score.Correctness)
	}
	if score.Readability != 0 {
		t.Errorf("RubricScore.Readability default should be 0, got %d", score.Readability)
	}
	if score.EdgeCases != 0 {
		t.Errorf("RubricScore.EdgeCases default should be 0, got %d", score.EdgeCases)
	}
	if score.Total() != 0 {
		t.Errorf("RubricScore{}.Total() should be 0, got %d", score.Total())
	}
	if score.Valid() {
		t.Error("RubricScore{} should not be valid")
	}
}

func TestSession_DefaultValues(t *testing.T) {
	session := Session{}

	if session.ID != "" {
		t.Errorf("Session.ID default should be empty string, got %q", session.ID)
	}
	if session.RootTask != "" {
		t.Errorf("Session.RootTask default should be empty string, got %q", session.RootTask)
	}
	if session.Tier != "" {
		t.Errorf("Session.Tier default should be empty string, got %q", session.Tier)
	}
	if session.TokenBudget != 0 {
		t.Errorf("Session.TokenBudget default should be 0, got %d", session.TokenBudget)
	}
	if session.TokensUsed != 0 {
		t.Errorf("Session.TokensUsed default should be 0, got %d", session.TokensUsed)
	}
	if !session.StartedAt.IsZero() {
		t.Errorf("Session.StartedAt default should be zero time, got %v", session.StartedAt)
	}
	if session.Status != "" {
		t.Errorf("Session.Status default should be empty string, got %q", session.Status)
	}
}

func TestSession_Fields(t *testing.T) {
	now := time.Now()

	session := Session{
		ID:          "session-123",
		RootTask:    "task-456",
		Tier:        TierArchitect,
		TokenBudget: 100000,
		TokensUsed:  50000,
		StartedAt:   now,
		Status:      TaskStatusInProgress,
	}

	if session.ID != "session-123" {
		t.Errorf("Session.ID = %q, want %q", session.ID, "session-123")
	}
	if session.RootTask != "task-456" {
		t.Errorf("Session.RootTask = %q, want %q", session.RootTask, "task-456")
	}
	if session.Tier != TierArchitect {
		t.Errorf("Session.Tier = %q, want %q", session.Tier, TierArchitect)
	}
	if session.TokenBudget != 100000 {
		t.Errorf("Session.TokenBudget = %d, want %d", session.TokenBudget, 100000)
	}
	if session.TokensUsed != 50000 {
		t.Errorf("Session.TokensUsed = %d, want %d", session.TokensUsed, 50000)
	}
	if !session.StartedAt.Equal(now) {
		t.Errorf("Session.StartedAt = %v, want %v", session.StartedAt, now)
	}
	if session.Status != TaskStatusInProgress {
		t.Errorf("Session.Status = %q, want %q", session.Status, TaskStatusInProgress)
	}
}
