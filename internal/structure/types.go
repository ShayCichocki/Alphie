// Package structure provides directory structure analysis and guidance.
package structure

// StructureRule represents a detected directory pattern in the repository.
type StructureRule struct {
	// Pattern is the glob pattern for this directory structure (e.g., "backend/internal/store/*.go")
	Pattern string
	// Description is a human-readable description of what this pattern represents
	Description string
	// Examples are concrete file paths that match this pattern
	Examples []string
	// Directory is the directory this rule applies to
	Directory string
}

// GetPattern returns the pattern for this rule.
func (sr *StructureRule) GetPattern() string {
	return sr.Pattern
}

// GetDescription returns the description for this rule.
func (sr *StructureRule) GetDescription() string {
	return sr.Description
}

// GetExamples returns the examples for this rule.
func (sr *StructureRule) GetExamples() []string {
	return sr.Examples
}

// StructureRules is a collection of structure rules for a repository.
type StructureRules struct {
	// Rules is the list of detected structure rules
	Rules []StructureRule
	// Timestamp is when the rules were last analyzed
	Timestamp int64
}

// RuleGetter is an interface for getting rule information.
// This interface is used to avoid circular dependencies with the agent package.
type RuleGetter interface {
	GetPattern() string
	GetDescription() string
	GetExamples() []string
}

// GetRulesForPath returns structure rules relevant to the given path or file boundaries.
// It matches rules where the given path/boundaries intersect with the rule's directory.
// Returns rules as an interface slice to avoid circular dependencies.
func (sr *StructureRules) GetRulesForPath(boundaries []string) []RuleGetter {
	var rawRules []StructureRule
	if len(boundaries) == 0 {
		rawRules = sr.Rules
	} else {
		for _, rule := range sr.Rules {
			for _, boundary := range boundaries {
				// Check if the boundary matches or contains this rule's directory
				if matchesOrContains(boundary, rule.Directory) {
					rawRules = append(rawRules, rule)
					break
				}
			}
		}
	}

	// Convert to interface slice
	result := make([]RuleGetter, len(rawRules))
	for i := range rawRules {
		result[i] = &rawRules[i]
	}
	return result
}

// matchesOrContains checks if path1 matches or contains path2.
// Examples:
//   - "backend/" contains "backend/internal/"
//   - "backend/internal/" matches "backend/internal/"
//   - "frontend/" does not contain "backend/"
func matchesOrContains(path1, path2 string) bool {
	// Simple prefix match for now
	if len(path1) >= len(path2) {
		return path1[:len(path2)] == path2
	}
	return path2[:len(path1)] == path1
}
