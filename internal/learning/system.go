// Package learning provides learning and context management capabilities.
package learning

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// LearningSystem integrates all learning components and provides
// hooks for agent execution.
type LearningSystem struct {
	store     *LearningStore
	retriever *Retriever
	lifecycle *LifecycleManager
	concepts  *ConceptManager
}

// NewLearningSystem creates a new LearningSystem with all components wired together.
// It opens the database at dbPath, runs migrations, and initializes all sub-components.
func NewLearningSystem(dbPath string) (*LearningSystem, error) {
	store, err := NewLearningStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("create store: %w", err)
	}

	if err := store.Migrate(); err != nil {
		store.Close()
		return nil, fmt.Errorf("migrate store: %w", err)
	}

	ls := &LearningSystem{
		store:     store,
		retriever: NewRetriever(store),
		lifecycle: NewLifecycleManager(store, 0), // Use default TTL
		concepts:  NewConceptManager(store),
	}

	// Run cleanup of stale learnings on system init
	if _, err := ls.lifecycle.CleanupStale(); err != nil {
		// Log but don't fail - cleanup is not critical for startup
		// The system can operate with stale learnings present
	}

	return ls, nil
}

// Close closes the learning system and releases all resources.
func (ls *LearningSystem) Close() error {
	if ls.store != nil {
		return ls.store.Close()
	}
	return nil
}

// OnTaskStart is called at the beginning of a task to retrieve relevant learnings.
// It retrieves learnings based on task description and file paths,
// records triggers for matched learnings, and returns them for injection
// into the agent context.
func (ls *LearningSystem) OnTaskStart(taskDescription string, filePaths []string) ([]*Learning, error) {
	// Step 1: Retrieve relevant learnings
	learnings, err := ls.retriever.RetrieveForTask(taskDescription, filePaths)
	if err != nil {
		return nil, fmt.Errorf("retrieve learnings: %w", err)
	}

	// Step 2: Record trigger for each used learning
	for _, learning := range learnings {
		if err := ls.lifecycle.RecordTrigger(learning.ID); err != nil {
			// Log but don't fail - trigger recording is not critical
			continue
		}
	}

	return learnings, nil
}

// OnTaskComplete is called when a task finishes to handle learning opportunities.
// If the task failed with an unknown pattern, it returns information suggesting
// a candidate learning should be created. If the task succeeded with a novel
// approach, it may suggest learning creation as well.
func (ls *LearningSystem) OnTaskComplete(taskID string, success bool) error {
	// For now, this is a placeholder for future integration.
	// The actual learning creation would be triggered by the caller
	// based on task context and user feedback.
	//
	// Future enhancements could include:
	// - Analyzing task execution logs for patterns
	// - Comparing with existing learnings to detect novelty
	// - Auto-suggesting learnings based on error-to-resolution patterns
	return nil
}

// OnFailure is called when an error occurs to check for existing learnings
// that match the error pattern. If found, it returns the learnings and
// records their triggers. If no matching learning exists, it returns nil
// to signal the caller may want to create a new learning.
func (ls *LearningSystem) OnFailure(errorMessage string) ([]*Learning, error) {
	// Step 1: Check if learnings exist for this error
	learnings, err := ls.retriever.RetrieveForError(errorMessage)
	if err != nil {
		return nil, fmt.Errorf("retrieve for error: %w", err)
	}

	// Step 2: If found, record triggers
	for _, learning := range learnings {
		if err := ls.lifecycle.RecordTrigger(learning.ID); err != nil {
			// Log but don't fail
			continue
		}
	}

	// Step 3: Return learnings (may be empty if no matches)
	return learnings, nil
}

// AddLearning creates a new learning from a CAO triple and associates it
// with the specified concepts. It generates a unique ID and stores the learning.
func (ls *LearningSystem) AddLearning(cao *CAOTriple, conceptNames []string) (*Learning, error) {
	if cao == nil {
		return nil, fmt.Errorf("cao triple is required")
	}

	if err := cao.Validate(); err != nil {
		return nil, fmt.Errorf("invalid cao triple: %w", err)
	}

	// Generate unique ID
	id := generateLearningID()

	learning := &Learning{
		ID:          id,
		Condition:   cao.Condition,
		Action:      cao.Action,
		Outcome:     cao.Outcome,
		Scope:       "repo",
		OutcomeType: "neutral",
		CreatedAt:   time.Now(),
	}

	// Store the learning
	if err := ls.store.Create(learning); err != nil {
		return nil, fmt.Errorf("create learning: %w", err)
	}

	// Associate with concepts
	for _, name := range conceptNames {
		// Find or create concept
		concept, err := ls.concepts.GetByName(name)
		if err != nil {
			return nil, fmt.Errorf("get concept %q: %w", name, err)
		}

		if concept == nil {
			// Create new concept
			concept = &Concept{
				ID:        generateConceptID(),
				Name:      name,
				CreatedAt: time.Now(),
			}
			if err := ls.concepts.Create(concept); err != nil {
				return nil, fmt.Errorf("create concept %q: %w", name, err)
			}
		}

		// Add relationship
		if err := ls.concepts.AddLearningToConcept(learning.ID, concept.ID); err != nil {
			return nil, fmt.Errorf("link learning to concept %q: %w", name, err)
		}
	}

	return learning, nil
}

// GetStats returns lifecycle statistics about the learnings in the system.
func (ls *LearningSystem) GetStats() (*LifecycleStats, error) {
	return ls.lifecycle.GetHealthStats()
}

// ExportData represents the complete export format for learnings.
type ExportData struct {
	Version   string             `json:"version"`
	ExportedAt time.Time         `json:"exported_at"`
	Learnings []*ExportLearning  `json:"learnings"`
	Concepts  []*ExportConcept   `json:"concepts"`
}

// ExportLearning represents a learning with its concept associations for export.
type ExportLearning struct {
	ID            string        `json:"id"`
	Condition     string        `json:"condition"`
	Action        string        `json:"action"`
	Outcome       string        `json:"outcome"`
	CommitHash    string        `json:"commit_hash,omitempty"`
	LogSnippetID  string        `json:"log_snippet_id,omitempty"`
	Scope         string        `json:"scope"`
	TTL           time.Duration `json:"ttl"`
	LastTriggered time.Time     `json:"last_triggered,omitempty"`
	TriggerCount  int           `json:"trigger_count"`
	OutcomeType   string        `json:"outcome_type"`
	CreatedAt     time.Time     `json:"created_at"`
	Concepts      []string      `json:"concepts,omitempty"`
}

// ExportConcept represents a concept for export.
type ExportConcept struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Export exports all learnings and concepts to a JSON file at the given path.
func (ls *LearningSystem) Export(path string) error {
	// Get all learnings
	learnings, err := ls.store.List(10000) // Large limit to get all
	if err != nil {
		return fmt.Errorf("list learnings: %w", err)
	}

	// Get all concepts
	concepts, err := ls.concepts.List()
	if err != nil {
		return fmt.Errorf("list concepts: %w", err)
	}

	// Build export data
	data := &ExportData{
		Version:    "1.0",
		ExportedAt: time.Now(),
		Learnings:  make([]*ExportLearning, 0, len(learnings)),
		Concepts:   make([]*ExportConcept, 0, len(concepts)),
	}

	// Export learnings with their concept associations
	for _, l := range learnings {
		// Get concepts for this learning
		lConcepts, err := ls.concepts.GetConceptsForLearning(l.ID)
		if err != nil {
			return fmt.Errorf("get concepts for learning %s: %w", l.ID, err)
		}

		conceptNames := make([]string, len(lConcepts))
		for i, c := range lConcepts {
			conceptNames[i] = c.Name
		}

		data.Learnings = append(data.Learnings, &ExportLearning{
			ID:            l.ID,
			Condition:     l.Condition,
			Action:        l.Action,
			Outcome:       l.Outcome,
			CommitHash:    l.CommitHash,
			LogSnippetID:  l.LogSnippetID,
			Scope:         l.Scope,
			TTL:           l.TTL,
			LastTriggered: l.LastTriggered,
			TriggerCount:  l.TriggerCount,
			OutcomeType:   l.OutcomeType,
			CreatedAt:     l.CreatedAt,
			Concepts:      conceptNames,
		})
	}

	// Export concepts
	for _, c := range concepts {
		data.Concepts = append(data.Concepts, &ExportConcept{
			ID:        c.ID,
			Name:      c.Name,
			Summary:   c.Summary,
			CreatedAt: c.CreatedAt,
		})
	}

	// Write to file
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("encode export data: %w", err)
	}

	return nil
}

// Import imports learnings and concepts from a JSON file at the given path.
// It merges with existing data, skipping duplicates by ID.
func (ls *LearningSystem) Import(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open import file: %w", err)
	}
	defer file.Close()

	var data ExportData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return fmt.Errorf("decode import data: %w", err)
	}

	// Import concepts first (learnings may reference them)
	conceptIDMap := make(map[string]string) // old ID -> new ID
	for _, c := range data.Concepts {
		// Check if concept already exists by name
		existing, err := ls.concepts.GetByName(c.Name)
		if err != nil {
			return fmt.Errorf("check concept %q: %w", c.Name, err)
		}

		if existing != nil {
			conceptIDMap[c.ID] = existing.ID
			continue
		}

		// Create new concept with new ID
		newID := generateConceptID()
		conceptIDMap[c.ID] = newID

		concept := &Concept{
			ID:        newID,
			Name:      c.Name,
			Summary:   c.Summary,
			CreatedAt: c.CreatedAt,
		}
		if err := ls.concepts.Create(concept); err != nil {
			return fmt.Errorf("create concept %q: %w", c.Name, err)
		}
	}

	// Import learnings
	for _, l := range data.Learnings {
		// Check if learning already exists
		existing, err := ls.store.Get(l.ID)
		if err != nil {
			return fmt.Errorf("check learning %s: %w", l.ID, err)
		}

		if existing != nil {
			// Skip duplicate
			continue
		}

		// Create new learning with new ID to avoid conflicts
		newID := generateLearningID()
		learning := &Learning{
			ID:            newID,
			Condition:     l.Condition,
			Action:        l.Action,
			Outcome:       l.Outcome,
			CommitHash:    l.CommitHash,
			LogSnippetID:  l.LogSnippetID,
			Scope:         l.Scope,
			TTL:           l.TTL,
			LastTriggered: l.LastTriggered,
			TriggerCount:  l.TriggerCount,
			OutcomeType:   l.OutcomeType,
			CreatedAt:     l.CreatedAt,
		}

		if err := ls.store.Create(learning); err != nil {
			return fmt.Errorf("create learning: %w", err)
		}

		// Link to concepts by name
		for _, conceptName := range l.Concepts {
			concept, err := ls.concepts.GetByName(conceptName)
			if err != nil {
				return fmt.Errorf("get concept %q for learning: %w", conceptName, err)
			}
			if concept == nil {
				// Create concept if it doesn't exist
				concept = &Concept{
					ID:        generateConceptID(),
					Name:      conceptName,
					CreatedAt: time.Now(),
				}
				if err := ls.concepts.Create(concept); err != nil {
					return fmt.Errorf("create concept %q: %w", conceptName, err)
				}
			}
			if err := ls.concepts.AddLearningToConcept(learning.ID, concept.ID); err != nil {
				return fmt.Errorf("link learning to concept %q: %w", conceptName, err)
			}
		}
	}

	return nil
}

// generateLearningID generates a unique ID for a learning.
func generateLearningID() string {
	return fmt.Sprintf("ln-%d", time.Now().UnixNano())
}

// generateConceptID generates a unique ID for a concept.
func generateConceptID() string {
	return fmt.Sprintf("cp-%d", time.Now().UnixNano())
}

// Store returns the underlying LearningStore for direct access.
func (ls *LearningSystem) Store() *LearningStore {
	return ls.store
}

// Retriever returns the underlying Retriever for direct access.
func (ls *LearningSystem) Retriever() *Retriever {
	return ls.retriever
}

// Lifecycle returns the underlying LifecycleManager for direct access.
func (ls *LearningSystem) Lifecycle() *LifecycleManager {
	return ls.lifecycle
}

// Concepts returns the underlying ConceptManager for direct access.
func (ls *LearningSystem) Concepts() *ConceptManager {
	return ls.concepts
}

// CleanupStale delegates to the lifecycle manager to remove stale learnings.
func (ls *LearningSystem) CleanupStale() (int, error) {
	return ls.lifecycle.CleanupStale()
}
