// Package orchestrator manages the coordination of agents and workflows.
package orchestrator

import (
	"fmt"

	"github.com/ShayCichocki/alphie/internal/merge"
)

// FallbackStrategy handles merge fallback logic when semantic merge fails.
// It provides various fallback strategies based on the types of conflicting files.
type FallbackStrategy struct {
	merger        *merge.Handler
	repoPath      string
	sessionBranch string
	verifier      *MergeVerifier
}

// NewFallbackStrategy creates a new FallbackStrategy.
func NewFallbackStrategy(merger *merge.Handler, repoPath, sessionBranch string) *FallbackStrategy {
	return &FallbackStrategy{
		merger:        merger,
		repoPath:      repoPath,
		sessionBranch: sessionBranch,
		verifier:      nil, // Set via SetVerifier
	}
}

// SetVerifier sets the merge verifier for post-merge validation.
func (f *FallbackStrategy) SetVerifier(verifier *MergeVerifier) {
	f.verifier = verifier
}

// Attempt tries fallback merge strategies for the given conflicts.
// Returns a merge outcome indicating success or failure.
//
// Strategy:
// 1. Smart merge critical files (package.json, go.mod, etc.) - these can be structurally merged
// 2. Handle remaining non-critical conflicts: accept ours for non-code, fail for code
func (f *FallbackStrategy) Attempt(req *MergeRequest, conflicts []string) MergeOutcome {
	debugLog("[fallback] attempting fallback merge for task %s with %d conflicts", req.TaskID, len(conflicts))

	// Step 1: Separate critical from non-critical conflicts
	var critical, remaining []string
	for _, file := range conflicts {
		if merge.IsCriticalFile(file) {
			critical = append(critical, file)
		} else {
			remaining = append(remaining, file)
		}
	}

	debugLog("[fallback] conflicts: %d critical, %d other", len(critical), len(remaining))

	// Step 2: Smart merge critical files (if any)
	if len(critical) > 0 {
		debugLog("[fallback] attempting smart merge for critical files: %v", critical)

		smartResult, err := merge.SmartMerge(f.repoPath, critical, f.sessionBranch, req.AgentBranch)
		if err != nil {
			debugLog("[fallback] smart merge error: %v", err)
			// Smart merge failed entirely, add critical files back to remaining
			remaining = append(remaining, critical...)
		} else if !smartResult.Success {
			debugLog("[fallback] smart merge had conflicts: %v", smartResult.Conflicts)
			// Some critical files couldn't be merged, add them to remaining
			remaining = append(remaining, smartResult.Conflicts...)
			// But apply the ones that succeeded
			if len(smartResult.MergedFiles) > 0 {
				if err := merge.ApplySmartMerge(f.repoPath, smartResult); err != nil {
					debugLog("[fallback] failed to apply partial smart merge: %v", err)
				} else {
					for file := range smartResult.MergedFiles {
						_ = f.merger.StageFiles(file)
					}
					debugLog("[fallback] applied smart merge for %d files", len(smartResult.MergedFiles))
				}
			}
		} else {
			// Smart merge succeeded for all critical files
			if err := merge.ApplySmartMerge(f.repoPath, smartResult); err != nil {
				debugLog("[fallback] failed to apply smart merge: %v", err)
				remaining = append(remaining, critical...)
			} else {
				for file := range smartResult.MergedFiles {
					_ = f.merger.StageFiles(file)
				}
				debugLog("[fallback] smart merge resolved %d critical files", len(smartResult.MergedFiles))
			}
		}
	}

	// Step 3: Handle remaining conflicts
	if len(remaining) == 0 {
		// All conflicts resolved by smart merge
		if err := f.merger.CommitMerge(fmt.Sprintf("Smart merge for task %s", req.TaskID)); err != nil {
			return MergeOutcome{
				Success:      false,
				FallbackUsed: true,
				Error:        fmt.Errorf("commit failed after smart merge: %w", err),
				Reason:       "smart merge succeeded but commit failed",
			}
		}

		// Post-merge verification: ensure the smart-merged code builds
		if f.verifier != nil && f.verifier.ShouldVerify() {
			debugLog("[fallback] running post-merge verification for task %s", req.TaskID)
			verifyResult, err := f.verifier.VerifyMerge(req.Ctx, f.sessionBranch)
			if err != nil || !verifyResult.Passed {
				// Build verification failed - rollback the commit
				errorMsg := "build verification failed after smart merge"
				if verifyResult.Error != nil {
					errorMsg = verifyResult.Error.Error()
				}

				debugLog("[fallback] verification failed for task %s: %v", req.TaskID, errorMsg)

				// Rollback the commit we just made
				if rollbackErr := f.merger.GitRunner().Reset("HEAD~1"); rollbackErr != nil {
					debugLog("[fallback] CRITICAL: verification failed AND rollback failed for task %s", req.TaskID)
				}

				return MergeOutcome{
					Success:      false,
					FallbackUsed: true,
					Error:        fmt.Errorf("post-merge verification failed: %w", verifyResult.Error),
					Reason:       "smart merge committed but build failed",
				}
			}
			debugLog("[fallback] verification passed for task %s", req.TaskID)
		}

		return MergeOutcome{
			Success:      true,
			FallbackUsed: true,
			Reason:       "smart merge resolved all critical file conflicts",
		}
	}

	// Step 4: Handle remaining non-critical conflicts
	return f.handleRemainingConflicts(req, remaining, len(critical) > 0)
}

// handleRemainingConflicts handles non-critical conflicts after smart merge has been attempted.
// If smartMergeApplied is true, some files were already staged and we just need to handle the rest.
func (f *FallbackStrategy) handleRemainingConflicts(req *MergeRequest, conflicts []string, smartMergeApplied bool) MergeOutcome {
	// Separate code and non-code conflicts
	var codeConflicts, nonCodeConflicts []string
	for _, file := range conflicts {
		if merge.IsCodeFile(file) {
			codeConflicts = append(codeConflicts, file)
		} else {
			nonCodeConflicts = append(nonCodeConflicts, file)
		}
	}

	debugLog("[fallback] remaining conflicts: %d code, %d non-code", len(codeConflicts), len(nonCodeConflicts))

	// Code conflicts require semantic merge or human intervention
	if len(codeConflicts) > 0 {
		return MergeOutcome{
			Success:       false,
			FallbackUsed:  true,
			ConflictFiles: codeConflicts,
			Error:         fmt.Errorf("code conflicts require resolution: %v", codeConflicts),
			Reason:        "smart merge resolved config files but code conflicts remain",
		}
	}

	// Non-code conflicts only - accept ours
	if len(nonCodeConflicts) > 0 {
		debugLog("[fallback] accepting 'ours' for non-code conflicts: %v", nonCodeConflicts)
		for _, file := range nonCodeConflicts {
			_ = f.merger.CheckoutOurs(file)
			_ = f.merger.StageFiles(file)
		}
	}

	// Commit everything (smart merged files + ours for non-code)
	commitMsg := fmt.Sprintf("Fallback merge for task %s", req.TaskID)
	if smartMergeApplied {
		commitMsg = fmt.Sprintf("Smart + fallback merge for task %s", req.TaskID)
	}

	if err := f.merger.CommitMerge(commitMsg); err != nil {
		return MergeOutcome{
			Success:      false,
			FallbackUsed: true,
			Error:        fmt.Errorf("fallback commit failed: %w", err),
			Reason:       "fallback commit failed",
		}
	}

	// Post-merge verification: ensure the fallback-merged code builds
	if f.verifier != nil && f.verifier.ShouldVerify() {
		debugLog("[fallback] running post-merge verification for task %s", req.TaskID)
		verifyResult, err := f.verifier.VerifyMerge(req.Ctx, f.sessionBranch)
		if err != nil || !verifyResult.Passed {
			// Build verification failed - rollback the commit
			errorMsg := "build verification failed after fallback merge"
			if verifyResult.Error != nil {
				errorMsg = verifyResult.Error.Error()
			}

			debugLog("[fallback] verification failed for task %s: %v", req.TaskID, errorMsg)

			// Rollback the commit we just made
			if rollbackErr := f.merger.GitRunner().Reset("HEAD~1"); rollbackErr != nil {
				debugLog("[fallback] CRITICAL: verification failed AND rollback failed for task %s", req.TaskID)
			}

			return MergeOutcome{
				Success:      false,
				FallbackUsed: true,
				Error:        fmt.Errorf("post-merge verification failed: %w", verifyResult.Error),
				Reason:       "fallback merge committed but build failed",
			}
		}
		debugLog("[fallback] verification passed for task %s", req.TaskID)
	}

	reason := "fallback accepted session branch version for non-code conflicts"
	if smartMergeApplied {
		reason = "smart merge resolved config files, accepted ours for remaining"
	}

	return MergeOutcome{
		Success:      true,
		FallbackUsed: true,
		Reason:       reason,
	}
}
