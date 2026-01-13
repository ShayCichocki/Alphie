package learning

import (
	"testing"
	"time"
)

func TestNewRetriever(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	retriever := NewRetriever(store)
	if retriever == nil {
		t.Fatal("NewRetriever() returned nil")
	}
}

func TestRetriever_RetrieveForTask_NilStore(t *testing.T) {
	retriever := NewRetriever(nil)

	results, err := retriever.RetrieveForTask("test task", nil)
	if err != nil {
		t.Fatalf("RetrieveForTask() error = %v, want nil", err)
	}
	if results != nil {
		t.Errorf("RetrieveForTask() = %v, want nil for nil store", results)
	}
}

func TestRetriever_RetrieveForTask_KeywordMatching(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:           "test-1",
			Condition:    "database migration fails",
			Action:       "check schema changes",
			Outcome:      "migration succeeds",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 5,
			CreatedAt:    now,
		},
		{
			ID:           "test-2",
			Condition:    "API authentication error",
			Action:       "refresh token",
			Outcome:      "authentication succeeds",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 3,
			CreatedAt:    now,
		},
		{
			ID:           "test-3",
			Condition:    "build compilation error",
			Action:       "fix syntax",
			Outcome:      "build succeeds",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 1,
			CreatedAt:    now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	// Search for database-related task
	results, err := retriever.RetrieveForTask("fix database migration issues", nil)
	if err != nil {
		t.Fatalf("RetrieveForTask() error = %v, want nil", err)
	}

	if len(results) == 0 {
		t.Fatal("RetrieveForTask() returned no results, expected at least 1")
	}

	// The database learning should be in results
	found := false
	for _, r := range results {
		if r.ID == "test-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("RetrieveForTask() did not return expected database learning")
	}
}

func TestRetriever_RetrieveForTask_FilePathMatching(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:           "test-1",
			Condition:    "editing internal/learning/store.go causes issues",
			Action:       "run tests first",
			Outcome:      "issues prevented",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 5,
			CreatedAt:    now,
		},
		{
			ID:           "test-2",
			Condition:    "editing cmd/main.go",
			Action:       "rebuild binary",
			Outcome:      "changes applied",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 3,
			CreatedAt:    now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	// Search with file path hints
	results, err := retriever.RetrieveForTask("update code", []string{"internal/learning/store.go"})
	if err != nil {
		t.Fatalf("RetrieveForTask() error = %v, want nil", err)
	}

	if len(results) == 0 {
		t.Fatal("RetrieveForTask() returned no results")
	}

	// The store.go learning should be in results
	found := false
	for _, r := range results {
		if r.ID == "test-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("RetrieveForTask() did not return expected learning for file path")
	}
}

func TestRetriever_RetrieveForTask_Ranking(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:            "low-trigger",
			Condition:     "database error",
			Action:        "check connection",
			Outcome:       "fixed",
			Scope:         "repo",
			OutcomeType:   "success",
			TriggerCount:  1,
			LastTriggered: now.Add(-30 * 24 * time.Hour), // 30 days ago
			CreatedAt:     now.Add(-30 * 24 * time.Hour),
		},
		{
			ID:            "high-trigger",
			Condition:     "database timeout",
			Action:        "increase timeout",
			Outcome:       "resolved",
			Scope:         "repo",
			OutcomeType:   "success",
			TriggerCount:  10,
			LastTriggered: now.Add(-1 * time.Hour), // 1 hour ago
			CreatedAt:     now.Add(-7 * 24 * time.Hour),
		},
		{
			ID:            "medium-trigger",
			Condition:     "database migration",
			Action:        "run migrations",
			Outcome:       "done",
			Scope:         "repo",
			OutcomeType:   "success",
			TriggerCount:  5,
			LastTriggered: now.Add(-7 * 24 * time.Hour), // 7 days ago
			CreatedAt:     now.Add(-14 * 24 * time.Hour),
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	results, err := retriever.RetrieveForTask("database issues", nil)
	if err != nil {
		t.Fatalf("RetrieveForTask() error = %v, want nil", err)
	}

	if len(results) < 2 {
		t.Fatalf("RetrieveForTask() returned %d results, want at least 2", len(results))
	}

	// High trigger count + recent should be ranked first
	if results[0].ID != "high-trigger" {
		t.Errorf("First result ID = %v, want high-trigger (highest score)", results[0].ID)
	}
}

func TestRetriever_RetrieveForTask_LimitsTo5(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	// Create more than 5 learnings with same keyword
	for i := 0; i < 10; i++ {
		learning := &Learning{
			ID:           "test-" + string(rune('a'+i)),
			Condition:    "error in component",
			Action:       "fix component",
			Outcome:      "component fixed",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: i,
			CreatedAt:    now.Add(time.Duration(i) * time.Second),
		}
		if err := store.Create(learning); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	results, err := retriever.RetrieveForTask("error component", nil)
	if err != nil {
		t.Fatalf("RetrieveForTask() error = %v, want nil", err)
	}

	if len(results) > 5 {
		t.Errorf("RetrieveForTask() returned %d results, want max 5", len(results))
	}
}

func TestRetriever_RetrieveForError(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:           "test-1",
			Condition:    "connection refused error",
			Action:       "check server status",
			Outcome:      "server restarted",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 5,
			CreatedAt:    now,
		},
		{
			ID:           "test-2",
			Condition:    "null pointer exception",
			Action:       "add nil check",
			Outcome:      "crash prevented",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 3,
			CreatedAt:    now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	results, err := retriever.RetrieveForError("connection refused")
	if err != nil {
		t.Fatalf("RetrieveForError() error = %v, want nil", err)
	}

	if len(results) == 0 {
		t.Fatal("RetrieveForError() returned no results")
	}

	// The connection refused learning should be in results
	found := false
	for _, r := range results {
		if r.ID == "test-1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("RetrieveForError() did not return expected learning")
	}
}

func TestRetriever_RetrieveForError_NilStore(t *testing.T) {
	retriever := NewRetriever(nil)

	results, err := retriever.RetrieveForError("some error")
	if err != nil {
		t.Fatalf("RetrieveForError() error = %v, want nil", err)
	}
	if results != nil {
		t.Errorf("RetrieveForError() = %v, want nil for nil store", results)
	}
}

func TestRetriever_extractKeywords(t *testing.T) {
	retriever := NewRetriever(nil)

	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "only stop words",
			input: "the a an and or but is are",
			want:  nil,
		},
		{
			name:  "mixed content",
			input: "fix the database connection error",
			want:  []string{"fix", "database", "connection", "error"},
		},
		{
			name:  "short words filtered",
			input: "a b c fix db error",
			want:  []string{"fix", "error"},
		},
		{
			name:  "duplicates removed",
			input: "error error database error",
			want:  []string{"error", "database"},
		},
		{
			name:  "case normalized",
			input: "Fix Database ERROR",
			want:  []string{"fix", "database", "error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := retriever.extractKeywords(tt.input)

			if len(got) != len(tt.want) {
				t.Errorf("extractKeywords() = %v, want %v", got, tt.want)
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractKeywords()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestExtractPathPrefix(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		want  string
	}{
		{
			name: "empty string",
			path: "",
			want: "",
		},
		{
			name: "file with extension",
			path: "internal/learning/store.go",
			want: "internal/learning",
		},
		{
			name: "directory path",
			path: "internal/learning",
			want: "internal/learning",
		},
		{
			name: "single file",
			path: "main.go",
			want: "main.go",
		},
		{
			name: "leading slash",
			path: "/internal/learning/store.go",
			want: "internal/learning",
		},
		{
			name: "deeply nested",
			path: "a/b/c/d/e/file.go",
			want: "a/b/c/d/e",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPathPrefix(tt.path)
			if got != tt.want {
				t.Errorf("extractPathPrefix(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestRetriever_calculateScore(t *testing.T) {
	retriever := NewRetriever(nil)
	now := time.Now()

	// Note: The new scoring formula is: (1 + BM25) * sqrt(1 + triggerCount) * recencyFactor
	// Without BM25 context set, BM25 = 0, so formula is: sqrt(1 + triggerCount) * recencyFactor
	tests := []struct {
		name     string
		learning *Learning
		wantGT   float64 // Score should be greater than this
	}{
		{
			name: "high trigger count, recent",
			learning: &Learning{
				TriggerCount:  10,
				LastTriggered: now.Add(-1 * time.Hour),
			},
			wantGT: 3.0, // sqrt(11) * ~1.0 recency = ~3.3
		},
		{
			name: "low trigger count, recent",
			learning: &Learning{
				TriggerCount:  1,
				LastTriggered: now.Add(-1 * time.Hour),
			},
			wantGT: 1.0, // sqrt(2) * ~1.0 recency = ~1.4
		},
		{
			name: "high trigger count, old",
			learning: &Learning{
				TriggerCount:  10,
				LastTriggered: now.Add(-30 * 24 * time.Hour),
			},
			wantGT: 0.5, // sqrt(11) * ~0.19 recency = ~0.63
		},
		{
			name: "zero trigger count",
			learning: &Learning{
				TriggerCount:  0,
				LastTriggered: now,
			},
			wantGT: 0.5, // sqrt(1) * 1.0 recency = 1.0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := retriever.calculateScore(tt.learning, now)
			if score <= tt.wantGT {
				t.Errorf("calculateScore() = %v, want > %v", score, tt.wantGT)
			}
		})
	}
}

func TestRetriever_rankLearnings(t *testing.T) {
	retriever := NewRetriever(nil)
	now := time.Now()

	learnings := []*Learning{
		{
			ID:            "low",
			TriggerCount:  1,
			LastTriggered: now.Add(-30 * 24 * time.Hour),
		},
		{
			ID:            "high",
			TriggerCount:  10,
			LastTriggered: now.Add(-1 * time.Hour),
		},
		{
			ID:            "medium",
			TriggerCount:  5,
			LastTriggered: now.Add(-7 * 24 * time.Hour),
		},
	}

	retriever.rankLearnings(learnings)

	// High should be first (highest score)
	if learnings[0].ID != "high" {
		t.Errorf("rankLearnings() first ID = %v, want high", learnings[0].ID)
	}

	// Low should be last (lowest score)
	if learnings[2].ID != "low" {
		t.Errorf("rankLearnings() last ID = %v, want low", learnings[2].ID)
	}
}

func TestBM25Score(t *testing.T) {
	// Create test learnings
	learnings := []*Learning{
		{
			ID:        "db-learning",
			Condition: "database connection timeout",
			Action:    "increase connection pool size",
			Outcome:   "database performance improved",
		},
		{
			ID:        "api-learning",
			Condition: "API response slow",
			Action:    "add caching layer",
			Outcome:   "API faster",
		},
		{
			ID:        "generic-learning",
			Condition: "error occurs",
			Action:    "check logs",
			Outcome:   "issue found",
		},
	}

	avgDocLen, docFreqs := computeCorpusStats(learnings)
	queryTerms := tokenize("database timeout")

	// Database learning should have highest BM25 score for "database timeout" query
	dbScore := bm25Score(learnings[0], queryTerms, avgDocLen, docFreqs, len(learnings))
	apiScore := bm25Score(learnings[1], queryTerms, avgDocLen, docFreqs, len(learnings))
	genericScore := bm25Score(learnings[2], queryTerms, avgDocLen, docFreqs, len(learnings))

	if dbScore <= apiScore {
		t.Errorf("BM25: database learning score (%v) should be > API learning score (%v)", dbScore, apiScore)
	}
	if dbScore <= genericScore {
		t.Errorf("BM25: database learning score (%v) should be > generic learning score (%v)", dbScore, genericScore)
	}
}

func TestBM25Score_EmptyInputs(t *testing.T) {
	learning := &Learning{
		Condition: "test condition",
		Action:    "test action",
		Outcome:   "test outcome",
	}

	// Empty query terms
	score := bm25Score(learning, []string{}, 10.0, map[string]int{"test": 1}, 1)
	if score != 0 {
		t.Errorf("BM25 with empty query terms = %v, want 0", score)
	}

	// Zero total docs
	score = bm25Score(learning, []string{"test"}, 10.0, map[string]int{"test": 1}, 0)
	if score != 0 {
		t.Errorf("BM25 with zero total docs = %v, want 0", score)
	}
}

func TestBM25Score_EmptyDocument(t *testing.T) {
	learning := &Learning{
		Condition: "",
		Action:    "",
		Outcome:   "",
	}

	score := bm25Score(learning, []string{"test"}, 10.0, map[string]int{"test": 1}, 1)
	if score != 0 {
		t.Errorf("BM25 with empty document = %v, want 0", score)
	}
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "simple words",
			input: "hello world",
			want:  []string{"hello", "world"},
		},
		{
			name:  "mixed case",
			input: "Hello World",
			want:  []string{"hello", "world"},
		},
		{
			name:  "with numbers",
			input: "error404 test123",
			want:  []string{"error404", "test123"},
		},
		{
			name:  "single char filtered",
			input: "a b c test",
			want:  []string{"test"},
		},
		{
			name:  "punctuation ignored",
			input: "hello, world! test.",
			want:  []string{"hello", "world", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("tokenize() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tokenize()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestComputeCorpusStats(t *testing.T) {
	learnings := []*Learning{
		{
			Condition: "database error",
			Action:    "check database",
			Outcome:   "fixed",
		},
		{
			Condition: "API error",
			Action:    "restart API",
			Outcome:   "fixed",
		},
	}

	avgDocLen, docFreqs := computeCorpusStats(learnings)

	// Both docs have "error" and "fixed", so df should be 2
	if docFreqs["error"] != 2 {
		t.Errorf("docFreqs[error] = %v, want 2", docFreqs["error"])
	}
	if docFreqs["fixed"] != 2 {
		t.Errorf("docFreqs[fixed] = %v, want 2", docFreqs["fixed"])
	}

	// "database" appears only in first doc
	if docFreqs["database"] != 1 {
		t.Errorf("docFreqs[database] = %v, want 1", docFreqs["database"])
	}

	// Average doc length should be > 0
	if avgDocLen <= 0 {
		t.Errorf("avgDocLen = %v, want > 0", avgDocLen)
	}
}

func TestRetriever_SemanticRanking(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	// Create learnings with different semantic relevance to "database migration"
	// All have similar trigger counts and recency so BM25 dominates
	learnings := []*Learning{
		{
			ID:            "exact-match",
			Condition:     "database migration fails",
			Action:        "run database migration scripts",
			Outcome:       "migration succeeds",
			Scope:         "repo",
			OutcomeType:   "success",
			TriggerCount:  2,
			LastTriggered: now.Add(-1 * time.Hour),
			CreatedAt:     now,
		},
		{
			ID:            "partial-match",
			Condition:     "database connection timeout",
			Action:        "increase timeout",
			Outcome:       "connection restored",
			Scope:         "repo",
			OutcomeType:   "success",
			TriggerCount:  2,
			LastTriggered: now.Add(-1 * time.Hour),
			CreatedAt:     now,
		},
		{
			ID:            "no-match",
			Condition:     "API authentication error",
			Action:        "refresh token",
			Outcome:       "auth fixed",
			Scope:         "repo",
			OutcomeType:   "success",
			TriggerCount:  2,
			LastTriggered: now.Add(-1 * time.Hour),
			CreatedAt:     now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	// Search for "database migration" - exact-match should rank highest due to BM25
	results, err := retriever.RetrieveForTask("database migration issue", nil)
	if err != nil {
		t.Fatalf("RetrieveForTask() error = %v", err)
	}

	if len(results) < 2 {
		t.Fatalf("Expected at least 2 results, got %d", len(results))
	}

	// The exact-match learning should be first because it has highest BM25 score
	// (contains both "database" and "migration" multiple times)
	if results[0].ID != "exact-match" {
		t.Errorf("Expected exact-match to be ranked first (BM25 relevance), got %s", results[0].ID)
	}

	// Partial match should be second (has "database" but not "migration")
	if results[1].ID != "partial-match" {
		t.Errorf("Expected partial-match to be ranked second, got %s", results[1].ID)
	}
}

func TestRetriever_RetrieveForTaskWithScope(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:           "repo-db",
			Condition:    "database error in repo context",
			Action:       "check repo config",
			Outcome:      "fixed in repo",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 5,
			CreatedAt:    now,
		},
		{
			ID:           "global-db",
			Condition:    "database error in global context",
			Action:       "check global settings",
			Outcome:      "fixed globally",
			Scope:        "global",
			OutcomeType:  "success",
			TriggerCount: 3,
			CreatedAt:    now,
		},
		{
			ID:           "module-db",
			Condition:    "database error in module",
			Action:       "check module deps",
			Outcome:      "fixed in module",
			Scope:        "module",
			OutcomeType:  "success",
			TriggerCount: 2,
			CreatedAt:    now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	// Test with repo scope only
	results, err := retriever.RetrieveForTaskWithScope("database error", nil, []string{"repo"})
	if err != nil {
		t.Fatalf("RetrieveForTaskWithScope() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Errorf("RetrieveForTaskWithScope([repo]) returned %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].Scope != "repo" {
		t.Errorf("RetrieveForTaskWithScope([repo]) returned scope %v, want repo", results[0].Scope)
	}

	// Test with multiple scopes
	results, err = retriever.RetrieveForTaskWithScope("database error", nil, []string{"repo", "global"})
	if err != nil {
		t.Fatalf("RetrieveForTaskWithScope() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Errorf("RetrieveForTaskWithScope([repo, global]) returned %d results, want 2", len(results))
	}
	for _, r := range results {
		if r.Scope != "repo" && r.Scope != "global" {
			t.Errorf("RetrieveForTaskWithScope([repo, global]) returned scope %v, want repo or global", r.Scope)
		}
	}

	// Test with nil scopes (should return all)
	results, err = retriever.RetrieveForTaskWithScope("database error", nil, nil)
	if err != nil {
		t.Fatalf("RetrieveForTaskWithScope() error = %v, want nil", err)
	}
	if len(results) != 3 {
		t.Errorf("RetrieveForTaskWithScope(nil scopes) returned %d results, want 3", len(results))
	}
}

func TestRetriever_RetrieveForTaskWithScope_FilePathFiltering(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:           "repo-path",
			Condition:    "editing internal/learning/store.go in repo",
			Action:       "run repo tests",
			Outcome:      "pass",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 3,
			CreatedAt:    now,
		},
		{
			ID:           "global-path",
			Condition:    "editing internal/learning/cao.go globally",
			Action:       "run global tests",
			Outcome:      "pass",
			Scope:        "global",
			OutcomeType:  "success",
			TriggerCount: 2,
			CreatedAt:    now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	// Test file path filtering with scope
	results, err := retriever.RetrieveForTaskWithScope("update code", []string{"internal/learning/store.go"}, []string{"repo"})
	if err != nil {
		t.Fatalf("RetrieveForTaskWithScope() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Errorf("RetrieveForTaskWithScope() returned %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].Scope != "repo" {
		t.Errorf("RetrieveForTaskWithScope() returned scope %v, want repo", results[0].Scope)
	}
}

func TestRetriever_RetrieveForErrorWithScope(t *testing.T) {
	store, cleanup := newTestStore(t)
	defer cleanup()

	now := time.Now().UTC()
	learnings := []*Learning{
		{
			ID:           "repo-conn",
			Condition:    "connection refused error in repo",
			Action:       "check repo server",
			Outcome:      "server restarted",
			Scope:        "repo",
			OutcomeType:  "success",
			TriggerCount: 5,
			CreatedAt:    now,
		},
		{
			ID:           "global-conn",
			Condition:    "connection refused error globally",
			Action:       "check global proxy",
			Outcome:      "proxy fixed",
			Scope:        "global",
			OutcomeType:  "success",
			TriggerCount: 3,
			CreatedAt:    now,
		},
	}

	for _, l := range learnings {
		if err := store.Create(l); err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	retriever := NewRetriever(store)

	// Test with repo scope only
	results, err := retriever.RetrieveForErrorWithScope("connection refused", []string{"repo"})
	if err != nil {
		t.Fatalf("RetrieveForErrorWithScope() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Errorf("RetrieveForErrorWithScope([repo]) returned %d results, want 1", len(results))
	}
	if len(results) > 0 && results[0].Scope != "repo" {
		t.Errorf("RetrieveForErrorWithScope([repo]) returned scope %v, want repo", results[0].Scope)
	}

	// Test with nil scopes (should return all)
	results, err = retriever.RetrieveForErrorWithScope("connection refused", nil)
	if err != nil {
		t.Fatalf("RetrieveForErrorWithScope() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Errorf("RetrieveForErrorWithScope(nil) returned %d results, want 2", len(results))
	}
}

func TestRetriever_RetrieveForErrorWithScope_NilStore(t *testing.T) {
	retriever := NewRetriever(nil)

	results, err := retriever.RetrieveForErrorWithScope("some error", []string{"repo"})
	if err != nil {
		t.Fatalf("RetrieveForErrorWithScope() error = %v, want nil", err)
	}
	if results != nil {
		t.Errorf("RetrieveForErrorWithScope() = %v, want nil for nil store", results)
	}
}

func TestRetriever_RetrieveForTaskWithScope_NilStore(t *testing.T) {
	retriever := NewRetriever(nil)

	results, err := retriever.RetrieveForTaskWithScope("test task", nil, []string{"repo"})
	if err != nil {
		t.Fatalf("RetrieveForTaskWithScope() error = %v, want nil", err)
	}
	if results != nil {
		t.Errorf("RetrieveForTaskWithScope() = %v, want nil for nil store", results)
	}
}
