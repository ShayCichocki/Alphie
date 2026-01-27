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

	// If no modified files provided, skip stub detection (can't check whole repo)
	if len(input.ModifiedFiles) == 0 {
		result.Output = "No modified files to check - skipping stub detection"
		return result
	}

	// Only search in modified files
	for _, pattern := range stubPatterns {
		for _, modFile := range input.ModifiedFiles {
			// Run grep on each modified file individually
			filePath := fmt.Sprintf("%s/%s", input.RepoPath, modFile)
			cmd := exec.CommandContext(ctx, "grep", "-n", pattern, filePath)
			output, err := cmd.CombinedOutput()

			if err == nil && len(output) > 0 {
				// Found matches in this file
				lines := strings.Split(string(output), "\n")
				for _, line := range lines {
					if strings.TrimSpace(line) != "" {
						foundStubs = append(foundStubs, fmt.Sprintf("  %s in %s: %s", pattern, modFile, line))
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
