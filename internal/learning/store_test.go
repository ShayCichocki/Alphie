package learning

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore creates a temporary LearningStore for testing.
// The caller should call cleanup() when done.
func newTestStore(t *testing.T) (*LearningStore, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "learning-store-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewLearningStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create store: %v", err)
	}

	if err := store.Migrate(); err != nil {
		store.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to migrate: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}

func TestNewLearningStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "learning-store-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewLearningStore(dbPath)
	if err != nil {
		t.Fatalf("NewLearningStore() error = %v, want nil", err)
	}
	defer store.Close()

	if store.Path() != dbPath {
		t.Errorf("Path() = %v, want %v", store.Path(), dbPath)
	}
}

func TestNewLearningStore_CreatesDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "learning-store-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Use a nested path that doesn't exist
	dbPath := filepath.Join(tmpDir, "nested", "path", "test.db")
	store, err := NewLearningStore(dbPath)
	if err != nil {
		t.Fatalf("NewLearningStore() error = %v, want nil", err)
	}
	defer store.Close()

	// Verify directory was created
	dir := filepath.Dir(dbPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("NewLearningStore() did not create parent directory")
	}
}

func TestLearningStore_Migrate(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	// Migrate should be idempotent
	if err := store.Migrate(); err != nil {
		t.Errorf("Migrate() second call error = %v, want nil", err)
	}
}

func TestLearningStore_Create(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	learning := &Learning{
		ID:           "test-1",
		Condition:    "test fails",
		Action:       "check logs",
		Outcome:      "error found",
		Scope:        "repo",
		TTL:          24 * time.Hour,
		OutcomeType:  "success",
		TriggerCount: 0,
		CreatedAt:    time.Now().UTC(),
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
}

func TestLearningStore_Create_WithOptionalFields(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learning := &Learning{
		ID:            "test-2",
		Condition:     "test fails",
		Action:        "check logs",
		Outcome:       "error found",
		CommitHash:    "abc123",
		LogSnippetID:  "log-456",
		Scope:         "module",
		TTL:           48 * time.Hour,
		LastTriggered: now.Add(-1 * time.Hour),
		TriggerCount:  5,
		OutcomeType:   "failure",
		CreatedAt:     now,
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	// Verify by getting
	got, err := store.Get(learning.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.CommitHash != learning.CommitHash {
		t.Errorf("CommitHash = %v, want %v", got.CommitHash, learning.CommitHash)
	}
	if got.LogSnippetID != learning.LogSnippetID {
		t.Errorf("LogSnippetID = %v, want %v", got.LogSnippetID, learning.LogSnippetID)
	}
}

func TestLearningStore_Get(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC().Round(time.Second) // Round for DB storage precision
	learning := &Learning{
		ID:           "test-1",
		Condition:    "test fails",
		Action:       "check logs",
		Outcome:      "error found",
		Scope:        "repo",
		TTL:          24 * time.Hour,
		OutcomeType:  "success",
		TriggerCount: 3,
		CreatedAt:    now,
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := store.Get("test-1")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}

	if got.ID != learning.ID {
		t.Errorf("ID = %v, want %v", got.ID, learning.ID)
	}
	if got.Condition != learning.Condition {
		t.Errorf("Condition = %v, want %v", got.Condition, learning.Condition)
	}
	if got.Action != learning.Action {
		t.Errorf("Action = %v, want %v", got.Action, learning.Action)
	}
	if got.Outcome != learning.Outcome {
		t.Errorf("Outcome = %v, want %v", got.Outcome, learning.Outcome)
	}
	if got.Scope != learning.Scope {
		t.Errorf("Scope = %v, want %v", got.Scope, learning.Scope)
	}
	if got.TTL != learning.TTL {
		t.Errorf("TTL = %v, want %v", got.TTL, learning.TTL)
	}
	if got.TriggerCount != learning.TriggerCount {
		t.Errorf("TriggerCount = %v, want %v", got.TriggerCount, learning.TriggerCount)
	}
	if got.OutcomeType != learning.OutcomeType {
		t.Errorf("OutcomeType = %v, want %v", got.OutcomeType, learning.OutcomeType)
	}
}

func TestLearningStore_Get_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	got, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("Get() = %v, want nil for nonexistent ID", got)
	}
}

func TestLearningStore_Update(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learning := &Learning{
		ID:           "test-1",
		Condition:    "test fails",
		Action:       "check logs",
		Outcome:      "error found",
		Scope:        "repo",
		TTL:          24 * time.Hour,
		OutcomeType:  "success",
		TriggerCount: 0,
		CreatedAt:    now,
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Update the learning
	learning.Condition = "test fails badly"
	learning.Action = "check all logs"
	learning.Outcome = "multiple errors found"
	learning.TriggerCount = 5
	learning.LastTriggered = now.Add(1 * time.Hour)

	if err := store.Update(learning); err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify update
	got, err := store.Get("test-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Condition != "test fails badly" {
		t.Errorf("Condition = %v, want %v", got.Condition, "test fails badly")
	}
	if got.Action != "check all logs" {
		t.Errorf("Action = %v, want %v", got.Action, "check all logs")
	}
	if got.TriggerCount != 5 {
		t.Errorf("TriggerCount = %v, want %v", got.TriggerCount, 5)
	}
}

func TestLearningStore_Update_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	learning := &Learning{
		ID:        "nonexistent",
		Condition: "test fails",
		Action:    "check logs",
		Outcome:   "error found",
	}

	err := store.Update(learning)
	if err == nil {
		t.Errorf("Update() error = nil, want error for nonexistent ID")
	}
}

func TestLearningStore_Delete(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	learning := &Learning{
		ID:          "test-1",
		Condition:   "test fails",
		Action:      "check logs",
		Outcome:     "error found",
		Scope:       "repo",
		OutcomeType: "success",
		CreatedAt:   time.Now().UTC(),
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := store.Delete("test-1"); err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify deletion
	got, err := store.Get("test-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Errorf("Get() after Delete() = %v, want nil", got)
	}
}

func TestLearningStore_Delete_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.Delete("nonexistent")
	if err == nil {
		t.Errorf("Delete() error = nil, want error for nonexistent ID")
	}
}

func TestLearningStore_Search_FTS(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "test-1",
			Condition:   "database connection fails",
			Action:      "check connection string",
			Outcome:     "connection restored",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-2",
			Condition:   "API request timeout",
			Action:      "increase timeout value",
			Outcome:     "request succeeds",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-3",
			Condition:   "database query slow",
			Action:      "add index",
			Outcome:     "query fast",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Search for "database"
	results, err := store.Search("database")
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() returned %d results, want 2", len(results))
	}

	// Verify results contain database-related learnings
	ids := make(map[string]bool)
	for _, r := range results {
		ids[r.ID] = true
	}
	if !ids["test-1"] || !ids["test-3"] {
		t.Errorf("Search() missing expected results, got IDs: %v", ids)
	}
}

func TestLearningStore_Search_FTS_MultipleTerms(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "test-1",
			Condition:   "authentication error",
			Action:      "check token",
			Outcome:     "token refreshed",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-2",
			Condition:   "authorization denied",
			Action:      "check permissions",
			Outcome:     "access granted",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Search using OR
	results, err := store.Search("authentication OR authorization")
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() returned %d results, want 2", len(results))
	}
}

func TestLearningStore_Search_NoResults(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learning := &Learning{
		ID:          "test-1",
		Condition:   "test fails",
		Action:      "check logs",
		Outcome:     "error found",
		Scope:       "repo",
		OutcomeType: "success",
		CreatedAt:   now,
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	results, err := store.Search("nonexistent")
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(results) != 0 {
		t.Errorf("Search() returned %d results, want 0", len(results))
	}
}

func TestLearningStore_List(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "test-1",
			Condition:   "test 1",
			Action:      "action 1",
			Outcome:     "outcome 1",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-2",
			Condition:   "test 2",
			Action:      "action 2",
			Outcome:     "outcome 2",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now.Add(1 * time.Second),
		},
		{
			ID:          "test-3",
			Condition:   "test 3",
			Action:      "action 3",
			Outcome:     "outcome 3",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now.Add(2 * time.Second),
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// List with limit
	results, err := store.List(2)
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	if len(results) != 2 {
		t.Errorf("List(2) returned %d results, want 2", len(results))
	}

	// Should be ordered by created_at DESC
	if results[0].ID != "test-3" {
		t.Errorf("List() first result ID = %v, want test-3", results[0].ID)
	}
}

func TestLearningStore_SearchByCondition(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "test-1",
			Condition:   "error in production environment",
			Action:      "check logs",
			Outcome:     "issue fixed",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-2",
			Condition:   "error in staging environment",
			Action:      "check config",
			Outcome:     "config fixed",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-3",
			Condition:   "build fails locally",
			Action:      "clear cache",
			Outcome:     "build succeeds",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	results, err := store.SearchByCondition("error")
	if err != nil {
		t.Fatalf("SearchByCondition() error = %v, want nil", err)
	}

	if len(results) != 2 {
		t.Errorf("SearchByCondition() returned %d results, want 2", len(results))
	}
}

func TestLearningStore_SearchByPath(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "test-1",
			Condition:   "editing internal/learning/store.go",
			Action:      "run tests",
			Outcome:     "tests pass",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-2",
			Condition:   "editing internal/learning/cao.go",
			Action:      "validate input",
			Outcome:     "validation passes",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "test-3",
			Condition:   "editing cmd/main.go",
			Action:      "rebuild",
			Outcome:     "build succeeds",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	results, err := store.SearchByPath("internal/learning")
	if err != nil {
		t.Fatalf("SearchByPath() error = %v, want nil", err)
	}

	if len(results) != 2 {
		t.Errorf("SearchByPath() returned %d results, want 2", len(results))
	}
}

func TestLearningStore_IncrementTriggerCount(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learning := &Learning{
		ID:           "test-1",
		Condition:    "test fails",
		Action:       "check logs",
		Outcome:      "error found",
		Scope:        "repo",
		OutcomeType:  "success",
		TriggerCount: 0,
		CreatedAt:    now,
	}

	if err := store.Create(learning); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Increment trigger count
	if err := store.IncrementTriggerCount("test-1"); err != nil {
		t.Fatalf("IncrementTriggerCount() error = %v, want nil", err)
	}

	// Verify increment
	got, err := store.Get("test-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.TriggerCount != 1 {
		t.Errorf("TriggerCount = %v, want 1", got.TriggerCount)
	}

	if got.LastTriggered.IsZero() {
		t.Error("LastTriggered should not be zero after increment")
	}

	// Increment again
	if err := store.IncrementTriggerCount("test-1"); err != nil {
		t.Fatalf("IncrementTriggerCount() second call error = %v", err)
	}

	got, _ = store.Get("test-1")
	if got.TriggerCount != 2 {
		t.Errorf("TriggerCount after second increment = %v, want 2", got.TriggerCount)
	}
}

func TestLearningStore_IncrementTriggerCount_NotFound(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	err := store.IncrementTriggerCount("nonexistent")
	if err == nil {
		t.Error("IncrementTriggerCount() error = nil, want error for nonexistent ID")
	}
}

func TestLearningStore_ListByScope(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "repo-1",
			Condition:   "repo learning 1",
			Action:      "action 1",
			Outcome:     "outcome 1",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "global-1",
			Condition:   "global learning 1",
			Action:      "action 2",
			Outcome:     "outcome 2",
			Scope:       "global",
			OutcomeType: "success",
			CreatedAt:   now.Add(1 * time.Second),
		},
		{
			ID:          "module-1",
			Condition:   "module learning 1",
			Action:      "action 3",
			Outcome:     "outcome 3",
			Scope:       "module",
			OutcomeType: "success",
			CreatedAt:   now.Add(2 * time.Second),
		},
		{
			ID:          "repo-2",
			Condition:   "repo learning 2",
			Action:      "action 4",
			Outcome:     "outcome 4",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now.Add(3 * time.Second),
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Test single scope
	results, err := store.ListByScope([]string{"repo"}, 10)
	if err != nil {
		t.Fatalf("ListByScope() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Errorf("ListByScope([repo]) returned %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Scope != "repo" {
			t.Errorf("ListByScope([repo]) returned scope %v, want repo", r.Scope)
		}
	}

	// Test multiple scopes
	results, err = store.ListByScope([]string{"repo", "global"}, 10)
	if err != nil {
		t.Fatalf("ListByScope() error = %v, want nil", err)
	}
	if len(results) != 3 {
		t.Errorf("ListByScope([repo, global]) returned %d results, want 3", len(results))
	}

	// Test empty scopes returns nil
	results, err = store.ListByScope([]string{}, 10)
	if err != nil {
		t.Fatalf("ListByScope([]) error = %v, want nil", err)
	}
	if results != nil {
		t.Errorf("ListByScope([]) returned %v, want nil", results)
	}
}

func TestLearningStore_SearchByScope(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "repo-db",
			Condition:   "database error in repo",
			Action:      "check connection",
			Outcome:     "fixed",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "global-db",
			Condition:   "database error global",
			Action:      "increase pool",
			Outcome:     "resolved",
			Scope:       "global",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "repo-api",
			Condition:   "API error",
			Action:      "retry request",
			Outcome:     "success",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Search for "database" in repo scope only
	results, err := store.SearchByScope("database", []string{"repo"})
	if err != nil {
		t.Fatalf("SearchByScope() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchByScope(database, [repo]) returned %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].ID != "repo-db" {
		t.Errorf("SearchByScope() returned ID %v, want repo-db", results[0].ID)
	}

	// Search for "database" in both scopes
	results, err = store.SearchByScope("database", []string{"repo", "global"})
	if err != nil {
		t.Fatalf("SearchByScope() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Errorf("SearchByScope(database, [repo, global]) returned %d results, want 2", len(results))
	}
}

func TestLearningStore_SearchByConditionAndScope(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "repo-error",
			Condition:   "error in production environment",
			Action:      "check logs",
			Outcome:     "fixed",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "global-error",
			Condition:   "error in staging environment",
			Action:      "check config",
			Outcome:     "resolved",
			Scope:       "global",
			OutcomeType: "success",
			CreatedAt:   now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Search in repo scope only
	results, err := store.SearchByConditionAndScope("error", []string{"repo"})
	if err != nil {
		t.Fatalf("SearchByConditionAndScope() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchByConditionAndScope() returned %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].Scope != "repo" {
		t.Errorf("SearchByConditionAndScope() returned scope %v, want repo", results[0].Scope)
	}
}

func TestLearningStore_SearchByPathAndScope(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:          "repo-path",
			Condition:   "editing internal/learning/store.go",
			Action:      "run tests",
			Outcome:     "pass",
			Scope:       "repo",
			OutcomeType: "success",
			CreatedAt:   now,
		},
		{
			ID:          "global-path",
			Condition:   "editing internal/learning/cao.go",
			Action:      "validate",
			Outcome:     "pass",
			Scope:       "global",
			OutcomeType: "success",
			CreatedAt:   now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Search in repo scope only
	results, err := store.SearchByPathAndScope("internal/learning", []string{"repo"})
	if err != nil {
		t.Fatalf("SearchByPathAndScope() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchByPathAndScope() returned %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].Scope != "repo" {
		t.Errorf("SearchByPathAndScope() returned scope %v, want repo", results[0].Scope)
	}

	// Search in both scopes
	results, err = store.SearchByPathAndScope("internal/learning", []string{"repo", "global"})
	if err != nil {
		t.Fatalf("SearchByPathAndScope() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Errorf("SearchByPathAndScope() returned %d results, want 2", len(results))
	}
}
