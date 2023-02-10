package jobs

import (
	"context"
)

// Run helps to manage concurrent background Jobs in your main.
// Each Job will run in its own goroutine.
// If any of the Job encounters a failure, the other jobs will receive a cancellation signal.
func Run(ctx context.Context, jobs ...Job) error {
	job := append(Concurrence{}, jobs...).Run
	return WithSignalNotify(job)(ctx)
}
