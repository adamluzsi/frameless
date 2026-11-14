package fileio

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/resilience"
)

// retryStrategy paces the attempts of a file operation which fails with a transient error.
// As such a file is usually released within milliseconds, the first retry waits 2ms,
// then each wait doubles, so the retries don't keep competing for the file with the process which holds it,
// and after the 10th attempt, about a second later, it gives up.
// This is close to how the Go toolchain's robustio package retries the same Windows errors.
var retryStrategy resilience.RetryStrategy = resilience.ExponentialBackoff{
	Delay:    time.Millisecond,
	Attempts: 10,
}

// Retry runs op until it doesn't fail with a transient error, see isTransient.
//
// On Windows, a file which is open without sharing its deletion can't be renamed, replaced or removed,
// and as virus scanners and search indexers open the new files for a moment, such a failure goes away by itself.
// On the other platforms, no error is transient, so op runs once.
func Retry(op func() error) error {
	return retryWhile(isTransient, retryStrategy, op)
}

// retryWhile runs op, and while it fails with a transient error, runs it again as long as the strategy allows it.
// When the strategy gives up, the error of the last attempt is returned.
//
// op always runs at least once, whatever the strategy says,
// and as the file operations are short and can't be stopped midway, the retries are bounded by the strategy, not by a context.
func retryWhile(isTransient func(error) bool, rs resilience.RetryStrategy, op func() error) error {
	err := op()
	attempt := resilience.MakeRetryAttempt()
	for err != nil && isTransient(err) {
		attempt.Next()
		if !rs.ShouldTry(context.Background(), attempt) {
			break
		}
		err = op()
	}
	return err
}
