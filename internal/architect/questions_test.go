package architect

import (
	"sync"
	"testing"
)

func TestQuestionQueue_Add(t *testing.T) {
	q := NewQuestionQueue()

	q.Add("task-1", "What is the API endpoint?", "Working on authentication")
	q.Add("task-2", "Where is the config file?", "Setting up database")

	if q.Len() != 2 {
		t.Errorf("expected 2 questions, got %d", q.Len())
	}
}

func TestQuestionQueue_GetBatch(t *testing.T) {
	q := NewQuestionQueue()

	// Empty queue returns nil
	batch := q.GetBatch()
	if batch != nil {
		t.Error("expected nil batch for empty queue")
	}

	// Add questions
	q.Add("task-1", "Question 1", "Context 1")
	q.Add("task-2", "Question 2", "Context 2")

	batch = q.GetBatch()
	if batch == nil {
		t.Fatal("expected non-nil batch")
	}

	if len(batch.Questions) != 2 {
		t.Errorf("expected 2 questions in batch, got %d", len(batch.Questions))
	}

	// Verify question contents
	if batch.Questions[0].TaskID != "task-1" {
		t.Errorf("expected task-1, got %s", batch.Questions[0].TaskID)
	}
	if batch.Questions[0].Question != "Question 1" {
		t.Errorf("expected 'Question 1', got %s", batch.Questions[0].Question)
	}
	if batch.Questions[0].Context != "Context 1" {
		t.Errorf("expected 'Context 1', got %s", batch.Questions[0].Context)
	}
}

func TestQuestionQueue_Clear(t *testing.T) {
	q := NewQuestionQueue()

	q.Add("task-1", "Question", "Context")
	if q.Len() != 1 {
		t.Errorf("expected 1 question, got %d", q.Len())
	}

	q.Clear()
	if q.Len() != 0 {
		t.Errorf("expected 0 questions after clear, got %d", q.Len())
	}

	// GetBatch should return nil after clear
	batch := q.GetBatch()
	if batch != nil {
		t.Error("expected nil batch after clear")
	}
}

func TestQuestionQueue_Concurrent(t *testing.T) {
	q := NewQuestionQueue()
	var wg sync.WaitGroup

	// Add questions concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			q.Add("task", "question", "context")
		}(i)
	}

	wg.Wait()

	if q.Len() != 10 {
		t.Errorf("expected 10 questions after concurrent adds, got %d", q.Len())
	}
}

func TestQuestionQueue_GetBatchDoesNotModify(t *testing.T) {
	q := NewQuestionQueue()
	q.Add("task-1", "Question", "Context")

	batch1 := q.GetBatch()
	batch2 := q.GetBatch()

	// Both batches should have the same content
	if len(batch1.Questions) != len(batch2.Questions) {
		t.Error("GetBatch should not modify queue")
	}

	// Modifying batch should not affect queue
	batch1.Questions[0].TaskID = "modified"
	batch3 := q.GetBatch()
	if batch3.Questions[0].TaskID == "modified" {
		t.Error("batch should be a copy, not reference")
	}
}
