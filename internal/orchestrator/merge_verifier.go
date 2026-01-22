// Package orchestrator provides post-merge verification.
package orchestrator

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// VerificationResult contains the result of a post-merge verification.
type VerificationResult struct {
	// Passed indicates if the verification succeeded.
	Passed bool
	// Output contains the command output (stdout + stderr).
	Output string
	// Error contains the error if verification failed.
	Error error
	// Duration is how long the verification took.
	Duration time.Duration
}

// MergeVerifier runs build/test commands after merge to ensure code integrity.
type MergeVerifier struct {
	repoPath    string
	projectInfo *ProjectTypeInfo
	timeout     time.Duration
}

// NewMergeVerifier creates a new MergeVerifier for the given repository.
func NewMergeVerifier(repoPath string, projectInfo *ProjectTypeInfo, timeout time.Duration) *MergeVerifier {
	return &MergeVerifier{
		repoPath:    repoPath,
		projectInfo: projectInfo,
		timeout:     timeout,
	}
}

// VerifyMerge runs build verification after a merge completes.
// It runs the project's build command (if available) to ensure the merged code compiles.
// Returns a VerificationResult indicating success or failure.
func (v *MergeVerifier) VerifyMerge(ctx context.Context, branchName string) (*VerificationResult, error) {
	startTime := time.Now()

	// Check if we have a build command for this project type
	if len(v.projectInfo.BuildCommand) == 0 {
		// No build command available - skip verification but log it
		debugLog("[verifier] no build command available for project type %s, skipping verification", v.projectInfo.Type)
		return &VerificationResult{
			Passed:   true, // Pass by default if we can't verify
			Output:   "No build command available for project type",
			Duration: time.Since(startTime),
		}, nil
	}

	// Create context with timeout
	verifyCtx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	// Build the command
	cmdName := v.projectInfo.BuildCommand[0]
	cmdArgs := v.projectInfo.BuildCommand[1:]

	debugLog("[verifier] running build verification: %s %v", cmdName, cmdArgs)

	cmd := exec.CommandContext(verifyCtx, cmdName, cmdArgs...)
	cmd.Dir = v.repoPath

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	duration := time.Since(startTime)

	if err != nil {
		// Build failed
		debugLog("[verifier] build verification FAILED after %v: %v", duration, err)
		debugLog("[verifier] output: %s", outputStr)

		return &VerificationResult{
			Passed:   false,
			Output:   outputStr,
			Error:    fmt.Errorf("build command failed: %w", err),
			Duration: duration,
		}, nil
	}

	// Build succeeded
	debugLog("[verifier] build verification PASSED after %v", duration)

	return &VerificationResult{
		Passed:   true,
		Output:   outputStr,
		Duration: duration,
	}, nil
}

// ShouldVerify returns true if verification should be run for this project.
// Verification is skipped for unknown project types.
func (v *MergeVerifier) ShouldVerify() bool {
	return v.projectInfo.Type != ProjectTypeUnknown && len(v.projectInfo.BuildCommand) > 0
}

// GetBuildCommandString returns a human-readable string of the build command.
func (v *MergeVerifier) GetBuildCommandString() string {
	if len(v.projectInfo.BuildCommand) == 0 {
		return "none"
	}
	return strings.Join(v.projectInfo.BuildCommand, " ")
}
