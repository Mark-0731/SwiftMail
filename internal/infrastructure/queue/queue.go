package queue

import (
	"context"
	"time"
)

// Queue defines the task queue interface for the application.
// This abstraction allows us to swap Asynq with other queue providers.
type Queue interface {
	// Enqueue adds a task to the queue.
	Enqueue(ctx context.Context, task *Task) error

	// EnqueueWithOptions adds a task with custom options.
	EnqueueWithOptions(ctx context.Context, task *Task, opts *EnqueueOptions) error

	// Close closes the queue client.
	Close() error
}

// Task represents a task to be queued.
type Task struct {
	Type    string
	Payload []byte
}

// EnqueueOptions contains options for enqueueing tasks.
type EnqueueOptions struct {
	// Queue name (e.g., "high", "default", "low")
	Queue string

	// MaxRetry is the maximum number of retry attempts
	MaxRetry int

	// TaskID is a unique identifier for the task (for idempotency)
	TaskID string

	// ProcessAt schedules the task to be processed at a specific time
	ProcessAt *time.Time

	// Timeout is the maximum duration for task processing
	Timeout time.Duration
}
