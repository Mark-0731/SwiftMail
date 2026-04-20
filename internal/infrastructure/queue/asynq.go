package queue

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
)

// AsynqQueue implements the Queue interface using Asynq.
type AsynqQueue struct {
	client *asynq.Client
	logger zerolog.Logger
}

// NewAsynqQueue creates a new Asynq queue implementation.
func NewAsynqQueue(client *asynq.Client, logger zerolog.Logger) Queue {
	return &AsynqQueue{
		client: client,
		logger: logger,
	}
}

// Enqueue adds a task to the queue with default options.
func (q *AsynqQueue) Enqueue(ctx context.Context, task *Task) error {
	return q.EnqueueWithOptions(ctx, task, &EnqueueOptions{
		Queue:    "default",
		MaxRetry: 3,
	})
}

// EnqueueWithOptions adds a task with custom options.
func (q *AsynqQueue) EnqueueWithOptions(ctx context.Context, task *Task, opts *EnqueueOptions) error {
	asynqTask := asynq.NewTask(task.Type, task.Payload)

	// Build Asynq options
	asynqOpts := []asynq.Option{
		asynq.Queue(opts.Queue),
		asynq.MaxRetry(opts.MaxRetry),
	}

	// Add task ID if provided (for idempotency)
	if opts.TaskID != "" {
		asynqOpts = append(asynqOpts, asynq.TaskID(opts.TaskID))
	}

	// Add process time if provided (for scheduled tasks)
	if opts.ProcessAt != nil {
		asynqOpts = append(asynqOpts, asynq.ProcessAt(*opts.ProcessAt))
	}

	// Add timeout if provided
	if opts.Timeout > 0 {
		asynqOpts = append(asynqOpts, asynq.Timeout(opts.Timeout))
	}

	// Enqueue the task
	info, err := q.client.EnqueueContext(ctx, asynqTask, asynqOpts...)
	if err != nil {
		q.logger.Error().
			Err(err).
			Str("task_type", task.Type).
			Str("queue", opts.Queue).
			Msg("failed to enqueue task")
		return err
	}

	q.logger.Debug().
		Str("task_id", info.ID).
		Str("task_type", task.Type).
		Str("queue", info.Queue).
		Msg("task enqueued successfully")

	return nil
}

// Close closes the Asynq client.
func (q *AsynqQueue) Close() error {
	return q.client.Close()
}
