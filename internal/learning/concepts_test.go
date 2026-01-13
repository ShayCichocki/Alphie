package learning

import (
	"testing"
	"time"
)

func TestNewConceptManager(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	if cm == nil {
		t.Fatal("NewConceptManager() returned nil")
	}
}

func TestConceptManager_Create(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		Summary:   "Testing best practices",
		CreatedAt: time.Now().UTC(),
	}

	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
}

func TestConceptManager_Create_DuplicateName(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	concept1 := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		Summary:   "Testing best practices",
		CreatedAt: now,
	}

	concept2 := &Concept{
		ID:        "concept-2",
		Name:      "testing", // Same name
		Summary:   "Another testing concept",
		CreatedAt: now,
	}

	if err := cm.Create(concept1); err != nil {
		t.Fatalf("Create() first concept error = %v", err)
	}

	err := cm.Create(concept2)
	if err == nil {
		t.Error("Create() error = nil, want error for duplicate name")
	}
}

func TestConceptManager_Get(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC().Round(time.Second)

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		Summary:   "Testing best practices",
		CreatedAt: now,
	}

	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := cm.Get("concept-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}

	if got.ID != concept.ID {
		t.Errorf("ID = %v, want %v", got.ID, concept.ID)
	}
	if got.Name != concept.Name {
		t.Errorf("Name = %v, want %v", got.Name, concept.Name)
	}
	if got.Summary != concept.Summary {
		t.Errorf("Summary = %v, want %v", got.Summary, concept.Summary)
	}
}

func TestConceptManager_Get_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	got, err := cm.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("Get() = %v, want nil for nonexistent ID", got)
	}
}

func TestConceptManager_GetByName(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		Summary:   "Testing best practices",
		CreatedAt: time.Now().UTC(),
	}

	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := cm.GetByName("testing")
	if err != nil {
		t.Fatalf("GetByName() error = %v, want nil", err)
	}

	if got == nil {
		t.Fatal("GetByName() returned nil, want concept")
	}

	if got.ID != concept.ID {
		t.Errorf("ID = %v, want %v", got.ID, concept.ID)
	}
}

func TestConceptManager_GetByName_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	got, err := cm.GetByName("nonexistent")
	if err != nil {
		t.Fatalf("GetByName() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("GetByName() = %v, want nil for nonexistent name", got)
	}
}

func TestConceptManager_List(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	concepts := []*Concept{
		{ID: "concept-1", Name: "testing", Summary: "Testing", CreatedAt: now},
		{ID: "concept-2", Name: "architecture", Summary: "Architecture", CreatedAt: now},
		{ID: "concept-3", Name: "debugging", Summary: "Debugging", CreatedAt: now},
	}

	for _, c := range concepts {
		if err := cm.Create(c); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	got, err := cm.List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	if len(got) != 3 {
		t.Errorf("List() returned %d concepts, want 3", len(got))
	}

	// Should be ordered by name
	if got[0].Name != "architecture" {
		t.Errorf("First concept name = %v, want architecture", got[0].Name)
	}
	if got[1].Name != "debugging" {
		t.Errorf("Second concept name = %v, want debugging", got[1].Name)
	}
	if got[2].Name != "testing" {
		t.Errorf("Third concept name = %v, want testing", got[2].Name)
	}
}

func TestConceptManager_List_Empty(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	got, err := cm.List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	if len(got) != 0 {
		t.Errorf("List() returned %d concepts, want 0", len(got))
	}
}

func TestConceptManager_Delete(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		Summary:   "Testing best practices",
		CreatedAt: time.Now().UTC(),
	}

	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := cm.Delete("concept-1"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify deletion
	got, err := cm.Get("concept-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Error("Concept should have been deleted")
	}
}

func TestConceptManager_Delete_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	err := cm.Delete("nonexistent")
	if err == nil {
		t.Error("Delete() error = nil, want error for nonexistent ID")
	}
}

func TestConceptManager_AddLearningToConcept(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	// Create a learning
	learning := &Learning{
		ID:          "learning-1",
		Condition:   "test condition",
		Action:      "test action",
		Outcome:     "test outcome",
		Scope:       "repo",
		OutcomeType: "success",
		CreatedAt:   now,
	}
	if err := store.Create(learning); err != nil {
		t.Fatalf("Create learning error = %v", err)
	}

	// Create a concept
	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		Summary:   "Testing best practices",
		CreatedAt: now,
	}
	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create concept error = %v", err)
	}

	// Link learning to concept
	if err := cm.AddLearningToConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() error = %v, want nil", err)
	}
}

func TestConceptManager_AddLearningToConcept_Idempotent(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	// Create learning and concept
	learning := &Learning{
		ID:          "learning-1",
		Condition:   "test condition",
		Action:      "test action",
		Outcome:     "test outcome",
		Scope:       "repo",
		OutcomeType: "success",
		CreatedAt:   now,
	}
	if err := store.Create(learning); err != nil {
		t.Fatalf("Create learning error = %v", err)
	}

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		CreatedAt: now,
	}
	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create concept error = %v", err)
	}

	// Link twice - should not error
	if err := cm.AddLearningToConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() first call error = %v", err)
	}
	if err := cm.AddLearningToConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() second call error = %v (should be idempotent)", err)
	}
}

func TestConceptManager_RemoveLearningFromConcept(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	// Create learning and concept
	learning := &Learning{
		ID:          "learning-1",
		Condition:   "test condition",
		Action:      "test action",
		Outcome:     "test outcome",
		Scope:       "repo",
		OutcomeType: "success",
		CreatedAt:   now,
	}
	if err := store.Create(learning); err != nil {
		t.Fatalf("Create learning error = %v", err)
	}

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		CreatedAt: now,
	}
	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create concept error = %v", err)
	}

	// Link then unlink
	if err := cm.AddLearningToConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() error = %v", err)
	}

	if err := cm.RemoveLearningFromConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("RemoveLearningFromConcept() error = %v, want nil", err)
	}
}

func TestConceptManager_RemoveLearningFromConcept_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	err := cm.RemoveLearningFromConcept("learning-1", "concept-1")
	if err == nil {
		t.Error("RemoveLearningFromConcept() error = nil, want error for nonexistent relationship")
	}
}

func TestConceptManager_GetLearningsByConcept(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	// Create learnings
	learnings := []*Learning{
		{ID: "learning-1", Condition: "cond1", Action: "act1", Outcome: "out1", Scope: "repo", OutcomeType: "success", CreatedAt: now},
		{ID: "learning-2", Condition: "cond2", Action: "act2", Outcome: "out2", Scope: "repo", OutcomeType: "success", CreatedAt: now.Add(1 * time.Second)},
		{ID: "learning-3", Condition: "cond3", Action: "act3", Outcome: "out3", Scope: "repo", OutcomeType: "success", CreatedAt: now.Add(2 * time.Second)},
	}
	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create learning error = %v", err)
		}
	}

	// Create concepts
	concept := &Concept{ID: "concept-1", Name: "testing", CreatedAt: now}
	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create concept error = %v", err)
	}

	// Link learning-1 and learning-2 to concept-1
	if err := cm.AddLearningToConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() error = %v", err)
	}
	if err := cm.AddLearningToConcept("learning-2", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() error = %v", err)
	}

	// Get learnings by concept
	got, err := cm.GetLearningsByConcept("concept-1")
	if err != nil {
		t.Fatalf("GetLearningsByConcept() error = %v, want nil", err)
	}

	if len(got) != 2 {
		t.Errorf("GetLearningsByConcept() returned %d learnings, want 2", len(got))
	}

	// Should be ordered by created_at DESC
	if got[0].ID != "learning-2" {
		t.Errorf("First learning ID = %v, want learning-2", got[0].ID)
	}
}

func TestConceptManager_GetConceptsForLearning(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	// Create learning
	learning := &Learning{
		ID:          "learning-1",
		Condition:   "test condition",
		Action:      "test action",
		Outcome:     "test outcome",
		Scope:       "repo",
		OutcomeType: "success",
		CreatedAt:   now,
	}
	if err := store.Create(learning); err != nil {
		t.Fatalf("Create learning error = %v", err)
	}

	// Create concepts
	concepts := []*Concept{
		{ID: "concept-1", Name: "testing", CreatedAt: now},
		{ID: "concept-2", Name: "debugging", CreatedAt: now},
		{ID: "concept-3", Name: "architecture", CreatedAt: now},
	}
	for _, c := range concepts {
		if err := cm.Create(c); err != nil {
			t.Fatalf("Create concept error = %v", err)
		}
	}

	// Link learning to concept-1 and concept-2
	if err := cm.AddLearningToConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() error = %v", err)
	}
	if err := cm.AddLearningToConcept("learning-1", "concept-2"); err != nil {
		t.Fatalf("AddLearningToConcept() error = %v", err)
	}

	// Get concepts for learning
	got, err := cm.GetConceptsForLearning("learning-1")
	if err != nil {
		t.Fatalf("GetConceptsForLearning() error = %v, want nil", err)
	}

	if len(got) != 2 {
		t.Errorf("GetConceptsForLearning() returned %d concepts, want 2", len(got))
	}

	// Should be ordered by name
	if got[0].Name != "debugging" {
		t.Errorf("First concept name = %v, want debugging", got[0].Name)
	}
	if got[1].Name != "testing" {
		t.Errorf("Second concept name = %v, want testing", got[1].Name)
	}
}

func TestConceptManager_SuggestConcepts(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	// Create concepts with various names and descriptions
	concepts := []*Concept{
		{ID: "concept-1", Name: "testing", Summary: "Unit and integration testing practices", CreatedAt: now},
		{ID: "concept-2", Name: "debugging", Summary: "Techniques for finding bugs", CreatedAt: now},
		{ID: "concept-3", Name: "performance", Summary: "Optimization and profiling", CreatedAt: now},
	}
	for _, c := range concepts {
		if err := cm.Create(c); err != nil {
			t.Fatalf("Create concept error = %v", err)
		}
	}

	// Suggest based on content matching name
	got, err := cm.SuggestConcepts("need help with testing")
	if err != nil {
		t.Fatalf("SuggestConcepts() error = %v, want nil", err)
	}

	if len(got) == 0 {
		t.Fatal("SuggestConcepts() returned no results")
	}

	found := false
	for _, c := range got {
		if c.Name == "testing" {
			found = true
			break
		}
	}
	if !found {
		t.Error("SuggestConcepts() should have suggested 'testing' concept")
	}
}

func TestConceptManager_SuggestConcepts_MatchesDescription(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	concept := &Concept{
		ID:        "concept-1",
		Name:      "qa",
		Summary:   "Quality assurance and integration testing",
		CreatedAt: now,
	}
	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create concept error = %v", err)
	}

	// Suggest based on content matching description
	got, err := cm.SuggestConcepts("integration testing")
	if err != nil {
		t.Fatalf("SuggestConcepts() error = %v, want nil", err)
	}

	if len(got) == 0 {
		t.Fatal("SuggestConcepts() returned no results")
	}

	if got[0].ID != "concept-1" {
		t.Errorf("SuggestConcepts() first result ID = %v, want concept-1", got[0].ID)
	}
}

func TestConceptManager_SuggestConcepts_EmptyContent(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)

	got, err := cm.SuggestConcepts("")
	if err != nil {
		t.Fatalf("SuggestConcepts() error = %v, want nil", err)
	}

	if got != nil {
		t.Errorf("SuggestConcepts() = %v, want nil for empty content", got)
	}
}

func TestConceptManager_SuggestConcepts_NoMatches(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		Summary:   "Testing practices",
		CreatedAt: now,
	}
	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create concept error = %v", err)
	}

	got, err := cm.SuggestConcepts("kubernetes deployment")
	if err != nil {
		t.Fatalf("SuggestConcepts() error = %v, want nil", err)
	}

	if len(got) != 0 {
		t.Errorf("SuggestConcepts() returned %d concepts, want 0 for no matches", len(got))
	}
}

func TestConceptManager_Delete_CascadesLinks(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	cm := NewConceptManager(store)
	now := time.Now().UTC()

	// Create learning and concept
	learning := &Learning{
		ID:          "learning-1",
		Condition:   "test condition",
		Action:      "test action",
		Outcome:     "test outcome",
		Scope:       "repo",
		OutcomeType: "success",
		CreatedAt:   now,
	}
	if err := store.Create(learning); err != nil {
		t.Fatalf("Create learning error = %v", err)
	}

	concept := &Concept{
		ID:        "concept-1",
		Name:      "testing",
		CreatedAt: now,
	}
	if err := cm.Create(concept); err != nil {
		t.Fatalf("Create concept error = %v", err)
	}

	// Link them
	if err := cm.AddLearningToConcept("learning-1", "concept-1"); err != nil {
		t.Fatalf("AddLearningToConcept() error = %v", err)
	}

	// Delete concept
	if err := cm.Delete("concept-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify learning still exists
	got, err := store.Get("learning-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Error("Learning should still exist after concept deletion")
	}

	// Verify concepts for learning is empty
	concepts, err := cm.GetConceptsForLearning("learning-1")
	if err != nil {
		t.Fatalf("GetConceptsForLearning() error = %v", err)
	}
	if len(concepts) != 0 {
		t.Errorf("GetConceptsForLearning() returned %d concepts, want 0 after cascade delete", len(concepts))
	}
}
