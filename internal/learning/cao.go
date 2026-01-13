package learning

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// CAOTriple represents a Condition-Action-Outcome learning triple.
// It captures a specific context (WHEN), the action taken (DO),
// and the resulting outcome (RESULT).
type CAOTriple struct {
	Condition string // WHEN - the context or trigger condition
	Action    string // DO - the action to take
	Outcome   string // RESULT - the expected outcome
}

var (
	// ErrMissingCondition indicates the WHEN clause is missing.
	ErrMissingCondition = errors.New("cao: missing WHEN condition")
	// ErrMissingAction indicates the DO clause is missing.
	ErrMissingAction = errors.New("cao: missing DO action")
	// ErrMissingOutcome indicates the RESULT clause is missing.
	ErrMissingOutcome = errors.New("cao: missing RESULT outcome")
	// ErrEmptyCondition indicates the WHEN clause is empty.
	ErrEmptyCondition = errors.New("cao: empty WHEN condition")
	// ErrEmptyAction indicates the DO clause is empty.
	ErrEmptyAction = errors.New("cao: empty DO action")
	// ErrEmptyOutcome indicates the RESULT clause is empty.
	ErrEmptyOutcome = errors.New("cao: empty RESULT outcome")
)

// markerPattern matches WHEN, DO, or RESULT markers (case-insensitive)
var markerPattern = regexp.MustCompile(`(?i)^(WHEN|DO|RESULT)\s+`)

// ParseCAO parses a CAO triple from input text.
// It supports both single-line and multi-line formats.
//
// Single-line format:
//
//	WHEN condition DO action RESULT outcome
//
// Multi-line format:
//
//	WHEN condition text
//	DO action text
//	RESULT outcome text
//
// Markers are case-insensitive.
func ParseCAO(input string) (*CAOTriple, error) {
	if strings.TrimSpace(input) == "" {
		return nil, ErrMissingCondition
	}

	// Normalize input: trim and convert to lines
	lines := strings.Split(input, "\n")

	// For single-line input, try the single-line parser first
	// This handles the case where all markers are on one line
	if len(lines) == 1 {
		return parseSingleLine(input)
	}

	// Build sections by finding markers
	sections := make(map[string][]string) // marker -> content lines
	var currentMarker string
	var currentContent []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			// Preserve blank lines within sections
			if currentMarker != "" {
				currentContent = append(currentContent, "")
			}
			continue
		}

		// Check if line starts with a marker
		match := markerPattern.FindStringSubmatch(trimmed)
		if match != nil {
			// Save previous section if any
			if currentMarker != "" {
				sections[currentMarker] = currentContent
			}
			// Start new section
			currentMarker = strings.ToUpper(match[1])
			// Get content after marker
			content := strings.TrimSpace(trimmed[len(match[0]):])
			currentContent = []string{}
			if content != "" {
				currentContent = append(currentContent, content)
			}
		} else if currentMarker != "" {
			// Continue current section
			currentContent = append(currentContent, trimmed)
		} else {
			// Try to parse single-line format
			return parseSingleLine(input)
		}
	}

	// Save last section
	if currentMarker != "" {
		sections[currentMarker] = currentContent
	}

	// If no sections found, try single-line parse
	if len(sections) == 0 {
		return parseSingleLine(input)
	}

	// Extract and validate sections
	cao := &CAOTriple{}

	condContent, hasWhen := sections["WHEN"]
	if !hasWhen {
		return nil, ErrMissingCondition
	}
	cao.Condition = joinAndTrim(condContent)

	actionContent, hasDo := sections["DO"]
	if !hasDo {
		return nil, ErrMissingAction
	}
	cao.Action = joinAndTrim(actionContent)

	resultContent, hasResult := sections["RESULT"]
	if !hasResult {
		return nil, ErrMissingOutcome
	}
	cao.Outcome = joinAndTrim(resultContent)

	if err := cao.Validate(); err != nil {
		return nil, err
	}

	return cao, nil
}

// parseSingleLine attempts to parse a CAO triple from a single line.
func parseSingleLine(input string) (*CAOTriple, error) {
	// Case-insensitive pattern for inline format
	pattern := regexp.MustCompile(`(?i)WHEN\s+(.+?)\s+DO\s+(.+?)\s+RESULT\s+(.+)`)
	match := pattern.FindStringSubmatch(input)
	if match == nil {
		// Check what's missing
		if !regexp.MustCompile(`(?i)WHEN\s+`).MatchString(input) {
			return nil, ErrMissingCondition
		}
		if !regexp.MustCompile(`(?i)DO\s+`).MatchString(input) {
			return nil, ErrMissingAction
		}
		return nil, ErrMissingOutcome
	}

	cao := &CAOTriple{
		Condition: strings.TrimSpace(match[1]),
		Action:    strings.TrimSpace(match[2]),
		Outcome:   strings.TrimSpace(match[3]),
	}

	if err := cao.Validate(); err != nil {
		return nil, err
	}

	return cao, nil
}

// joinAndTrim joins content lines and trims whitespace.
func joinAndTrim(lines []string) string {
	// Remove trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	// Remove leading empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// FormatCAO formats a CAOTriple as a multi-line string.
func FormatCAO(cao *CAOTriple) string {
	if cao == nil {
		return ""
	}
	return cao.String()
}

// String returns the CAOTriple as a formatted multi-line string.
func (c *CAOTriple) String() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("WHEN %s\nDO %s\nRESULT %s", c.Condition, c.Action, c.Outcome)
}

// Validate checks that all three fields are non-empty.
func (c *CAOTriple) Validate() error {
	if c == nil {
		return ErrMissingCondition
	}
	if strings.TrimSpace(c.Condition) == "" {
		return ErrEmptyCondition
	}
	if strings.TrimSpace(c.Action) == "" {
		return ErrEmptyAction
	}
	if strings.TrimSpace(c.Outcome) == "" {
		return ErrEmptyOutcome
	}
	return nil
}
