package validation

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// runLayer0StubDetection performs a fast pre-check for stub implementations.
// This runs before all other validation layers to fail fast if the code is incomplete.
func (v *Validator) runLayer0StubDetection(ctx context.Context, input ValidationInput) *LayerResult {
	result := &LayerResult{
		Name:   "Stub Detection",
		Passed: true,
		Output: "",
	}

	stubPatterns := []string{
		"Not implemented",
		"TODO: Implement",
		"TODO: implement",
		"http.StatusNotImplemented",
	}

	var foundStubs []string

	for _, pattern := range stubPatterns {
		// Run grep to search for stub patterns in modified files
		cmd := exec.CommandContext(ctx, "grep", "-r", "-n", pattern, input.RepoPath)
		output, err := cmd.CombinedOutput()
		
		if err == nil && len(output) > 0 {
			// Found matches
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.TrimSpace(line) != "" {
					// Filter to only modified files if possible
					shouldInclude := len(input.ModifiedFiles) == 0
					for _, modFile := range input.ModifiedFiles {
						if strings.Contains(line, modFile) {
							shouldInclude = true
							break
						}
					}
					if shouldInclude {
						foundStubs = append(foundStubs, fmt.Sprintf("  %s: %s", pattern, line))
					}
				}
			}
		}
	}

	if len(foundStubs) > 0 {
		result.Passed = false
		result.Output = fmt.Sprintf("Stub implementations detected:\n%s\n\nYou must implement complete, working functionality. Stubs like 'Not implemented' or 'TODO' are not acceptable.", 
			strings.Join(foundStubs, "\n"))
	} else {
		result.Output = "No stub implementations detected - code appears complete"
	}

	return result
}
