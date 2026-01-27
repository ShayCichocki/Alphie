// Package decompose provides task decomposition for user requests.
package decompose

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ShayCichocki/alphie/internal/agent"
	"github.com/ShayCichocki/alphie/pkg/models"
)

// decomposedTask is the JSON structure returned by Claude for a single task.
type decomposedTask struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	TaskType           string   `json:"task_type"`
	FileBoundaries     []string `json:"file_boundaries"`
	DependsOn          []string `json:"depends_on"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
	VerificationIntent string   `json:"verification_intent"`
}

// Decomposer breaks down user requests into parallelizable subtasks.
type Decomposer struct {
	claude agent.ClaudeRunner
}

// New creates a new Decomposer with the given Claude runner.
func New(claude agent.ClaudeRunner) *Decomposer {
	return &Decomposer{claude: claude}
}

// Decompose takes a user request and returns a list of tasks with dependencies.
func (d *Decomposer) Decompose(ctx context.Context, request string) ([]*models.Task, error) {
	prompt := fmt.Sprintf(decompositionPrompt, request)

	if err := d.claude.Start(prompt, ""); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	var response strings.Builder
	var resultCount int
	for event := range d.claude.Output() {
		select {
		case <-ctx.Done():
			_ = d.claude.Kill()
			return nil, ctx.Err()
		default:
		}

		switch event.Type {
		case agent.StreamEventResult:
			// Use final result only (avoids duplication with streaming chunks)
			resultCount++
			// Only use the FIRST result event to avoid duplication from multi-turn conversations
			if resultCount == 1 {
				response.WriteString(event.Message)
			}
		case agent.StreamEventError:
			return nil, fmt.Errorf("claude error: %s", event.Error)
		}
	}

	if err := d.claude.Wait(); err != nil {
		return nil, fmt.Errorf("wait for claude: %w", err)
	}

	// Debug: save response to file for troubleshooting
	responseStr := response.String()
	if debugFile, err := os.Create("/tmp/alphie-decompose-response.txt"); err == nil {
		debugFile.WriteString(responseStr)
		debugFile.Close()
	}

	tasks, err := ParseResponse(responseStr)
	if err != nil {
		return nil, fmt.Errorf("parse decomposition response: %w", err)
	}

	if err := ValidateNoCycles(tasks); err != nil {
		return nil, fmt.Errorf("validate dependencies: %w", err)
	}

	// DISABLED: Coalescing creates overly large tasks that agents can't complete
	// The prompt already instructs to minimize SETUP tasks, so additional coalescing
	// just makes things worse by bundling too much work into single tasks
	// tasks = CoalesceSetupTasks(tasks)

	return tasks, nil
}

// DecomposeWithReview performs decomposition with quality scoring and optional user review.
// Returns the tasks, quality assessment, and validation result.
func (d *Decomposer) DecomposeWithReview(ctx context.Context, request string, repoPath string, requireApproval bool) ([]*models.Task, *DecompositionQuality, error) {
	// Phase 1: Generate decomposition
	tasks, err := d.Decompose(ctx, request)
	if err != nil {
		return nil, nil, fmt.Errorf("decompose: %w", err)
	}

	// Phase 2: Score quality
	quality := ScoreDecomposition(tasks)

	// Phase 3: Validate against repo structure
	validator := NewValidator(repoPath)
	validation := validator.Validate(tasks)

	// Phase 4: User review (if required)
	if requireApproval {
		reviewer := NewDecompositionReviewer()
		decision, err := reviewer.Review(ctx, tasks, quality, validation)
		if err != nil {
			return nil, nil, fmt.Errorf("review failed: %w", err)
		}

		if !decision.Approved {
			return nil, nil, fmt.Errorf("decomposition rejected by user: %s", decision.Reason)
		}

		// If user modified tasks, re-validate
		if decision.Modified {
			if err := ValidateNoCycles(decision.Tasks); err != nil {
				return nil, nil, fmt.Errorf("modified decomposition has cycles: %w", err)
			}
			tasks = decision.Tasks
			// Re-score after modifications
			quality = ScoreDecomposition(tasks)
		}
	} else {
		// Auto-approve if confidence is above threshold
		if quality.OverallConfidence < 0.7 {
			return nil, &quality, fmt.Errorf("decomposition confidence too low (%.0f%%) for auto-approval", quality.OverallConfidence*100)
		}
		if !validation.Valid {
			return nil, &quality, fmt.Errorf("decomposition validation failed: %v", validation.Errors)
		}
	}

	return tasks, &quality, nil
}

// ParseResponse parses Claude's JSON response into Task objects.
func ParseResponse(response string) ([]*models.Task, error) {
	jsonStart := strings.Index(response, "[")
	jsonEnd := strings.LastIndex(response, "]")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		// Save failed response for debugging
		responsePreview := response
		if len(responsePreview) > 500 {
			responsePreview = responsePreview[:500] + "... (truncated)"
		}
		return nil, fmt.Errorf("no valid JSON array found in response (got %d chars): %q", len(response), responsePreview)
	}
	jsonStr := response[jsonStart : jsonEnd+1]

	var decomposed []decomposedTask
	if err := json.Unmarshal([]byte(jsonStr), &decomposed); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	if len(decomposed) == 0 {
		return nil, fmt.Errorf("empty task list returned")
	}

	titleToID := make(map[string]string)
	tasks := make([]*models.Task, len(decomposed))
	now := time.Now()

	for i, dt := range decomposed {
		id := uuid.New().String()
		titleToID[dt.Title] = id

		var taskType models.TaskType
		switch strings.ToUpper(dt.TaskType) {
		case "SETUP":
			taskType = models.TaskTypeSetup
		case "FEATURE":
			taskType = models.TaskTypeFeature
		case "BUGFIX":
			taskType = models.TaskTypeBugfix
		case "REFACTOR":
			taskType = models.TaskTypeRefactor
		default:
			taskType = models.TaskTypeFeature
		}

		verificationIntent := dt.VerificationIntent
		if verificationIntent == "" && dt.AcceptanceCriteria != "" {
			verificationIntent = dt.AcceptanceCriteria
		}

		tasks[i] = &models.Task{
			ID:                 id,
			Title:              dt.Title,
			Description:        dt.Description,
			TaskType:           taskType,
			FileBoundaries:     dt.FileBoundaries,
			AcceptanceCriteria: dt.AcceptanceCriteria,
			VerificationIntent: verificationIntent,
			Status:             models.TaskStatusPending,
			CreatedAt:          now,
		}
	}

	for i, dt := range decomposed {
		for _, depTitle := range dt.DependsOn {
			depID, ok := titleToID[depTitle]
			if !ok {
				return nil, fmt.Errorf("unknown dependency %q for task %q", depTitle, dt.Title)
			}
			tasks[i].DependsOn = append(tasks[i].DependsOn, depID)
		}
	}

	return tasks, nil
}

// ValidateNoCycles checks that there are no circular dependencies among tasks.
func ValidateNoCycles(tasks []*models.Task) error {
	idToTask := make(map[string]*models.Task)
	for _, task := range tasks {
		idToTask[task.ID] = task
	}

	state := make(map[string]int) // 0=unvisited, 1=visiting, 2=visited

	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		if state[id] == 2 {
			return nil
		}
		if state[id] == 1 {
			cycleStart := 0
			for i, p := range path {
				if p == id {
					cycleStart = i
					break
				}
			}
			cycle := append(path[cycleStart:], id)
			return fmt.Errorf("circular dependency detected: %s", strings.Join(cycle, " -> "))
		}

		state[id] = 1
		task := idToTask[id]
		if task != nil {
			for _, depID := range task.DependsOn {
				if err := visit(depID, append(path, id)); err != nil {
					return err
				}
			}
		}
		state[id] = 2
		return nil
	}

	for _, task := range tasks {
		if state[task.ID] == 0 {
			if err := visit(task.ID, nil); err != nil {
				return err
			}
		}
	}

	return nil
}
