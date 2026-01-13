// Package architect provides tools for analyzing and auditing codebases against specifications.
package architect

import "sync"

// Question represents a question from a blocked worker.
type Question struct {
	// TaskID is the ID of the task that generated the question.
	TaskID string
	// Question is the question text.
	Question string
	// Context provides additional context for the question.
	Context string
}

// QuestionBatch contains a collection of questions for batch presentation.
type QuestionBatch struct {
	// Questions is the list of questions to present.
	Questions []Question
}

// QuestionQueue collects questions from blocked workers during epic execution.
type QuestionQueue struct {
	mu        sync.Mutex
	questions []Question
}

// NewQuestionQueue creates a new empty question queue.
func NewQuestionQueue() *QuestionQueue {
	return &QuestionQueue{
		questions: make([]Question, 0),
	}
}

// Add adds a question from a blocked worker to the queue.
func (q *QuestionQueue) Add(taskID, question, context string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.questions = append(q.questions, Question{
		TaskID:   taskID,
		Question: question,
		Context:  context,
	})
}

// GetBatch returns all queued questions as a batch.
// Returns nil if no questions are queued.
func (q *QuestionQueue) GetBatch() *QuestionBatch {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.questions) == 0 {
		return nil
	}

	// Copy questions to avoid race conditions
	batch := &QuestionBatch{
		Questions: make([]Question, len(q.questions)),
	}
	copy(batch.Questions, q.questions)

	return batch
}

// Clear removes all questions from the queue.
func (q *QuestionQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.questions = q.questions[:0]
}

// Len returns the number of questions in the queue.
func (q *QuestionQueue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return len(q.questions)
}
