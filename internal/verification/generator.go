// Package verification provides verification contract generation and execution.
package verification

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// draftContractPrompt is the prompt template for generating PRE-implementation verification contracts.
// This is generated BEFORE the agent implements, based only on intent and expected changes.
const draftContractPrompt = `Generate a verification contract for a task BEFORE implementation.

## Task Intent
%s

## Expected File Changes
%s

## Project Context
%s

Based on the task intent, generate verification commands that will prove the task was completed correctly.
This contract is generated BEFORE implementation - focus on what SHOULD happen, not what DID happen.

Return ONLY a JSON object with this exact structure (no other text):
{
  "commands": [
    {
      "cmd": "command to run",
      "expect": "exit 0",
      "description": "What this verifies",
      "required": true
    }
  ],
  "file_constraints": {
    "must_exist": ["expected-file1.go", "expected-file2.go"],
    "must_not_exist": [],
    "must_not_change": []
  }
}

Guidelines for PRE-implementation contracts:
- Focus on verifying INTENT, not implementation details
- Include tests that verify the BEHAVIOR described in the intent
- Use general patterns: "go test ./..." rather than specific test names (those don't exist yet)
- For "add X", include must_exist for the expected file
- For "modify X", include targeted tests for that area
- Mark critical verifications as required=true
- Be conservative: it's better to have fewer, stronger checks than many weak ones

Examples:
- For "Add user authentication": {"cmd": "go test ./internal/auth/...", "expect": "exit 0", "description": "Auth tests pass", "required": true}
- For "Create config file": {"cmd": "test -f config.yaml", "expect": "exit 0", "description": "Config file exists", "required": true}
- For "Add password hashing": {"cmd": "grep -r 'bcrypt\\|argon2\\|scrypt' .", "expect": "exit 0", "description": "Uses secure hashing", "required": true}
`

// refineContractPrompt is the prompt template for refining contracts AFTER implementation.
const refineContractPrompt = `Refine a verification contract after implementation.

## Task Intent
%s

## Original Draft Contract (MUST NOT WEAKEN)
%s

## Files Actually Modified
%s

## Project Context
%s

The task has been implemented. Refine the verification contract with more specific checks.

CRITICAL RULES:
1. You CANNOT remove any required commands from the draft
2. You CANNOT remove any file constraints from the draft
3. You CAN add new commands and constraints
4. You CAN make expectations more specific (e.g., "exit 0" -> "output contains success")
5. You CANNOT downgrade required=true to required=false

Return ONLY a JSON object with the refined contract (same structure as draft):
{
  "commands": [...],
  "file_constraints": {...}
}

The refined contract must be a SUPERSET of the draft - only additions allowed.
`

// verificationPrompt is the prompt template for generating verification commands.
// DEPRECATED: Use draftContractPrompt for pre-impl and refineContractPrompt for post-impl.
const verificationPrompt = `Generate concrete verification commands for a completed task.

## Task Intent
%s

## Files Created/Modified
%s

## Project Context
%s

Based on the task intent and files modified, generate specific commands that can verify the task was completed correctly.

Return ONLY a JSON object with this exact structure (no other text):
{
  "commands": [
    {
      "cmd": "command to run",
      "expect": "exit 0",
      "description": "What this verifies",
      "required": true
    }
  ],
  "file_constraints": {
    "must_exist": ["file1.ts", "file2.ts"],
    "must_not_exist": [],
    "must_not_change": []
  }
}

Guidelines:
- Generate 1-5 verification commands based on task complexity
- Use "exit 0" for commands that should succeed
- Use "output contains X" for commands where we need to check output
- Prefer existing test commands (npm test, go test, pytest) when tests were modified
- For API endpoints, use curl commands if appropriate
- For file operations, check that expected files exist
- Mark required=false for "nice to have" verifications that shouldn't fail the task
- Only include must_not_change for files that were explicitly mentioned as off-limits

Examples:
- For "Add login endpoint": {"cmd": "npm test -- --grep login", "expect": "exit 0", "description": "Login tests pass", "required": true}
- For "Create auth module": {"cmd": "test -f src/auth/index.ts", "expect": "exit 0", "description": "Auth module entry point exists", "required": true}
- For "Add README": {"cmd": "test -f README.md", "expect": "exit 0", "description": "README exists", "required": true}
`

// verificationResponse is the JSON structure returned by Claude.
type verificationResponse struct {
	Commands []struct {
		Cmd         string `json:"cmd"`
		Expect      string `json:"expect"`
		Description string `json:"description"`
		Required    bool   `json:"required"`
	} `json:"commands"`
	FileConstraints struct {
		MustExist     []string `json:"must_exist"`
		MustNotExist  []string `json:"must_not_exist"`
		MustNotChange []string `json:"must_not_change"`
	} `json:"file_constraints"`
}

// PromptRunner is an interface for running a prompt and getting a response.
// This abstracts away the Claude process dependency.
type PromptRunner interface {
	// RunPrompt sends a prompt and returns the response.
	RunPrompt(ctx context.Context, prompt string, workDir string) (string, error)
}

// Generator generates verification contracts for completed tasks.
type Generator struct {
	workDir      string
	promptRunner PromptRunner
}

// NewGenerator creates a new generator for the given work directory.
func NewGenerator(workDir string, runner PromptRunner) *Generator {
	return &Generator{
		workDir:      workDir,
		promptRunner: runner,
	}
}

// Generate creates a VerificationContract based on task intent and modified files.
// It uses the prompt runner (typically Claude) to generate concrete verification commands.
func (g *Generator) Generate(
	ctx context.Context,
	intent string,
	modifiedFiles []string,
	projectContext string,
) (*VerificationContract, error) {
	if g.promptRunner == nil {
		// No runner provided, return minimal contract
		return g.GenerateMinimal(intent, modifiedFiles), nil
	}

	// Build the prompt
	filesStr := strings.Join(modifiedFiles, "\n")
	if filesStr == "" {
		filesStr = "(no files tracked)"
	}
	if projectContext == "" {
		projectContext = "(unknown project type)"
	}

	prompt := fmt.Sprintf(verificationPrompt, intent, filesStr, projectContext)

	// Run the prompt
	response, err := g.promptRunner.RunPrompt(ctx, prompt, g.workDir)
	if err != nil {
		return nil, fmt.Errorf("run verification prompt: %w", err)
	}

	// Parse the response
	contract, err := g.parseResponse(response, intent)
	if err != nil {
		return nil, fmt.Errorf("parse verification response: %w", err)
	}

	return contract, nil
}

// parseResponse parses Claude's JSON response into a VerificationContract.
func (g *Generator) parseResponse(response string, intent string) (*VerificationContract, error) {
	// Find the JSON object in the response (Claude might include extra text)
	jsonStart := strings.Index(response, "{")
	jsonEnd := strings.LastIndex(response, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		// If no JSON found, return a minimal contract with just the intent
		return &VerificationContract{
			Intent: intent,
		}, nil
	}
	jsonStr := response[jsonStart : jsonEnd+1]

	var vr verificationResponse
	if err := json.Unmarshal([]byte(jsonStr), &vr); err != nil {
		// If JSON parsing fails, return a minimal contract
		return &VerificationContract{
			Intent: intent,
		}, nil
	}

	// Build the contract
	contract := &VerificationContract{
		Intent:   intent,
		Commands: make([]VerificationCommand, 0, len(vr.Commands)),
		FileConstraints: FileConstraints{
			MustExist:     vr.FileConstraints.MustExist,
			MustNotExist:  vr.FileConstraints.MustNotExist,
			MustNotChange: vr.FileConstraints.MustNotChange,
		},
	}

	for _, cmd := range vr.Commands {
		contract.Commands = append(contract.Commands, VerificationCommand{
			Command:     cmd.Cmd,
			Expect:      cmd.Expect,
			Description: cmd.Description,
			Required:    cmd.Required,
		})
	}

	return contract, nil
}

// GenerateMinimal creates a minimal verification contract without using the prompt runner.
// This is used when quick verification is needed or the runner is unavailable.
func (g *Generator) GenerateMinimal(intent string, modifiedFiles []string) *VerificationContract {
	contract := &VerificationContract{
		Intent: intent,
	}

	// Add file existence checks for modified files
	for _, file := range modifiedFiles {
		if file != "" {
			contract.FileConstraints.MustExist = append(contract.FileConstraints.MustExist, file)
		}
	}

	return contract
}

// DraftContract generates a verification contract BEFORE implementation.
// This is based only on the task intent and expected file changes, not actual implementation.
// The draft establishes minimum verification requirements that cannot be weakened later.
func (g *Generator) DraftContract(
	ctx context.Context,
	intent string,
	expectedFiles []string,
	projectContext string,
) (*VerificationContract, error) {
	if g.promptRunner == nil {
		// No runner provided, return minimal contract with expected files
		return g.GenerateMinimal(intent, expectedFiles), nil
	}

	// Build the prompt
	filesStr := strings.Join(expectedFiles, "\n")
	if filesStr == "" {
		filesStr = "(no specific files expected)"
	}
	if projectContext == "" {
		projectContext = GetProjectContext(g.workDir)
	}

	prompt := fmt.Sprintf(draftContractPrompt, intent, filesStr, projectContext)

	// Run the prompt
	response, err := g.promptRunner.RunPrompt(ctx, prompt, g.workDir)
	if err != nil {
		// On error, return minimal contract rather than failing
		return g.GenerateMinimal(intent, expectedFiles), nil
	}

	// Parse the response
	contract, err := g.parseResponse(response, intent)
	if err != nil {
		return g.GenerateMinimal(intent, expectedFiles), nil
	}

	// Enhance contract with patterns
	patterns := DetectPatterns(intent, expectedFiles)
	ApplyPatterns(contract, patterns)

	// Enhance contract with project-specific commands
	projCtx := DetectProjectContext(g.workDir)
	EnhanceContractWithProjectContext(contract, projCtx)

	return contract, nil
}

// RefineContract generates a refined contract AFTER implementation.
// The refined contract must be a superset of the draft - it can only add checks, never remove them.
// Returns an error if the refinement would weaken the contract.
func (g *Generator) RefineContract(
	ctx context.Context,
	draft *VerificationContract,
	modifiedFiles []string,
	projectContext string,
) (*VerificationContract, error) {
	if g.promptRunner == nil {
		// No runner - just add modified files to draft's must_exist
		refined := &VerificationContract{
			Intent:   draft.Intent,
			Commands: append([]VerificationCommand{}, draft.Commands...),
			FileConstraints: FileConstraints{
				MustExist:     append([]string{}, draft.FileConstraints.MustExist...),
				MustNotExist:  append([]string{}, draft.FileConstraints.MustNotExist...),
				MustNotChange: append([]string{}, draft.FileConstraints.MustNotChange...),
			},
		}
		// Add any new files that were modified
		existingFiles := make(map[string]bool)
		for _, f := range refined.FileConstraints.MustExist {
			existingFiles[f] = true
		}
		for _, f := range modifiedFiles {
			if f != "" && !existingFiles[f] {
				refined.FileConstraints.MustExist = append(refined.FileConstraints.MustExist, f)
			}
		}
		return refined, nil
	}

	// Serialize draft for the prompt
	draftJSON, err := json.MarshalIndent(draft, "", "  ")
	if err != nil {
		draftJSON = []byte("{}")
	}

	filesStr := strings.Join(modifiedFiles, "\n")
	if filesStr == "" {
		filesStr = "(no files modified)"
	}
	if projectContext == "" {
		projectContext = GetProjectContext(g.workDir)
	}

	prompt := fmt.Sprintf(refineContractPrompt, draft.Intent, string(draftJSON), filesStr, projectContext)

	// Run the prompt
	response, err := g.promptRunner.RunPrompt(ctx, prompt, g.workDir)
	if err != nil {
		// On error, return the draft unchanged (safe fallback)
		return draft, nil
	}

	// Parse the response
	refined, err := g.parseResponse(response, draft.Intent)
	if err != nil {
		return draft, nil
	}

	// Enhance refined contract with patterns (based on actual modified files)
	patterns := DetectPatterns(draft.Intent, modifiedFiles)
	ApplyPatterns(refined, patterns)

	// Enhance with project-specific commands if not already present
	projCtx := DetectProjectContext(g.workDir)
	EnhanceContractWithProjectContext(refined, projCtx)

	// Validate that refinement only strengthens
	storage := NewContractStorage(g.workDir)
	if err := storage.ValidateRefinement(draft, refined); err != nil {
		// Refinement tried to weaken - reject and return draft
		return nil, fmt.Errorf("refinement violated monotonic strengthening: %w", err)
	}

	return refined, nil
}

// ProjectType represents the detected project type.
type ProjectType int

const (
	ProjectTypeUnknown ProjectType = iota
	ProjectTypeGo
	ProjectTypeNode
	ProjectTypeRust
	ProjectTypePython
)

// DetectProjectType detects the project type from the work directory.
func DetectProjectType(workDir string) ProjectType {
	// Check for Go
	if fileExists(workDir, "go.mod") {
		return ProjectTypeGo
	}
	// Check for Node.js
	if fileExists(workDir, "package.json") {
		return ProjectTypeNode
	}
	// Check for Rust
	if fileExists(workDir, "Cargo.toml") {
		return ProjectTypeRust
	}
	// Check for Python
	if fileExists(workDir, "pyproject.toml") || fileExists(workDir, "setup.py") || fileExists(workDir, "requirements.txt") {
		return ProjectTypePython
	}
	return ProjectTypeUnknown
}

// GetProjectContext returns a description of the project type
// suitable for use in verification generation prompts.
func GetProjectContext(workDir string) string {
	pt := DetectProjectType(workDir)

	switch pt {
	case ProjectTypeGo:
		return "Go project (go test ./..., go build ./...)"
	case ProjectTypeNode:
		return "Node.js/TypeScript project (npm test, npm run build)"
	case ProjectTypeRust:
		return "Rust project (cargo test, cargo build)"
	case ProjectTypePython:
		return "Python project (pytest, ruff)"
	default:
		return "Unknown project type"
	}
}

// fileExists checks if a file exists in the given directory.
func fileExists(dir, name string) bool {
	path := dir + "/" + name
	_, err := os.Stat(path)
	return err == nil
}
