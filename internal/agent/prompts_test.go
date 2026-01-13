package agent

import (
	"strings"
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestScopeGuidancePromptContent(t *testing.T) {
	// Verify the prompt contains key instructions
	requiredPhrases := []string{
		"Stay focused on this task",
		"refactoring opportunities",
		"note them as new tasks but do not implement",
		"prog add",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(ScopeGuidancePrompt, phrase) {
			t.Errorf("ScopeGuidancePrompt missing required phrase: %q", phrase)
		}
	}
}

func TestScopeGuidancePromptNonEmpty(t *testing.T) {
	if ScopeGuidancePrompt == "" {
		t.Error("ScopeGuidancePrompt should not be empty")
	}

	if len(ScopeGuidancePrompt) < 100 {
		t.Error("ScopeGuidancePrompt seems too short to provide meaningful guidance")
	}
}

func TestBuildPromptIncludesScopeGuidance(t *testing.T) {
	executor := &Executor{
		model: "claude-sonnet-4-20250514",
	}

	task := &models.Task{
		ID:          "test-task-123",
		Title:       "Test task",
		Description: "A test task description",
	}

	prompt := executor.buildPrompt(task, models.TierBuilder, nil)

	// Verify scope guidance is included at the start
	if !strings.HasPrefix(prompt, "## Scope Guidance") {
		t.Error("buildPrompt should start with scope guidance")
	}

	// Verify key scope guidance content is present
	if !strings.Contains(prompt, "Stay focused on this task") {
		t.Error("buildPrompt should include scope guidance about staying focused")
	}

	if !strings.Contains(prompt, "note them as new tasks") {
		t.Error("buildPrompt should include guidance about noting discoveries")
	}
}

func TestBuildPromptScopeGuidanceBeforeTaskInfo(t *testing.T) {
	executor := &Executor{
		model: "claude-sonnet-4-20250514",
	}

	task := &models.Task{
		ID:    "test-task-456",
		Title: "Another test task",
	}

	prompt := executor.buildPrompt(task, models.TierScout, nil)

	// Scope guidance should come before task ID
	scopeIndex := strings.Index(prompt, "## Scope Guidance")
	taskIndex := strings.Index(prompt, "Task ID:")

	if scopeIndex == -1 {
		t.Fatal("Scope guidance not found in prompt")
	}
	if taskIndex == -1 {
		t.Fatal("Task ID not found in prompt")
	}
	if scopeIndex >= taskIndex {
		t.Error("Scope guidance should appear before task information")
	}
}
