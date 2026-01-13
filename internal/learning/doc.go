// Package learning provides learning and context management capabilities.
//
// # CAO Triples
//
// The CAO (Condition-Action-Outcome) triple format captures learnings in a
// structured way. Each triple consists of:
//
//   - WHEN: The context or trigger condition
//   - DO: The action to take
//   - RESULT: The expected outcome
//
// Example:
//
//	WHEN build fails with "assets not embedded" error
//	DO use `go build` instead of `go run`
//	RESULT build succeeds with embedded assets
//
// Both single-line and multi-line formats are supported:
//
//	// Single-line
//	WHEN X DO Y RESULT Z
//
//	// Multi-line
//	WHEN X
//	DO Y
//	RESULT Z
package learning
