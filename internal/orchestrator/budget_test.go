package orchestrator

import (
	"testing"
)

func TestBudgetHandler_StatusTransitions(t *testing.T) {
	tests := []struct {
		name           string
		budget         int64
		used           int64
		expectedStatus BudgetStatus
	}{
		{
			name:           "OK - 0% usage",
			budget:         1000,
			used:           0,
			expectedStatus: BudgetOK,
		},
		{
			name:           "OK - 50% usage",
			budget:         1000,
			used:           500,
			expectedStatus: BudgetOK,
		},
		{
			name:           "OK - just under threshold (79%)",
			budget:         1000,
			used:           790,
			expectedStatus: BudgetOK,
		},
		{
			name:           "Warning - at threshold (80%)",
			budget:         1000,
			used:           800,
			expectedStatus: BudgetWarning,
		},
		{
			name:           "Warning - 90% usage",
			budget:         1000,
			used:           900,
			expectedStatus: BudgetWarning,
		},
		{
			name:           "Warning - 99% usage",
			budget:         1000,
			used:           990,
			expectedStatus: BudgetWarning,
		},
		{
			name:           "Exhausted - 100% usage",
			budget:         1000,
			used:           1000,
			expectedStatus: BudgetExhausted,
		},
		{
			name:           "Exhausted - over budget (110%)",
			budget:         1000,
			used:           1100,
			expectedStatus: BudgetExhausted,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewBudgetHandler(tc.budget)
			handler.Update(tc.used)

			status := handler.CheckBudget()
			if status != tc.expectedStatus {
				t.Errorf("expected status %v, got %v", tc.expectedStatus, status)
			}
		})
	}
}

func TestBudgetHandler_ThresholdChecking(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Default threshold should be 80%
	if handler.GetWarningThreshold() != DefaultWarningThreshold {
		t.Errorf("expected default threshold %v, got %v", DefaultWarningThreshold, handler.GetWarningThreshold())
	}

	// At 79%, should be OK
	handler.Update(790)
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK at 79%")
	}

	// At 80%, should be Warning
	handler.Update(10) // Now at 800
	if handler.CheckBudget() != BudgetWarning {
		t.Error("expected BudgetWarning at 80%")
	}
}

func TestBudgetHandler_CustomThreshold(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Set custom threshold to 50%
	handler.SetWarningThreshold(0.5)

	// At 49%, should be OK
	handler.Update(490)
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK at 49% with 50% threshold")
	}

	// At 50%, should be Warning
	handler.Update(10) // Now at 500
	if handler.CheckBudget() != BudgetWarning {
		t.Error("expected BudgetWarning at 50% with 50% threshold")
	}
}

func TestBudgetHandler_ThresholdClamping(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Set threshold below 0
	handler.SetWarningThreshold(-0.5)
	if handler.GetWarningThreshold() != 0 {
		t.Errorf("expected threshold clamped to 0, got %v", handler.GetWarningThreshold())
	}

	// Set threshold above 1
	handler.SetWarningThreshold(1.5)
	if handler.GetWarningThreshold() != 1 {
		t.Errorf("expected threshold clamped to 1, got %v", handler.GetWarningThreshold())
	}
}

func TestBudgetHandler_ZeroBudget(t *testing.T) {
	handler := NewBudgetHandler(0)

	// With zero budget, should always return OK (no limit set)
	handler.Update(1000)
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK with zero budget (no limit)")
	}

	if !handler.CanStartNew() {
		t.Error("expected CanStartNew to be true with zero budget")
	}
}

func TestBudgetHandler_NegativeBudget(t *testing.T) {
	handler := NewBudgetHandler(-100)

	// Negative budget should behave like no limit
	handler.Update(1000)
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK with negative budget")
	}
}

func TestBudgetHandler_CanStartNew(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Should be able to start new when OK
	if !handler.CanStartNew() {
		t.Error("expected CanStartNew true when BudgetOK")
	}

	// Should be able to start new when Warning
	handler.Update(900)
	if !handler.CanStartNew() {
		t.Error("expected CanStartNew true when BudgetWarning")
	}

	// Should NOT be able to start new when Exhausted
	handler.Update(200) // Now at 1100
	if handler.CanStartNew() {
		t.Error("expected CanStartNew false when BudgetExhausted")
	}
}

func TestBudgetHandler_OnExhausted(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Initially not exhausted
	if handler.IsExhausted() {
		t.Error("expected IsExhausted false initially")
	}

	// Call OnExhausted
	handler.OnExhausted()

	// Now should be exhausted
	if !handler.IsExhausted() {
		t.Error("expected IsExhausted true after OnExhausted")
	}
}

func TestBudgetHandler_OnExhausted_Idempotent(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Call OnExhausted multiple times
	handler.OnExhausted()
	handler.OnExhausted()
	handler.OnExhausted()

	// Should still be exhausted (idempotent)
	if !handler.IsExhausted() {
		t.Error("expected IsExhausted true after multiple OnExhausted calls")
	}
}

func TestBudgetHandler_Reset(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Use some budget and mark exhausted
	handler.Update(1100)
	handler.OnExhausted()

	// Verify state before reset
	if handler.CheckBudget() != BudgetExhausted {
		t.Error("expected BudgetExhausted before reset")
	}
	if !handler.IsExhausted() {
		t.Error("expected IsExhausted true before reset")
	}

	// Reset
	handler.Reset()

	// Verify state after reset
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK after reset")
	}
	if handler.IsExhausted() {
		t.Error("expected IsExhausted false after reset")
	}
}

func TestBudgetHandler_GetUsage(t *testing.T) {
	handler := NewBudgetHandler(1000)
	handler.Update(250)

	used, budget, percentage := handler.GetUsage()

	if used != 250 {
		t.Errorf("expected used 250, got %d", used)
	}
	if budget != 1000 {
		t.Errorf("expected budget 1000, got %d", budget)
	}
	if percentage != 0.25 {
		t.Errorf("expected percentage 0.25, got %v", percentage)
	}
}

func TestBudgetHandler_GetUsage_ZeroBudget(t *testing.T) {
	handler := NewBudgetHandler(0)
	handler.Update(100)

	used, budget, percentage := handler.GetUsage()

	if used != 100 {
		t.Errorf("expected used 100, got %d", used)
	}
	if budget != 0 {
		t.Errorf("expected budget 0, got %d", budget)
	}
	if percentage != 0.0 {
		t.Errorf("expected percentage 0.0 with zero budget, got %v", percentage)
	}
}

func TestBudgetHandler_SetBudget(t *testing.T) {
	handler := NewBudgetHandler(1000)
	handler.Update(500) // 50% used

	// Should be OK at 50%
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK at 50%")
	}

	// Reduce budget so 500 is now 100%
	handler.SetBudget(500)

	// Should now be Exhausted
	if handler.CheckBudget() != BudgetExhausted {
		t.Error("expected BudgetExhausted after reducing budget")
	}
}

func TestBudgetStatus_String(t *testing.T) {
	tests := []struct {
		status   BudgetStatus
		expected string
	}{
		{BudgetOK, "OK"},
		{BudgetWarning, "Warning"},
		{BudgetExhausted, "Exhausted"},
		{BudgetStatus(99), "Unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if tc.status.String() != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, tc.status.String())
			}
		})
	}
}

func TestNewBudgetHandler(t *testing.T) {
	handler := NewBudgetHandler(5000)

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}

	used, budget, _ := handler.GetUsage()
	if used != 0 {
		t.Errorf("expected initial used 0, got %d", used)
	}
	if budget != 5000 {
		t.Errorf("expected budget 5000, got %d", budget)
	}
	if handler.GetWarningThreshold() != DefaultWarningThreshold {
		t.Errorf("expected default threshold, got %v", handler.GetWarningThreshold())
	}
}

func TestBudgetHandler_IncrementalUpdate(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Update incrementally
	handler.Update(100)
	handler.Update(200)
	handler.Update(300)

	used, _, _ := handler.GetUsage()
	if used != 600 {
		t.Errorf("expected total used 600, got %d", used)
	}

	// Should be OK at 60%
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK at 60%")
	}
}

func TestBudgetHandler_TransitionFromOKToWarningToExhausted(t *testing.T) {
	handler := NewBudgetHandler(1000)

	// Start at OK
	if handler.CheckBudget() != BudgetOK {
		t.Error("expected BudgetOK initially")
	}

	// Move to Warning (80%)
	handler.Update(800)
	if handler.CheckBudget() != BudgetWarning {
		t.Error("expected BudgetWarning at 80%")
	}

	// Move to Exhausted (100%)
	handler.Update(200)
	if handler.CheckBudget() != BudgetExhausted {
		t.Error("expected BudgetExhausted at 100%")
	}
}

func TestDefaultWarningThreshold(t *testing.T) {
	if DefaultWarningThreshold != 0.80 {
		t.Errorf("expected DefaultWarningThreshold to be 0.80, got %v", DefaultWarningThreshold)
	}
}
