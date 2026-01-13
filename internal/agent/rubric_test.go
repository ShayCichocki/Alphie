package agent

import (
	"testing"

	"github.com/shayc/alphie/pkg/models"
)

func TestParseScore_ValidResponse(t *testing.T) {
	tests := []struct {
		name        string
		response    string
		wantCorrect int
		wantRead    int
		wantEdge    int
	}{
		{
			name: "standard format",
			response: `CORRECTNESS: 3
READABILITY: 2
EDGE CASES: 2`,
			wantCorrect: 3,
			wantRead:    2,
			wantEdge:    2,
		},
		{
			name: "with /3 suffix",
			response: `CORRECTNESS: 3/3
READABILITY: 2/3
EDGE CASES: 1/3`,
			wantCorrect: 3,
			wantRead:    2,
			wantEdge:    1,
		},
		{
			name: "lowercase",
			response: `correctness: 2
readability: 3
edge cases: 1`,
			wantCorrect: 2,
			wantRead:    3,
			wantEdge:    1,
		},
		{
			name: "with underscore",
			response: `CORRECTNESS: 3
READABILITY: 3
EDGE_CASES: 3`,
			wantCorrect: 3,
			wantRead:    3,
			wantEdge:    3,
		},
		{
			name: "mixed case and extra text",
			response: `Here is my evaluation:
Correctness: 2 - some issues
Readability: 3 - well written
EdgeCases: 2 - mostly covered`,
			wantCorrect: 2,
			wantRead:    3,
			wantEdge:    2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score, err := ParseScore(tt.response)
			if err != nil {
				t.Fatalf("ParseScore() error = %v", err)
			}
			if score.Correctness != tt.wantCorrect {
				t.Errorf("Correctness = %d, want %d", score.Correctness, tt.wantCorrect)
			}
			if score.Readability != tt.wantRead {
				t.Errorf("Readability = %d, want %d", score.Readability, tt.wantRead)
			}
			if score.EdgeCases != tt.wantEdge {
				t.Errorf("EdgeCases = %d, want %d", score.EdgeCases, tt.wantEdge)
			}
		})
	}
}

func TestParseScore_Malformed(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  error
	}{
		{
			name:     "empty string",
			response: "",
			wantErr:  ErrMalformedResponse,
		},
		{
			name:     "whitespace only",
			response: "   \n\t  ",
			wantErr:  ErrMalformedResponse,
		},
		{
			name: "no scores at all",
			response: `This is just text
without any scores`,
			wantErr: ErrMissingScore,
		},
		{
			name: "garbage text",
			response: `lksjdf lkjsdf
random garbage`,
			wantErr: ErrMissingScore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseScore(tt.response)
			if err != tt.wantErr {
				t.Errorf("ParseScore() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseScore_MissingScore(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name: "missing correctness",
			response: `READABILITY: 2
EDGE CASES: 2`,
		},
		{
			name: "missing readability",
			response: `CORRECTNESS: 3
EDGE CASES: 2`,
		},
		{
			name: "missing edge cases",
			response: `CORRECTNESS: 3
READABILITY: 2`,
		},
		{
			name: "only one score",
			response: `CORRECTNESS: 3`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseScore(tt.response)
			if err != ErrMissingScore {
				t.Errorf("ParseScore() error = %v, want %v", err, ErrMissingScore)
			}
		})
	}
}

func TestParseScore_OutOfRange(t *testing.T) {
	tests := []struct {
		name     string
		response string
	}{
		{
			name: "correctness zero",
			response: `CORRECTNESS: 0
READABILITY: 2
EDGE CASES: 2`,
		},
		{
			name: "readability four",
			response: `CORRECTNESS: 2
READABILITY: 4
EDGE CASES: 2`,
		},
		{
			name: "all zeros",
			response: `CORRECTNESS: 0
READABILITY: 0
EDGE CASES: 0`,
		},
		{
			name: "score too high",
			response: `CORRECTNESS: 5
READABILITY: 2
EDGE CASES: 2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseScore(tt.response)
			if err != ErrScoreOutOfRange {
				t.Errorf("ParseScore() error = %v, want %v", err, ErrScoreOutOfRange)
			}
		})
	}
}

func TestParseScore_NegativeNotMatched(t *testing.T) {
	// Negative numbers are not matched by the regex (\d+ only matches digits)
	// so they result in ErrMissingScore, not ErrScoreOutOfRange
	response := `CORRECTNESS: 2
READABILITY: 2
EDGE CASES: -1`

	_, err := ParseScore(response)
	if err != ErrMissingScore {
		t.Errorf("ParseScore() with negative should return ErrMissingScore (pattern doesn't match), got %v", err)
	}
}

func TestParseScore_TotalFormat(t *testing.T) {
	// Response with only total (no individual scores) should fail
	response := `Total: 7/9`
	_, err := ParseScore(response)
	if err != ErrMissingScore {
		t.Errorf("ParseScore() with only total should return ErrMissingScore, got %v", err)
	}
}

func TestMeetsThreshold(t *testing.T) {
	tests := []struct {
		name      string
		score     *models.RubricScore
		threshold int
		want      bool
	}{
		{
			name:      "nil score",
			score:     nil,
			threshold: 5,
			want:      false,
		},
		{
			name:      "exactly at threshold",
			score:     &models.RubricScore{Correctness: 2, Readability: 2, EdgeCases: 3},
			threshold: 7,
			want:      true,
		},
		{
			name:      "above threshold",
			score:     &models.RubricScore{Correctness: 3, Readability: 3, EdgeCases: 3},
			threshold: 7,
			want:      true,
		},
		{
			name:      "below threshold",
			score:     &models.RubricScore{Correctness: 2, Readability: 2, EdgeCases: 2},
			threshold: 7,
			want:      false,
		},
		{
			name:      "minimum score (3)",
			score:     &models.RubricScore{Correctness: 1, Readability: 1, EdgeCases: 1},
			threshold: 3,
			want:      true,
		},
		{
			name:      "maximum score (9)",
			score:     &models.RubricScore{Correctness: 3, Readability: 3, EdgeCases: 3},
			threshold: 9,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MeetsThreshold(tt.score, tt.threshold); got != tt.want {
				t.Errorf("MeetsThreshold() = %v, want %v", got, tt.want)
			}
		})
	}
}
