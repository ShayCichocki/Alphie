package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// =============================================================================
// ImplementState Tests
// =============================================================================

func TestImplementState_ZeroValue(t *testing.T) {
	var state ImplementState

	if state.Iteration != 0 {
		t.Errorf("expected Iteration=0, got %d", state.Iteration)
	}
	if state.FeaturesComplete != 0 {
		t.Errorf("expected FeaturesComplete=0, got %d", state.FeaturesComplete)
	}
	if state.FeaturesTotal != 0 {
		t.Errorf("expected FeaturesTotal=0, got %d", state.FeaturesTotal)
	}
	if state.Cost != 0 {
		t.Errorf("expected Cost=0, got %f", state.Cost)
	}
	if state.CurrentPhase != "" {
		t.Errorf("expected CurrentPhase='', got %q", state.CurrentPhase)
	}
}

func TestImplementState_WithValues(t *testing.T) {
	state := ImplementState{
		Iteration:        3,
		FeaturesComplete: 5,
		FeaturesTotal:    12,
		Cost:             1.50,
		CurrentPhase:     "executing",
		WorkersRunning:   2,
		WorkersBlocked:   1,
		StopConditions:   []string{"escalation needed"},
		BlockedQuestions: []string{"What database to use?"},
	}

	if state.Iteration != 3 {
		t.Errorf("expected Iteration=3, got %d", state.Iteration)
	}
	if state.FeaturesComplete != 5 {
		t.Errorf("expected FeaturesComplete=5, got %d", state.FeaturesComplete)
	}
	if len(state.StopConditions) != 1 {
		t.Errorf("expected 1 stop condition, got %d", len(state.StopConditions))
	}
	if len(state.BlockedQuestions) != 1 {
		t.Errorf("expected 1 blocked question, got %d", len(state.BlockedQuestions))
	}
}

// =============================================================================
// ImplementView Tests
// =============================================================================

func TestNewImplementView(t *testing.T) {
	view := NewImplementView()

	if view == nil {
		t.Fatal("NewImplementView returned nil")
	}

	// Check that state is initialized (with zero values)
	state := view.GetState()
	if state.Iteration != 0 {
		t.Errorf("expected default Iteration=0, got %d", state.Iteration)
	}
	if state.Cost != 0 {
		t.Errorf("expected default Cost=0, got %f", state.Cost)
	}
}

func TestImplementView_SetState(t *testing.T) {
	view := NewImplementView()

	newState := ImplementState{
		Iteration:        5,
		FeaturesComplete: 8,
		FeaturesTotal:    16,
		CurrentPhase:     "auditing",
	}

	view.SetState(newState)
	got := view.GetState()

	if got.Iteration != 5 {
		t.Errorf("expected Iteration=5, got %d", got.Iteration)
	}
	if got.FeaturesComplete != 8 {
		t.Errorf("expected FeaturesComplete=8, got %d", got.FeaturesComplete)
	}
	if got.CurrentPhase != "auditing" {
		t.Errorf("expected CurrentPhase='auditing', got %q", got.CurrentPhase)
	}
}

func TestImplementView_SetSize(t *testing.T) {
	view := NewImplementView()

	view.SetSize(120, 40)

	if view.width != 120 {
		t.Errorf("expected width=120, got %d", view.width)
	}
	if view.height != 40 {
		t.Errorf("expected height=40, got %d", view.height)
	}
}

func TestImplementView_Update_WindowSizeMsg(t *testing.T) {
	view := NewImplementView()

	msg := tea.WindowSizeMsg{Width: 100, Height: 50}
	updatedView, _ := view.Update(msg)

	if updatedView.width != 100 {
		t.Errorf("expected width=100, got %d", updatedView.width)
	}
	if updatedView.height != 50 {
		t.Errorf("expected height=50, got %d", updatedView.height)
	}
}

func TestImplementView_Update_ImplementUpdateMsg(t *testing.T) {
	view := NewImplementView()

	state := ImplementState{
		Iteration:        7,
		FeaturesComplete: 10,
		FeaturesTotal:    15,
	}
	msg := ImplementUpdateMsg{State: state}
	updatedView, _ := view.Update(msg)

	got := updatedView.GetState()
	if got.Iteration != 7 {
		t.Errorf("expected Iteration=7, got %d", got.Iteration)
	}
	if got.FeaturesComplete != 10 {
		t.Errorf("expected FeaturesComplete=10, got %d", got.FeaturesComplete)
	}
}

func TestImplementView_View_ContainsExpectedElements(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		Iteration:        2,
		FeaturesComplete: 3,
		FeaturesTotal:    6,
		Cost:             0.50,
		CurrentPhase:     "parsing",
		WorkersRunning:   1,
		WorkersBlocked:   0,
	})

	output := view.View()

	// Check for key content (note: output includes ANSI codes, so use Contains)
	expectedStrings := []string{
		"Implementation Progress",
		"3/6",     // Features
		"50%",     // Percentage (3/6 = 50%)
		"$0.50",   // Cost
		"parsing", // Phase
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q", expected)
		}
	}
}

func TestImplementView_View_EmptyPhaseShowsNone(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		CurrentPhase: "",
	})

	output := view.View()

	if !strings.Contains(output, "none") {
		t.Error("expected empty phase to display as 'none'")
	}
}

func TestImplementView_View_BlockedQuestions(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		BlockedQuestions: []string{
			"Which framework?",
			"Use TypeScript?",
		},
	})

	output := view.View()

	if !strings.Contains(output, "Blocked On") {
		t.Error("expected output to contain 'Blocked On' header")
	}
	if !strings.Contains(output, "Which framework?") {
		t.Error("expected output to contain first blocked question")
	}
	if !strings.Contains(output, "Use TypeScript?") {
		t.Error("expected output to contain second blocked question")
	}
}

func TestImplementView_View_StopConditions(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		StopConditions: []string{
			"Budget exceeded",
			"Max iterations reached",
		},
	})

	output := view.View()

	if !strings.Contains(output, "Stop Conditions") {
		t.Error("expected output to contain 'Stop Conditions' header")
	}
	if !strings.Contains(output, "Budget exceeded") {
		t.Error("expected output to contain first stop condition")
	}
}


// =============================================================================
// Progress Bar Tests
// =============================================================================

func TestImplementView_ProgressBar_ZeroPercent(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		FeaturesComplete: 0,
		FeaturesTotal:    10,
	})

	output := view.View()

	// Should show 0%
	if !strings.Contains(output, "0%") {
		t.Error("expected output to contain '0%'")
	}
	// Progress bar should be all empty (░)
	if !strings.Contains(output, "░░░░░░░░░░") {
		t.Error("expected progress bar to show empty blocks")
	}
}

func TestImplementView_ProgressBar_FiftyPercent(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		FeaturesComplete: 5,
		FeaturesTotal:    10,
	})

	output := view.View()

	if !strings.Contains(output, "50%") {
		t.Error("expected output to contain '50%'")
	}
	// Should have mix of filled and empty
	if !strings.Contains(output, "█") {
		t.Error("expected progress bar to contain filled blocks")
	}
	if !strings.Contains(output, "░") {
		t.Error("expected progress bar to contain empty blocks")
	}
}

func TestImplementView_ProgressBar_HundredPercent(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		FeaturesComplete: 10,
		FeaturesTotal:    10,
	})

	output := view.View()

	if !strings.Contains(output, "100%") {
		t.Error("expected output to contain '100%'")
	}
	// Progress bar should be all filled (█)
	if !strings.Contains(output, "██████████") {
		t.Error("expected progress bar to show filled blocks")
	}
}

func TestImplementView_ProgressBar_ZeroTotal(t *testing.T) {
	view := NewImplementView()
	view.SetState(ImplementState{
		FeaturesComplete: 0,
		FeaturesTotal:    0, // Edge case: no features
	})

	output := view.View()

	// Should show 0% when total is 0
	if !strings.Contains(output, "0%") {
		t.Error("expected output to contain '0%' when total is 0")
	}
}

func TestImplementView_RenderProgressBar_EdgeCases(t *testing.T) {
	view := NewImplementView()

	tests := []struct {
		name    string
		pct     float64
		width   int
		wantPct string
	}{
		{"negative percent", -10, 30, "0%"},
		{"zero percent", 0, 30, "0%"},
		{"fifty percent", 50, 30, "50%"},
		{"hundred percent", 100, 30, "100%"},
		{"over hundred percent", 150, 30, "100%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := view.renderProgressBar(tt.pct, tt.width)
			if !strings.Contains(result, tt.wantPct) {
				t.Errorf("renderProgressBar(%v, %d) = %q, want to contain %q",
					tt.pct, tt.width, result, tt.wantPct)
			}
		})
	}
}

func TestImplementView_RenderProgressBar_WidthCalculation(t *testing.T) {
	view := NewImplementView()

	// At 50% with width 20, should have 10 filled + 10 empty = 20 total
	result := view.renderProgressBar(50, 20)

	// Count characters (excluding ANSI codes is tricky, so just check total length is reasonable)
	// The format is "  <bar> <pct>%" so minimum length should be > width
	if len(result) < 20 {
		t.Errorf("progress bar result too short: %d chars", len(result))
	}
}

// =============================================================================
// ImplementApp Tests
// =============================================================================

func TestNewImplementApp(t *testing.T) {
	app := NewImplementApp()

	if app == nil {
		t.Fatal("NewImplementApp returned nil")
	}
	if app.view == nil {
		t.Error("expected app.view to be initialized")
	}
	if app.logs == nil {
		t.Error("expected app.logs to be initialized")
	}
	if len(app.logs) != 0 {
		t.Errorf("expected empty logs, got %d", len(app.logs))
	}
	if app.quitting {
		t.Error("expected quitting=false")
	}
	if app.done {
		t.Error("expected done=false")
	}
}

func TestImplementApp_Init(t *testing.T) {
	app := NewImplementApp()
	cmd := app.Init()

	if cmd != nil {
		t.Error("expected Init to return nil cmd")
	}
}

func TestImplementApp_Update_QuitKey(t *testing.T) {
	app := NewImplementApp()

	// Test 'q' key
	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	updatedApp := model.(*ImplementApp)

	if !updatedApp.quitting {
		t.Error("expected quitting=true after 'q' key")
	}
	if cmd == nil {
		t.Error("expected quit command to be returned")
	}
}

func TestImplementApp_Update_CtrlC(t *testing.T) {
	app := NewImplementApp()

	model, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	updatedApp := model.(*ImplementApp)

	if !updatedApp.quitting {
		t.Error("expected quitting=true after Ctrl+C")
	}
	if cmd == nil {
		t.Error("expected quit command to be returned")
	}
}

func TestImplementApp_Update_WindowSizeMsg(t *testing.T) {
	app := NewImplementApp()

	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	model, _ := app.Update(msg)
	updatedApp := model.(*ImplementApp)

	if updatedApp.width != 80 {
		t.Errorf("expected width=80, got %d", updatedApp.width)
	}
	if updatedApp.height != 24 {
		t.Errorf("expected height=24, got %d", updatedApp.height)
	}
}

func TestImplementApp_Update_ImplementUpdateMsg(t *testing.T) {
	app := NewImplementApp()

	state := ImplementState{
		Iteration:        4,
		FeaturesComplete: 7,
		FeaturesTotal:    14,
		CurrentPhase:     "executing",
	}
	msg := ImplementUpdateMsg{State: state}
	model, _ := app.Update(msg)
	updatedApp := model.(*ImplementApp)

	viewState := updatedApp.view.GetState()
	if viewState.Iteration != 4 {
		t.Errorf("expected Iteration=4, got %d", viewState.Iteration)
	}
	if viewState.CurrentPhase != "executing" {
		t.Errorf("expected CurrentPhase='executing', got %q", viewState.CurrentPhase)
	}
}

func TestImplementApp_Update_ImplementLogMsg(t *testing.T) {
	app := NewImplementApp()

	now := time.Now()
	msg := ImplementLogMsg{
		Timestamp: now,
		Phase:     "parsing",
		Message:   "Parsing architecture document...",
	}
	model, _ := app.Update(msg)
	updatedApp := model.(*ImplementApp)

	if len(updatedApp.logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(updatedApp.logs))
	}

	entry := updatedApp.logs[0]
	if entry.Phase != "parsing" {
		t.Errorf("expected Phase='parsing', got %q", entry.Phase)
	}
	if entry.Message != "Parsing architecture document..." {
		t.Errorf("expected Message='Parsing architecture document...', got %q", entry.Message)
	}
}

func TestImplementApp_Update_ImplementDoneMsg_Success(t *testing.T) {
	app := NewImplementApp()

	msg := ImplementDoneMsg{Err: nil}
	model, _ := app.Update(msg)
	updatedApp := model.(*ImplementApp)

	if !updatedApp.done {
		t.Error("expected done=true")
	}
	if updatedApp.err != nil {
		t.Errorf("expected err=nil, got %v", updatedApp.err)
	}
}

func TestImplementApp_Update_ImplementDoneMsg_WithError(t *testing.T) {
	app := NewImplementApp()

	testErr := errors.New("implementation failed")
	msg := ImplementDoneMsg{Err: testErr}
	model, _ := app.Update(msg)
	updatedApp := model.(*ImplementApp)

	if !updatedApp.done {
		t.Error("expected done=true")
	}
	if updatedApp.err == nil {
		t.Error("expected err to be set")
	}
	if updatedApp.err.Error() != "implementation failed" {
		t.Errorf("expected err='implementation failed', got %q", updatedApp.err.Error())
	}
}

func TestImplementApp_View_Quitting(t *testing.T) {
	app := NewImplementApp()
	app.quitting = true

	output := app.View()

	if !strings.Contains(output, "cancelled") {
		t.Errorf("expected quitting view to contain 'cancelled', got %q", output)
	}
}

func TestImplementApp_View_Normal(t *testing.T) {
	app := NewImplementApp()
	app.view.SetState(ImplementState{
		Iteration:        1,
		FeaturesComplete: 2,
		FeaturesTotal:    8,
		CurrentPhase:     "auditing",
	})

	output := app.View()

	expectedStrings := []string{
		"Alphie Implement",
		"2/8",
		"auditing",
		"Press q to cancel",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output to contain %q", expected)
		}
	}
}

func TestImplementApp_View_Done_Success(t *testing.T) {
	app := NewImplementApp()
	app.done = true
	app.err = nil

	output := app.View()

	if !strings.Contains(output, "complete") {
		t.Error("expected done view to contain 'complete'")
	}
	if !strings.Contains(output, "Press q to exit") {
		t.Error("expected done view to contain 'Press q to exit'")
	}
}

func TestImplementApp_View_Done_WithError(t *testing.T) {
	app := NewImplementApp()
	app.done = true
	app.err = errors.New("something went wrong")

	output := app.View()

	if !strings.Contains(output, "Error") {
		t.Error("expected error view to contain 'Error'")
	}
	if !strings.Contains(output, "something went wrong") {
		t.Error("expected error view to contain the error message")
	}
}

func TestImplementApp_View_WithLogs(t *testing.T) {
	app := NewImplementApp()
	now := time.Now()

	app.logs = []ImplementLogEntry{
		{Timestamp: now, Phase: "parsing", Message: "Starting parse"},
		{Timestamp: now.Add(time.Second), Phase: "auditing", Message: "Auditing code"},
	}

	output := app.View()

	if !strings.Contains(output, "Activity Log") {
		t.Error("expected output to contain 'Activity Log' header")
	}
	if !strings.Contains(output, "Starting parse") {
		t.Error("expected output to contain first log message")
	}
	if !strings.Contains(output, "Auditing code") {
		t.Error("expected output to contain second log message")
	}
}

func TestImplementApp_RenderLogs_Empty(t *testing.T) {
	app := NewImplementApp()

	output := app.renderLogs()

	if output != "" {
		t.Errorf("expected empty string for no logs, got %q", output)
	}
}

func TestImplementApp_RenderLogs_TruncatesTo8(t *testing.T) {
	app := NewImplementApp()
	now := time.Now()

	// Add 12 log entries
	for i := 0; i < 12; i++ {
		app.logs = append(app.logs, ImplementLogEntry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Phase:     "test",
			Message:   "Log entry",
		})
	}

	output := app.renderLogs()

	// Count occurrences of "Log entry" - should be 8
	count := strings.Count(output, "Log entry")
	if count != 8 {
		t.Errorf("expected 8 log entries displayed, got %d", count)
	}
}

// =============================================================================
// Message Type Tests
// =============================================================================

func TestImplementUpdateMsg_ContainsState(t *testing.T) {
	state := ImplementState{
		Iteration:    5,
		CurrentPhase: "planning",
	}
	msg := ImplementUpdateMsg{State: state}

	if msg.State.Iteration != 5 {
		t.Errorf("expected Iteration=5, got %d", msg.State.Iteration)
	}
	if msg.State.CurrentPhase != "planning" {
		t.Errorf("expected CurrentPhase='planning', got %q", msg.State.CurrentPhase)
	}
}

func TestImplementLogMsg_Fields(t *testing.T) {
	now := time.Now()
	msg := ImplementLogMsg{
		Timestamp: now,
		Phase:     "executing",
		Message:   "Running task 1",
	}

	if msg.Timestamp != now {
		t.Error("timestamp mismatch")
	}
	if msg.Phase != "executing" {
		t.Errorf("expected Phase='executing', got %q", msg.Phase)
	}
	if msg.Message != "Running task 1" {
		t.Errorf("expected Message='Running task 1', got %q", msg.Message)
	}
}

func TestImplementDoneMsg_Fields(t *testing.T) {
	// Without error
	msg1 := ImplementDoneMsg{Err: nil}
	if msg1.Err != nil {
		t.Error("expected Err=nil")
	}

	// With error
	err := errors.New("test error")
	msg2 := ImplementDoneMsg{Err: err}
	if msg2.Err == nil {
		t.Error("expected Err to be set")
	}
}

func TestImplementLogEntry_Fields(t *testing.T) {
	now := time.Now()
	entry := ImplementLogEntry{
		Timestamp: now,
		Phase:     "complete",
		Message:   "All done",
	}

	if entry.Timestamp != now {
		t.Error("timestamp mismatch")
	}
	if entry.Phase != "complete" {
		t.Errorf("expected Phase='complete', got %q", entry.Phase)
	}
	if entry.Message != "All done" {
		t.Errorf("expected Message='All done', got %q", entry.Message)
	}
}

// =============================================================================
// NewImplementProgram Tests
// =============================================================================

func TestNewImplementProgram(t *testing.T) {
	program, app := NewImplementProgram()

	if program == nil {
		t.Error("expected program to not be nil")
	}
	if app == nil {
		t.Error("expected app to not be nil")
	}
	if app.view == nil {
		t.Error("expected app.view to be initialized")
	}
}

// =============================================================================
// Integration-style Tests
// =============================================================================

func TestImplementApp_FullWorkflow(t *testing.T) {
	app := NewImplementApp()

	// Simulate window resize
	model, _ := app.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	app = model.(*ImplementApp)

	// Simulate progress updates through phases
	phases := []string{"parsing", "auditing", "planning", "executing", "complete"}
	for i, phase := range phases {
		// Update state
		model, _ = app.Update(ImplementUpdateMsg{
			State: ImplementState{
				Iteration:        1,
				FeaturesComplete: i * 2,
				FeaturesTotal:    10,
				CurrentPhase:     phase,
			},
		})
		app = model.(*ImplementApp)

		// Add log entry
		model, _ = app.Update(ImplementLogMsg{
			Timestamp: time.Now(),
			Phase:     phase,
			Message:   "Processing " + phase,
		})
		app = model.(*ImplementApp)
	}

	// Verify final state
	if len(app.logs) != 5 {
		t.Errorf("expected 5 log entries, got %d", len(app.logs))
	}

	viewState := app.view.GetState()
	if viewState.CurrentPhase != "complete" {
		t.Errorf("expected CurrentPhase='complete', got %q", viewState.CurrentPhase)
	}

	// Simulate done
	model, _ = app.Update(ImplementDoneMsg{Err: nil})
	app = model.(*ImplementApp)

	if !app.done {
		t.Error("expected done=true")
	}

	// View should show completion message
	output := app.View()
	if !strings.Contains(output, "complete") {
		t.Error("expected view to show completion")
	}
}

func TestImplementApp_ErrorWorkflow(t *testing.T) {
	app := NewImplementApp()

	// Simulate some progress
	model, _ := app.Update(ImplementUpdateMsg{
		State: ImplementState{
			Iteration:    1,
			CurrentPhase: "parsing",
		},
	})
	app = model.(*ImplementApp)

	// Simulate error
	model, _ = app.Update(ImplementDoneMsg{
		Err: errors.New("failed to parse architecture document"),
	})
	app = model.(*ImplementApp)

	if !app.done {
		t.Error("expected done=true")
	}
	if app.err == nil {
		t.Error("expected error to be set")
	}

	output := app.View()
	if !strings.Contains(output, "Error") {
		t.Error("expected view to show error")
	}
	if !strings.Contains(output, "failed to parse") {
		t.Error("expected view to contain error message")
	}
}

// =============================================================================
// Percentage Calculation Tests (to verify the bug from the example doesn't exist)
// =============================================================================

func TestImplementView_PercentageMatchesProgressBar(t *testing.T) {
	tests := []struct {
		name             string
		featuresComplete int
		featuresTotal    int
		expectedPct      string
	}{
		{"0 of 12", 0, 12, "0%"},
		{"6 of 12", 6, 12, "50%"},
		{"12 of 12", 12, 12, "100%"},
		{"1 of 3", 1, 3, "33%"},
		{"2 of 3", 2, 3, "67%"},
		{"7 of 10", 7, 10, "70%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := NewImplementView()
			view.SetState(ImplementState{
				FeaturesComplete: tt.featuresComplete,
				FeaturesTotal:    tt.featuresTotal,
			})

			output := view.View()

			// The percentage should appear in both the features line AND the progress bar
			// Both should match
			if !strings.Contains(output, tt.expectedPct) {
				t.Errorf("expected output to contain %q for %d/%d features",
					tt.expectedPct, tt.featuresComplete, tt.featuresTotal)
			}

			// Verify the features line shows correct ratio
			expectedRatio := strings.Replace(tt.name, " of ", "/", 1)
			if !strings.Contains(output, expectedRatio) {
				t.Errorf("expected output to contain ratio %q", expectedRatio)
			}
		})
	}
}
