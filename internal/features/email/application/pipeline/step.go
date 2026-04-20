package pipeline

import (
	"context"
)

// Step represents a single step in the email pipeline.
// Each step is isolated, testable, and composable.
type Step interface {
	// Execute performs the step's operation on the email context.
	// It should be idempotent and side-effect free where possible.
	Execute(ctx context.Context, emailCtx *EmailContext) error

	// Name returns the step's identifier for logging and metrics.
	Name() string
}

// Stage is an alias for Step (backward compatibility).
type Stage = Step

// StepFunc is a function adapter that implements Step interface.
// Useful for simple inline steps without creating full structs.
type StepFunc struct {
	name string
	fn   func(context.Context, *EmailContext) error
}

// NewStepFunc creates a new function-based step.
func NewStepFunc(name string, fn func(context.Context, *EmailContext) error) Step {
	return &StepFunc{name: name, fn: fn}
}

// Execute implements Step interface.
func (s *StepFunc) Execute(ctx context.Context, emailCtx *EmailContext) error {
	return s.fn(ctx, emailCtx)
}

// Name implements Step interface.
func (s *StepFunc) Name() string {
	return s.name
}

// CompositeStep combines multiple steps into one.
// Useful for grouping related steps while maintaining granularity.
type CompositeStep struct {
	name  string
	steps []Step
}

// NewCompositeStep creates a new composite step.
func NewCompositeStep(name string, steps ...Step) Step {
	return &CompositeStep{
		name:  name,
		steps: steps,
	}
}

// Execute runs all sub-steps in sequence.
func (c *CompositeStep) Execute(ctx context.Context, emailCtx *EmailContext) error {
	for _, step := range c.steps {
		if err := step.Execute(ctx, emailCtx); err != nil {
			return err
		}
	}
	return nil
}

// Name implements Step interface.
func (c *CompositeStep) Name() string {
	return c.name
}
