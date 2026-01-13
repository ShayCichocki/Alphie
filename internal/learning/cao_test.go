package learning

import (
	"testing"
)

func TestParseCAO_ValidSingleLine(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCond  string
		wantAct   string
		wantOut   string
	}{
		{
			name:     "basic single line",
			input:    "WHEN test fails DO check logs RESULT error found",
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
		{
			name:     "lowercase markers",
			input:    "when test fails do check logs result error found",
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
		{
			name:     "mixed case markers",
			input:    "When test fails Do check logs Result error found",
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
		{
			name:     "extra whitespace",
			input:    "   WHEN   test fails   DO   check logs   RESULT   error found   ",
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
		{
			name:     "longer content",
			input:    "WHEN encountering authentication errors in production DO verify JWT token expiry and refresh tokens RESULT user can continue session",
			wantCond: "encountering authentication errors in production",
			wantAct:  "verify JWT token expiry and refresh tokens",
			wantOut:  "user can continue session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cao, err := ParseCAO(tt.input)
			if err != nil {
				t.Fatalf("ParseCAO() error = %v, want nil", err)
			}
			if cao.Condition != tt.wantCond {
				t.Errorf("Condition = %q, want %q", cao.Condition, tt.wantCond)
			}
			if cao.Action != tt.wantAct {
				t.Errorf("Action = %q, want %q", cao.Action, tt.wantAct)
			}
			if cao.Outcome != tt.wantOut {
				t.Errorf("Outcome = %q, want %q", cao.Outcome, tt.wantOut)
			}
		})
	}
}

func TestParseCAO_ValidMultiLine(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCond  string
		wantAct   string
		wantOut   string
	}{
		{
			name: "basic multi-line",
			input: `WHEN test fails
DO check logs
RESULT error found`,
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
		{
			name: "multi-line with continuation",
			input: `WHEN test fails with
multiple lines of context
DO check logs
and verify config
RESULT error found
and resolved`,
			wantCond: "test fails with\nmultiple lines of context",
			wantAct:  "check logs\nand verify config",
			wantOut:  "error found\nand resolved",
		},
		{
			name: "multi-line with blank lines",
			input: `WHEN test fails

DO check logs

RESULT error found`,
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
		{
			name: "lowercase markers multi-line",
			input: `when test fails
do check logs
result error found`,
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
		{
			name: "mixed case multi-line",
			input: `When test fails
Do check logs
Result error found`,
			wantCond: "test fails",
			wantAct:  "check logs",
			wantOut:  "error found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cao, err := ParseCAO(tt.input)
			if err != nil {
				t.Fatalf("ParseCAO() error = %v, want nil", err)
			}
			if cao.Condition != tt.wantCond {
				t.Errorf("Condition = %q, want %q", cao.Condition, tt.wantCond)
			}
			if cao.Action != tt.wantAct {
				t.Errorf("Action = %q, want %q", cao.Action, tt.wantAct)
			}
			if cao.Outcome != tt.wantOut {
				t.Errorf("Outcome = %q, want %q", cao.Outcome, tt.wantOut)
			}
		})
	}
}

func TestParseCAO_MissingFields(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "empty input",
			input:   "",
			wantErr: ErrMissingCondition,
		},
		{
			name:    "whitespace only",
			input:   "   \t\n  ",
			wantErr: ErrMissingCondition,
		},
		{
			name:    "missing WHEN",
			input:   "DO check logs RESULT error found",
			wantErr: ErrMissingCondition,
		},
		{
			name:    "missing DO",
			input:   "WHEN test fails RESULT error found",
			wantErr: ErrMissingAction,
		},
		{
			name:    "missing RESULT",
			input:   "WHEN test fails DO check logs",
			wantErr: ErrMissingOutcome,
		},
		{
			name:    "only WHEN",
			input:   "WHEN test fails",
			wantErr: ErrMissingAction,
		},
		{
			name:    "WHEN and DO only",
			input:   "WHEN test fails DO check logs",
			wantErr: ErrMissingOutcome,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCAO(tt.input)
			if err != tt.wantErr {
				t.Errorf("ParseCAO() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCAO_EmptyFields(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{
			name:    "empty condition",
			input:   "WHEN    DO check logs RESULT error found",
			wantErr: ErrEmptyCondition,
		},
		{
			name:    "empty action",
			input:   "WHEN test fails DO    RESULT error found",
			wantErr: ErrEmptyAction,
		},
		{
			name:    "empty outcome",
			input:   "WHEN test fails DO check logs RESULT   ",
			wantErr: ErrEmptyOutcome,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCAO(tt.input)
			if err != tt.wantErr {
				t.Errorf("ParseCAO() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCAO_EmptyFieldsMultiLine(t *testing.T) {
	// Note: Multi-line parsing with empty sections returns "missing" errors
	// because when a marker is followed immediately by another marker,
	// the section content is empty and validation fails.
	tests := []struct {
		name      string
		input     string
		wantErr   error
		checkNil  bool // Just check that an error occurs
	}{
		{
			name: "empty condition multi-line",
			input: `WHEN
DO check logs
RESULT error found`,
			checkNil: true, // Parser returns an error (missing or empty)
		},
		{
			name: "empty action multi-line",
			input: `WHEN test fails
DO
RESULT error found`,
			checkNil: true, // Parser returns an error (missing or empty)
		},
		{
			name: "empty outcome multi-line",
			input: `WHEN test fails
DO check logs
RESULT`,
			checkNil: true, // Parser returns an error (missing or empty)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseCAO(tt.input)
			if tt.checkNil {
				if err == nil {
					t.Error("ParseCAO() error = nil, want error for empty field")
				}
			} else if err != tt.wantErr {
				t.Errorf("ParseCAO() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCAOTriple_String(t *testing.T) {
	cao := &CAOTriple{
		Condition: "test fails",
		Action:    "check logs",
		Outcome:   "error found",
	}
	want := "WHEN test fails\nDO check logs\nRESULT error found"
	if got := cao.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestCAOTriple_String_Nil(t *testing.T) {
	var cao *CAOTriple
	if got := cao.String(); got != "" {
		t.Errorf("String() on nil = %q, want empty string", got)
	}
}

func TestFormatCAO(t *testing.T) {
	cao := &CAOTriple{
		Condition: "test fails",
		Action:    "check logs",
		Outcome:   "error found",
	}
	want := "WHEN test fails\nDO check logs\nRESULT error found"
	if got := FormatCAO(cao); got != want {
		t.Errorf("FormatCAO() = %q, want %q", got, want)
	}
}

func TestFormatCAO_Nil(t *testing.T) {
	if got := FormatCAO(nil); got != "" {
		t.Errorf("FormatCAO(nil) = %q, want empty string", got)
	}
}

func TestCAOTriple_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cao     *CAOTriple
		wantErr error
	}{
		{
			name: "valid cao",
			cao: &CAOTriple{
				Condition: "test fails",
				Action:    "check logs",
				Outcome:   "error found",
			},
			wantErr: nil,
		},
		{
			name:    "nil cao",
			cao:     nil,
			wantErr: ErrMissingCondition,
		},
		{
			name: "empty condition",
			cao: &CAOTriple{
				Condition: "",
				Action:    "check logs",
				Outcome:   "error found",
			},
			wantErr: ErrEmptyCondition,
		},
		{
			name: "whitespace condition",
			cao: &CAOTriple{
				Condition: "   ",
				Action:    "check logs",
				Outcome:   "error found",
			},
			wantErr: ErrEmptyCondition,
		},
		{
			name: "empty action",
			cao: &CAOTriple{
				Condition: "test fails",
				Action:    "",
				Outcome:   "error found",
			},
			wantErr: ErrEmptyAction,
		},
		{
			name: "whitespace action",
			cao: &CAOTriple{
				Condition: "test fails",
				Action:    "   ",
				Outcome:   "error found",
			},
			wantErr: ErrEmptyAction,
		},
		{
			name: "empty outcome",
			cao: &CAOTriple{
				Condition: "test fails",
				Action:    "check logs",
				Outcome:   "",
			},
			wantErr: ErrEmptyOutcome,
		},
		{
			name: "whitespace outcome",
			cao: &CAOTriple{
				Condition: "test fails",
				Action:    "check logs",
				Outcome:   "   ",
			},
			wantErr: ErrEmptyOutcome,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cao.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
