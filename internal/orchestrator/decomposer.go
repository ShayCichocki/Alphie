// Package orchestrator provides task decomposition and coordination.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/shayc/alphie/internal/agent"
	"github.com/shayc/alphie/pkg/models"
)

// decompositionPrompt is the prompt template for task decomposition.
const decompositionPrompt = `Break this user request into parallelizable subtasks. Each task should be sized for a single agent to complete.

User request:
%s

Return ONLY a JSON array of tasks with this exact structure (no other text):
[
  {
    "title": "Short task title",
    "description": "Detailed task description",
    "depends_on": ["title of dependency 1", "title of dependency 2"],
    "acceptance_criteria": "Criteria to verify this task is complete"
  }
]

Guidelines:
- Tasks should be as independent as possible to allow parallel execution
- Only add dependencies when truly necessary (task A must complete before task B)
- Each task should be completable by a single agent in one session
- Acceptance criteria should be specific and verifiable
- Use empty array [] for depends_on if there are no dependencies`

// decomposedTask is the JSON structure returned by Claude for a single task.
type decomposedTask struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	DependsOn          []string `json:"depends_on"`
	AcceptanceCriteria string   `json:"acceptance_criteria"`
}

// Decomposer breaks down user requests into parallelizable subtasks.
type Decomposer struct {
	// claude is the Claude process used for task decomposition.
	claude *agent.ClaudeProcess
}

// NewDecomposer creates a new Decomposer with the given Claude process.
func NewDecomposer(claude *agent.ClaudeProcess) *Decomposer {
	return &Decomposer{
		claude: claude,
	}
}

// Decompose takes a user request and returns a list of tasks with dependencies.
// It prompts Claude to break down the request, parses the structured JSON response,
// creates Task objects with DependsOn fields, and validates there are no circular dependencies.
func (d *Decomposer) Decompose(ctx context.Context, request string) ([]*models.Task, error) {
	prompt := fmt.Sprintf(decompositionPrompt, request)

	// Start the Claude process with the decomposition prompt
	if err := d.claude.Start(prompt, ""); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	// Collect the response
	var response strings.Builder
	for event := range d.claude.Output() {
		select {
		case <-ctx.Done():
			_ = d.claude.Kill()
			return nil, ctx.Err()
		default:
		}

		switch event.Type {
		case agent.StreamEventResult:
			response.WriteString(event.Message)
		case agent.StreamEventAssistant:
			response.WriteString(event.Message)
		case agent.StreamEventError:
			return nil, fmt.Errorf("claude error: %s", event.Error)
		}
	}

	// Wait for process to complete
	if err := d.claude.Wait(); err != nil {
		return nil, fmt.Errorf("wait for claude: %w", err)
	}

	// Parse the response
	tasks, err := parseDecompositionResponse(response.String())
	if err != nil {
		return nil, fmt.Errorf("parse decomposition response: %w", err)
	}

	// Validate no circular dependencies
	if err := validateNoCycles(tasks); err != nil {
		return nil, fmt.Errorf("validate dependencies: %w", err)
	}

	return tasks, nil
}

// parseDecompositionResponse parses Claude's JSON response into Task objects.
func parseDecompositionResponse(response string) ([]*models.Task, error) {
	// Find the JSON array in the response (Claude might include extra text)
	jsonStart := strings.Index(response, "[")
	jsonEnd := strings.LastIndex(response, "]")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		return nil, fmt.Errorf("no valid JSON array found in response")
	}
	jsonStr := response[jsonStart : jsonEnd+1]

	var decomposed []decomposedTask
	if err := json.Unmarshal([]byte(jsonStr), &decomposed); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w", err)
	}

	if len(decomposed) == 0 {
		return nil, fmt.Errorf("empty task list returned")
	}

	// Build a title to ID mapping for dependency resolution
	titleToID := make(map[string]string)
	tasks := make([]*models.Task, len(decomposed))
	now := time.Now()

	for i, dt := range decomposed {
		id := uuid.New().String()
		titleToID[dt.Title] = id
		tasks[i] = &models.Task{
			ID:                 id,
			Title:              dt.Title,
			Description:        dt.Description,
			AcceptanceCriteria: dt.AcceptanceCriteria,
			Status:             models.TaskStatusPending,
			CreatedAt:          now,
		}
	}

	// Resolve dependency titles to IDs
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

// validateNoCycles checks that there are no circular dependencies among tasks.
// Returns an error if a cycle is detected.
func validateNoCycles(tasks []*models.Task) error {
	// Build adjacency list
	idToTask := make(map[string]*models.Task)
	for _, task := range tasks {
		idToTask[task.ID] = task
	}

	// Track visit state: 0=unvisited, 1=visiting, 2=visited
	state := make(map[string]int)

	// DFS to detect cycles
	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		if state[id] == 2 {
			return nil // Already fully visited
		}
		if state[id] == 1 {
			// Found a cycle - build error message
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

		state[id] = 1 // Mark as visiting
		task := idToTask[id]
		if task != nil {
			for _, depID := range task.DependsOn {
				if err := visit(depID, append(path, id)); err != nil {
					return err
				}
			}
		}
		state[id] = 2 // Mark as visited
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
