// Package validation provides an adapter for integration with the agent package.
package validation

import (
	"context"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/internal/verification"
)

// ValidatorInterface defines the interface for validators that can be adapted.
type ValidatorInterface interface {
	Validate(ctx context.Context, input ValidationInput) (*ValidationResult, error)
}

// ValidatorAdapter adapts the Validator to implement agent.TaskValidator interface.
// This breaks the import cycle between agent and validation packages.
type ValidatorAdapter struct {
	validator ValidatorInterface
}

// NewValidatorAdapter creates a new adapter that implements agent.TaskValidator.
func NewValidatorAdapter(validator ValidatorInterface) *ValidatorAdapter {
	return &ValidatorAdapter{
		validator: validator,
	}
}

// Validate implements agent.TaskValidator.Validate.
func (a *ValidatorAdapter) Validate(ctx context.Context, input agent.TaskValidationInput) (*agent.TaskValidationResult, error) {
	// Convert agent.TaskValidationInput to validation.ValidationInput
	var contract *verification.VerificationContract
	if input.VerificationContract != nil {
		if c, ok := input.VerificationContract.(*verification.VerificationContract); ok {
			contract = c
		}
	}

	validationInput := ValidationInput{
		RepoPath:             input.RepoPath,
		TaskTitle:            input.TaskTitle,
		TaskDescription:      input.TaskDescription,
		VerificationContract: contract,
		Implementation:       input.Implementation,
		ModifiedFiles:        input.ModifiedFiles,
		AcceptanceCriteria:   input.AcceptanceCriteria,
	}

	// Run validation
	result, err := a.validator.Validate(ctx, validationInput)
	if err != nil {
		return nil, err
	}

	// Convert validation.ValidationResult to agent.TaskValidationResult
	return &agent.TaskValidationResult{
		AllPassed:     result.AllPassed,
		Summary:       result.Summary,
		FailureReason: result.FailureReason,
	}, nil
}
