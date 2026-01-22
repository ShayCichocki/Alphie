package orchestrator

import (
	"testing"

	"github.com/ShayCichocki/alphie/internal/protect"
	"github.com/ShayCichocki/alphie/pkg/models"
)

func TestScoutOverrideGate_DefaultConfig(t *testing.T) {
	cfg := DefaultScoutOverrideConfig()

	if cfg.BlockedAfterNAttempts != 5 {
		t.Errorf("expected BlockedAfterNAttempts=5, got %d", cfg.BlockedAfterNAttempts)
	}
	if !cfg.ProtectedAreaDetected {
		t.Error("expected ProtectedAreaDetected=true")
	}
}

func TestScoutOverrideGate_CanAskQuestion_NoOverride(t *testing.T) {
	gate := NewScoutOverrideGate(nil, DefaultScoutOverrideConfig())

	// By default, Scout cannot ask questions
	if gate.CanAskQuestion("task-1") {
		t.Error("expected CanAskQuestion=false for new task")
	}
}

func TestScoutOverrideGate_BlockedAfterN(t *testing.T) {
	gate := NewScoutOverrideGate(nil, ScoutOverrideConfig{
		BlockedAfterNAttempts: 3, // Lower threshold for testing
		ProtectedAreaDetected: false,
	})

	taskID := "task-blocked"

	// First 2 attempts should not allow questions
	for i := 1; i <= 2; i++ {
		gate.RecordAttempt(taskID)
		if gate.CanAskQuestion(taskID) {
			t.Errorf("expected CanAskQuestion=false after %d attempts", i)
		}
	}

	// Third attempt should trigger override
	gate.RecordAttempt(taskID)
	if !gate.CanAskQuestion(taskID) {
		t.Error("expected CanAskQuestion=true after 3 attempts")
	}

	// Verify reason
	reason := gate.GetOverrideReason(taskID)
	if reason != "blocked_after_n_attempts" {
		t.Errorf("expected reason='blocked_after_n_attempts', got '%s'", reason)
	}
}

func TestScoutOverrideGate_ProtectedArea(t *testing.T) {
	detector := protect.New()
	gate := NewScoutOverrideGate(detector, ScoutOverrideConfig{
		BlockedAfterNAttempts: 5,
		ProtectedAreaDetected: true,
	})

	taskID := "task-protected"

	// Initially no override
	if gate.CanAskQuestion(taskID) {
		t.Error("expected CanAskQuestion=false for new task")
	}

	// Check protected paths
	paths := []string{"src/auth/login.go", "src/utils/helpers.go"}
	protected := gate.CheckProtectedArea(taskID, paths)
	if !protected {
		t.Error("expected auth path to be detected as protected")
	}

	// Now should allow questions
	if !gate.CanAskQuestion(taskID) {
		t.Error("expected CanAskQuestion=true for protected area")
	}

	// Verify reason
	reason := gate.GetOverrideReason(taskID)
	if reason != "protected_area_detected" {
		t.Errorf("expected reason='protected_area_detected', got '%s'", reason)
	}
}

func TestScoutOverrideGate_ProtectedAreaDisabled(t *testing.T) {
	detector := protect.New()
	gate := NewScoutOverrideGate(detector, ScoutOverrideConfig{
		BlockedAfterNAttempts: 5,
		ProtectedAreaDetected: false, // Disabled
	})

	taskID := "task-no-override"

	// Mark as protected
	gate.SetProtectedArea(taskID, true)

	// Should not allow questions since feature is disabled
	if gate.CanAskQuestion(taskID) {
		t.Error("expected CanAskQuestion=false when protected area feature is disabled")
	}
}

func TestScoutOverrideGate_Reset(t *testing.T) {
	gate := NewScoutOverrideGate(nil, ScoutOverrideConfig{
		BlockedAfterNAttempts: 2,
		ProtectedAreaDetected: true,
	})

	taskID := "task-reset"

	// Build up state
	gate.RecordAttempt(taskID)
	gate.RecordAttempt(taskID)
	gate.SetProtectedArea(taskID, true)

	// Verify override is active
	if !gate.CanAskQuestion(taskID) {
		t.Error("expected CanAskQuestion=true before reset")
	}

	// Reset
	gate.Reset(taskID)

	// Should no longer allow questions
	if gate.CanAskQuestion(taskID) {
		t.Error("expected CanAskQuestion=false after reset")
	}
	if gate.GetAttempts(taskID) != 0 {
		t.Errorf("expected attempts=0 after reset, got %d", gate.GetAttempts(taskID))
	}
}

func TestScoutOverrideGate_GettersAndSetters(t *testing.T) {
	detector := protect.New()
	gate := NewScoutOverrideGate(detector, ScoutOverrideConfig{
		BlockedAfterNAttempts: 7,
		ProtectedAreaDetected: true,
	})

	if gate.GetBlockedAfterN() != 7 {
		t.Errorf("expected GetBlockedAfterN=7, got %d", gate.GetBlockedAfterN())
	}

	if !gate.IsProtectedAreaEnabled() {
		t.Error("expected IsProtectedAreaEnabled=true")
	}

	taskID := "task-getset"
	gate.SetProtectedArea(taskID, true)
	if !gate.IsProtectedArea(taskID) {
		t.Error("expected IsProtectedArea=true after SetProtectedArea")
	}
}

func TestQuestionsAllowed_Scout(t *testing.T) {
	gate := NewScoutOverrideGate(nil, ScoutOverrideConfig{
		BlockedAfterNAttempts: 3,
		ProtectedAreaDetected: true,
	})

	taskID := "task-scout"

	// Scout without override: 0 questions
	allowed := QuestionsAllowed(models.TierScout, gate, taskID)
	if allowed != 0 {
		t.Errorf("expected 0 questions for Scout without override, got %d", allowed)
	}

	// Trigger override
	gate.RecordAttempt(taskID)
	gate.RecordAttempt(taskID)
	gate.RecordAttempt(taskID)

	// Scout with override: 1 question
	allowed = QuestionsAllowed(models.TierScout, gate, taskID)
	if allowed != 1 {
		t.Errorf("expected 1 question for Scout with override, got %d", allowed)
	}

	// Scout without gate: always 0
	allowed = QuestionsAllowed(models.TierScout, nil, taskID)
	if allowed != 0 {
		t.Errorf("expected 0 questions for Scout without gate, got %d", allowed)
	}
}

func TestQuestionsAllowed_Builder(t *testing.T) {
	gate := NewScoutOverrideGate(nil, DefaultScoutOverrideConfig())

	// Builder always gets 2 questions, regardless of gate
	allowed := QuestionsAllowed(models.TierBuilder, gate, "task-builder")
	if allowed != 2 {
		t.Errorf("expected 2 questions for Builder, got %d", allowed)
	}

	// Even without gate
	allowed = QuestionsAllowed(models.TierBuilder, nil, "task-builder")
	if allowed != 2 {
		t.Errorf("expected 2 questions for Builder without gate, got %d", allowed)
	}
}

func TestQuestionsAllowed_Architect(t *testing.T) {
	gate := NewScoutOverrideGate(nil, DefaultScoutOverrideConfig())

	// Architect gets unlimited questions (-1)
	allowed := QuestionsAllowed(models.TierArchitect, gate, "task-architect")
	if allowed != -1 {
		t.Errorf("expected -1 (unlimited) questions for Architect, got %d", allowed)
	}
}

func TestScoutOverrideGate_ConcurrentAccess(t *testing.T) {
	gate := NewScoutOverrideGate(nil, DefaultScoutOverrideConfig())

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		taskID := "task-concurrent"
		go func() {
			gate.RecordAttempt(taskID)
			gate.CanAskQuestion(taskID)
			gate.GetAttempts(taskID)
			gate.SetProtectedArea(taskID, true)
			gate.IsProtectedArea(taskID)
			gate.GetOverrideReason(taskID)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without deadlock or panic, concurrent access is safe
}

func TestScoutOverrideGate_CheckProtectedArea_NilDetector(t *testing.T) {
	gate := NewScoutOverrideGate(nil, DefaultScoutOverrideConfig())

	// Should return false without panic
	result := gate.CheckProtectedArea("task-nil", []string{"src/auth/login.go"})
	if result {
		t.Error("expected false with nil detector")
	}
}

func TestScoutOverrideGate_BlockedAfterN_ZeroConfig(t *testing.T) {
	// Test that zero config defaults to 5
	gate := NewScoutOverrideGate(nil, ScoutOverrideConfig{
		BlockedAfterNAttempts: 0,
		ProtectedAreaDetected: true,
	})

	if gate.GetBlockedAfterN() != 5 {
		t.Errorf("expected default BlockedAfterN=5 for zero config, got %d", gate.GetBlockedAfterN())
	}
}
