// Package verification provides verification contract types and persistence.
package verification

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ContractStorage handles persistence of verification contracts.
type ContractStorage struct {
	baseDir string
}

// StoredContract wraps a contract with metadata for persistence.
type StoredContract struct {
	// Contract is the verification contract.
	Contract *VerificationContract `json:"contract"`
	// TaskID is the ID of the task this contract is for.
	TaskID string `json:"task_id"`
	// Phase indicates whether this is a draft or final contract.
	Phase ContractPhase `json:"phase"`
	// CreatedAt is when this contract was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this contract was last modified.
	UpdatedAt time.Time `json:"updated_at"`
	// History tracks refinements for audit trail.
	History []ContractDelta `json:"history,omitempty"`
}

// ContractPhase indicates the lifecycle phase of a contract.
type ContractPhase string

const (
	// PhaseDraft is the pre-implementation contract.
	PhaseDraft ContractPhase = "draft"
	// PhaseFinal is the post-implementation refined contract.
	PhaseFinal ContractPhase = "final"
)

// ContractDelta records a change to the contract for audit purposes.
type ContractDelta struct {
	// Timestamp is when the change occurred.
	Timestamp time.Time `json:"timestamp"`
	// Action describes what changed.
	Action string `json:"action"`
	// Before is the state before the change (optional).
	Before string `json:"before,omitempty"`
	// After is the state after the change.
	After string `json:"after"`
}

// NewContractStorage creates a new storage for the given work directory.
// Contracts are stored in .alphie/contracts/ within the work directory.
func NewContractStorage(workDir string) *ContractStorage {
	return &ContractStorage{
		baseDir: filepath.Join(workDir, ".alphie", "contracts"),
	}
}

// ensureDir creates the contracts directory if it doesn't exist.
func (s *ContractStorage) ensureDir() error {
	return os.MkdirAll(s.baseDir, 0755)
}

// draftPath returns the path for a draft contract.
func (s *ContractStorage) draftPath(taskID string) string {
	return filepath.Join(s.baseDir, fmt.Sprintf("%s-draft.json", taskID))
}

// finalPath returns the path for a final contract.
func (s *ContractStorage) finalPath(taskID string) string {
	return filepath.Join(s.baseDir, fmt.Sprintf("%s.json", taskID))
}

// SaveDraft saves a draft (pre-implementation) contract.
func (s *ContractStorage) SaveDraft(taskID string, contract *VerificationContract) error {
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("create contracts directory: %w", err)
	}

	stored := &StoredContract{
		Contract:  contract,
		TaskID:    taskID,
		Phase:     PhaseDraft,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		History: []ContractDelta{
			{
				Timestamp: time.Now(),
				Action:    "draft_created",
				After:     fmt.Sprintf("%d commands, %d file constraints", len(contract.Commands), countFileConstraints(contract)),
			},
		},
	}

	return s.writeContract(s.draftPath(taskID), stored)
}

// SaveFinal saves a final (post-implementation) contract.
// This should only be called after ValidateRefinement passes.
func (s *ContractStorage) SaveFinal(taskID string, contract *VerificationContract, draft *StoredContract) error {
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("create contracts directory: %w", err)
	}

	// Build history from draft
	history := draft.History
	history = append(history, ContractDelta{
		Timestamp: time.Now(),
		Action:    "refined_to_final",
		Before:    fmt.Sprintf("%d commands, %d file constraints", len(draft.Contract.Commands), countFileConstraints(draft.Contract)),
		After:     fmt.Sprintf("%d commands, %d file constraints", len(contract.Commands), countFileConstraints(contract)),
	})

	stored := &StoredContract{
		Contract:  contract,
		TaskID:    taskID,
		Phase:     PhaseFinal,
		CreatedAt: draft.CreatedAt,
		UpdatedAt: time.Now(),
		History:   history,
	}

	return s.writeContract(s.finalPath(taskID), stored)
}

// LoadDraft loads a draft contract for a task.
func (s *ContractStorage) LoadDraft(taskID string) (*StoredContract, error) {
	return s.readContract(s.draftPath(taskID))
}

// LoadFinal loads a final contract for a task.
func (s *ContractStorage) LoadFinal(taskID string) (*StoredContract, error) {
	return s.readContract(s.finalPath(taskID))
}

// DraftExists checks if a draft contract exists for the task.
func (s *ContractStorage) DraftExists(taskID string) bool {
	_, err := os.Stat(s.draftPath(taskID))
	return err == nil
}

// FinalExists checks if a final contract exists for the task.
func (s *ContractStorage) FinalExists(taskID string) bool {
	_, err := os.Stat(s.finalPath(taskID))
	return err == nil
}

// ValidateRefinement checks that the refined contract only strengthens (never weakens) the draft.
// Returns nil if valid, error describing the violation otherwise.
func (s *ContractStorage) ValidateRefinement(draft, refined *VerificationContract) error {
	// Rule 1: Cannot remove required commands
	draftRequiredCmds := make(map[string]bool)
	for _, cmd := range draft.Commands {
		if cmd.Required {
			draftRequiredCmds[cmd.Command] = true
		}
	}

	refinedCmds := make(map[string]bool)
	for _, cmd := range refined.Commands {
		refinedCmds[cmd.Command] = true
	}

	for cmd := range draftRequiredCmds {
		if !refinedCmds[cmd] {
			return fmt.Errorf("refinement removed required command: %s", cmd)
		}
	}

	// Rule 2: Cannot remove must_exist constraints
	draftMustExist := make(map[string]bool)
	for _, f := range draft.FileConstraints.MustExist {
		draftMustExist[f] = true
	}

	refinedMustExist := make(map[string]bool)
	for _, f := range refined.FileConstraints.MustExist {
		refinedMustExist[f] = true
	}

	for f := range draftMustExist {
		if !refinedMustExist[f] {
			return fmt.Errorf("refinement removed must_exist constraint: %s", f)
		}
	}

	// Rule 3: Cannot remove must_not_exist constraints
	draftMustNotExist := make(map[string]bool)
	for _, f := range draft.FileConstraints.MustNotExist {
		draftMustNotExist[f] = true
	}

	refinedMustNotExist := make(map[string]bool)
	for _, f := range refined.FileConstraints.MustNotExist {
		refinedMustNotExist[f] = true
	}

	for f := range draftMustNotExist {
		if !refinedMustNotExist[f] {
			return fmt.Errorf("refinement removed must_not_exist constraint: %s", f)
		}
	}

	// Rule 4: Cannot remove must_not_change constraints
	draftMustNotChange := make(map[string]bool)
	for _, f := range draft.FileConstraints.MustNotChange {
		draftMustNotChange[f] = true
	}

	refinedMustNotChange := make(map[string]bool)
	for _, f := range refined.FileConstraints.MustNotChange {
		refinedMustNotChange[f] = true
	}

	for f := range draftMustNotChange {
		if !refinedMustNotChange[f] {
			return fmt.Errorf("refinement removed must_not_change constraint: %s", f)
		}
	}

	// Rule 5: Cannot make a required command non-required
	for _, draftCmd := range draft.Commands {
		if !draftCmd.Required {
			continue
		}
		for _, refinedCmd := range refined.Commands {
			if refinedCmd.Command == draftCmd.Command && !refinedCmd.Required {
				return fmt.Errorf("refinement downgraded required command to non-required: %s", draftCmd.Command)
			}
		}
	}

	return nil
}

// writeContract writes a stored contract to disk.
func (s *ContractStorage) writeContract(path string, stored *StoredContract) error {
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal contract: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write contract file: %w", err)
	}

	return nil
}

// readContract reads a stored contract from disk.
func (s *ContractStorage) readContract(path string) (*StoredContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contract file: %w", err)
	}

	var stored StoredContract
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal contract: %w", err)
	}

	return &stored, nil
}

// countFileConstraints returns the total number of file constraints.
func countFileConstraints(c *VerificationContract) int {
	return len(c.FileConstraints.MustExist) +
		len(c.FileConstraints.MustNotExist) +
		len(c.FileConstraints.MustNotChange)
}
