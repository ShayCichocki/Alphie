package agent

import (
	"strings"
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestNewCritiquePrompt_ThresholdClamping(t *testing.T) {
	tests := []struct {
		name      string
		threshold int
		want      int
	}{
		{
			name:      "valid threshold",
			threshold: 7,
			want:      7,
		},
		{
			name:      "minimum threshold",
			threshold: 1,
			want:      1,
		},
		{
			name:      "maximum threshold",
			threshold: 9,
			want:      9,
		},
		{
			name:      "below minimum",
			threshold: 0,
			want:      1,
		},
		{
			name:      "negative",
			threshold: -5,
			want:      1,
		},
		{
			name:      "above maximum",
			threshold: 10,
			want:      9,
		},
		{
			name:      "way above maximum",
			threshold: 100,
			want:      9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cp := NewCritiquePrompt(tt.threshold)
			if cp.Threshold() != tt.want {
				t.Errorf("Threshold() = %d, want %d", cp.Threshold(), tt.want)
			}
		})
	}
}

func TestCritiquePrompt_GetPromptTemplate(t *testing.T) {
	cp := NewCritiquePrompt(7)
	template := cp.GetPromptTemplate()

	// Check that the template contains key elements
	requiredElements := []string{
		"CORRECTNESS",
		"READABILITY",
		"EDGE CASES",
		"Total: X/9",
		"7/9", // threshold
		"DONE",
	}

	for _, elem := range requiredElements {
		if !strings.Contains(template, elem) {
			t.Errorf("Template missing required element: %s", elem)
		}
	}
}

func TestCritiquePrompt_InjectCritiquePrompt(t *testing.T) {
	cp := NewCritiquePrompt(7)
	response := "I implemented the feature."
	injected := cp.InjectCritiquePrompt(response)

	// Check that response is preserved
	if !strings.Contains(injected, response) {
		t.Error("Injected prompt should contain original response")
	}

	// Check that separator is present
	if !strings.Contains(injected, "---") {
		t.Error("Injected prompt should contain separator")
	}

	// Check that critique prompt is appended
	if !strings.Contains(injected, "CORRECTNESS") {
		t.Error("Injected prompt should contain critique template")
	}
}

func TestParseCritiqueResponse_IsDone(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantDone bool
	}{
		{
			name:     "DONE marker",
			response: "Everything looks good. DONE",
			wantDone: true,
		},
		{
			name:     "done lowercase",
			response: "All checks pass. done.",
			wantDone: true,
		},
		{
			name:     "Done mixed case",
			response: "Done",
			wantDone: true,
		},
		{
			name:     "no done marker",
			response: "Need to fix some issues.",
			wantDone: false,
		},
		{
			name:     "done inside word",
			response: "We are not abandoned yet.",
			wantDone: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCritiqueResponse(tt.response)
			if err != nil {
				t.Fatalf("ParseCritiqueResponse() error = %v", err)
			}
			if result.IsDone != tt.wantDone {
				t.Errorf("IsDone = %v, want %v", result.IsDone, tt.wantDone)
			}
		})
	}
}

func TestParseCritiqueResponse_ExtractScores(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		wantCorrect int
		wantRead    int
		wantEdge    int
	}{
		{
			name: "standard format",
			response: `CORRECTNESS (1-3): 3
READABILITY (1-3): 2
EDGE CASES (1-3): 2

Total: 7/9`,
			wantCorrect: 3,
			wantRead:    2,
			wantEdge:    2,
		},
		{
			name: "with descriptions",
			response: `CORRECTNESS (1-3): 2
- Works for happy path
- Some edge case bugs

READABILITY (1-3): 3
- Clear code

EDGE CASES (1-3): 1
- Missing null checks`,
			wantCorrect: 2,
			wantRead:    3,
			wantEdge:    1,
		},
		{
			name: "scores at start of line",
			response: `CORRECTNESS: 3
READABILITY: 2
EDGE CASES: 2`,
			wantCorrect: 3,
			wantRead:    2,
			wantEdge:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseCritiqueResponse(tt.response)
			if err != nil {
				t.Fatalf("ParseCritiqueResponse() error = %v", err)
			}
			if result.Score.Correctness != tt.wantCorrect {
				t.Errorf("Correctness = %d, want %d", result.Score.Correctness, tt.wantCorrect)
			}
			if result.Score.Readability != tt.wantRead {
				t.Errorf("Readability = %d, want %d", result.Score.Readability, tt.wantRead)
			}
			if result.Score.EdgeCases != tt.wantEdge {
				t.Errorf("EdgeCases = %d, want %d", result.Score.EdgeCases, tt.wantEdge)
			}
		})
	}
}

func TestParseCritiqueResponse_ExtractImprovements(t *testing.T) {
	response := `CORRECTNESS (1-3): 2
READABILITY (1-3): 2
EDGE CASES (1-3): 1

Improvements needed:
- Add null check for input parameter
- Handle empty array case
- Add input validation

Total: 5/9`

	result, err := ParseCritiqueResponse(response)
	if err != nil {
		t.Fatalf("ParseCritiqueResponse() error = %v", err)
	}

	// Should extract non-rubric improvements
	if len(result.Improvements) < 3 {
		t.Errorf("Expected at least 3 improvements, got %d", len(result.Improvements))
	}

	// Check specific improvements are extracted
	found := false
	for _, imp := range result.Improvements {
		if strings.Contains(imp, "null check") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'null check' improvement to be extracted")
	}
}

func TestParseCritiqueResponse_FilterRubricQuestions(t *testing.T) {
	// Response that includes rubric questions as bullet points
	response := `CORRECTNESS (1-3): 2
- Does it work for the happy path? Yes
- Does it handle edge cases? No
- Are there obvious bugs? No

READABILITY (1-3): 2
EDGE CASES (1-3): 1`

	result, err := ParseCritiqueResponse(response)
	if err != nil {
		t.Fatalf("ParseCritiqueResponse() error = %v", err)
	}

	// Rubric questions should be filtered out
	for _, imp := range result.Improvements {
		if strings.Contains(imp, "happy path") ||
			strings.Contains(imp, "obvious bugs") {
			t.Errorf("Rubric question should be filtered: %s", imp)
		}
	}
}

func TestCritiqueResult_Total(t *testing.T) {
	result := &CritiqueResult{
		Score: models.RubricScore{
			Correctness: 3,
			Readability: 2,
			EdgeCases:   2,
		},
	}

	if result.Total() != 7 {
		t.Errorf("Total() = %d, want 7", result.Total())
	}
}

func TestCritiqueResult_PassesThreshold(t *testing.T) {
	tests := []struct {
		name      string
		score     models.RubricScore
		threshold int
		want      bool
	}{
		{
			name:      "passes",
			score:     models.RubricScore{Correctness: 3, Readability: 2, EdgeCases: 2},
			threshold: 7,
			want:      true,
		},
		{
			name:      "fails",
			score:     models.RubricScore{Correctness: 2, Readability: 2, EdgeCases: 2},
			threshold: 7,
			want:      false,
		},
		{
			name:      "exactly at threshold",
			score:     models.RubricScore{Correctness: 3, Readability: 2, EdgeCases: 2},
			threshold: 7,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &CritiqueResult{Score: tt.score}
			if got := result.PassesThreshold(tt.threshold); got != tt.want {
				t.Errorf("PassesThreshold(%d) = %v, want %v", tt.threshold, got, tt.want)
			}
		})
	}
}

func TestThresholdForTier(t *testing.T) {
	tests := []struct {
		name string
		tier models.Tier
		want int
	}{
		{
			name: "Scout",
			tier: models.TierScout,
			want: 5,
		},
		{
			name: "Builder",
			tier: models.TierBuilder,
			want: 7,
		},
		{
			name: "Architect",
			tier: models.TierArchitect,
			want: 8,
		},
		{
			name: "Unknown",
			tier: models.Tier("unknown"),
			want: DefaultThreshold,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ThresholdForTier(tt.tier); got != tt.want {
				t.Errorf("ThresholdForTier() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMaxIterationsForTier(t *testing.T) {
	tests := []struct {
		name string
		tier models.Tier
		want int
	}{
		{
			name: "Scout",
			tier: models.TierScout,
			want: 3,
		},
		{
			name: "Builder",
			tier: models.TierBuilder,
			want: 5,
		},
		{
			name: "Architect",
			tier: models.TierArchitect,
			want: 7,
		},
		{
			name: "Unknown",
			tier: models.Tier("unknown"),
			want: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MaxIterationsForTier(tt.tier); got != tt.want {
				t.Errorf("MaxIterationsForTier() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsRubricQuestion(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"happy path question", "Does it work for the happy path?", true},
		{"edge cases question", "Does it handle edge cases?", true},
		{"obvious bugs question", "Are there obvious bugs?", true},
		{"code clarity question", "Is the code clear without comments?", true},
		{"names descriptive question", "Are names descriptive?", true},
		{"complexity question", "Is complexity appropriate?", true},
		{"errors handled question", "Are errors handled?", true},
		{"nulls/empty question", "Are nulls/empty states handled?", true},
		{"boundaries question", "Are boundaries checked?", true},
		{"null check improvement", "Add null check for input", false},
		{"validation improvement", "Fix the validation logic", false},
		{"error message improvement", "Improve error messages", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRubricQuestion(tt.text); got != tt.want {
				t.Errorf("isRubricQuestion(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
