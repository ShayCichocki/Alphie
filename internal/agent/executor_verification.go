package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShayCichocki/alphie/internal/verification"
)

// verificationContext holds verification state during task execution.
type verificationContext struct {
	draftContract   *verification.VerificationContract
	finalContract   *verification.VerificationContract
	contractStorage *verification.ContractStorage
	output          strings.Builder
}

// generateDraftContract creates a verification contract before implementation.
// This establishes minimum verification requirements that cannot be weakened.
func (e *Executor) generateDraftContract(
	ctx context.Context,
	taskID string,
	verificationIntent string,
	fileBoundaries []string,
	workDir string,
) *verificationContext {
	vc := &verificationContext{}

	if verificationIntent == "" {
		return vc
	}

	vc.contractStorage = verification.NewContractStorage(e.worktreeMgr.RepoPath())
	promptRunner := NewClaudePromptRunnerWithFactory(e.runnerFactory)
	verifyGen := verification.NewGenerator(workDir, promptRunner)
	projectCtx := verification.GetProjectContext(workDir)

	var draftErr error
	vc.draftContract, draftErr = verifyGen.DraftContract(ctx, verificationIntent, fileBoundaries, projectCtx)
	if draftErr == nil && vc.draftContract != nil {
		// Store draft contract before implementation
		if saveErr := vc.contractStorage.SaveDraft(taskID, vc.draftContract); saveErr != nil {
			// Log but continue - verification can still work in-memory
			vc.output.WriteString(fmt.Sprintf("[Contract storage warning: %v]\n", saveErr))
		}
	}
	// If draft generation fails, continue without it - we'll fallback to post-impl only

	return vc
}

// refineVerificationContract refines the draft contract post-implementation.
// The refinement can only ADD checks, never remove them.
func (e *Executor) refineVerificationContract(
	ctx context.Context,
	vc *verificationContext,
	taskID string,
	verificationIntent string,
	modifiedFiles []string,
	workDir string,
) *verification.VerificationContract {
	if verificationIntent == "" {
		return nil
	}

	promptRunner := NewClaudePromptRunnerWithFactory(e.runnerFactory)
	verifyGen := verification.NewGenerator(workDir, promptRunner)
	projectCtx := verification.GetProjectContext(workDir)

	var finalContract *verification.VerificationContract

	if vc.draftContract != nil {
		// Refine the draft - can only ADD checks, never remove
		refined, refineErr := verifyGen.RefineContract(ctx, vc.draftContract, modifiedFiles, projectCtx)
		if refineErr != nil {
			// Refinement failed (tried to weaken) - use draft as-is
			vc.output.WriteString(fmt.Sprintf("\n[Contract refinement rejected: %v - using draft]\n", refineErr))
			finalContract = vc.draftContract
		} else {
			finalContract = refined
		}

		// Store final contract with audit trail
		if vc.contractStorage != nil && finalContract != nil {
			storedDraft, loadErr := vc.contractStorage.LoadDraft(taskID)
			if loadErr == nil {
				if saveErr := vc.contractStorage.SaveFinal(taskID, finalContract, storedDraft); saveErr != nil {
					vc.output.WriteString(fmt.Sprintf("[Contract save warning: %v]\n", saveErr))
				}
			}
		}
	} else {
		// No draft - fallback to post-impl generation (legacy behavior)
		var genErr error
		finalContract, genErr = verifyGen.Generate(ctx, verificationIntent, modifiedFiles, projectCtx)
		if genErr != nil {
			vc.output.WriteString(fmt.Sprintf("[Verification generation warning: %v]\n", genErr))
		}
	}

	vc.finalContract = finalContract
	return finalContract
}
