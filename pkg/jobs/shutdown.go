package jobs

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"os"
	"sync"
	"syscall"
)

func Run(ctx context.Context, jobs ...Job) error {
	return ShutdownManager{
		Jobs:    jobs,
		Signals: defaultShutdownSignals(),
	}.Run(ctx)
}

// ShutdownManager helps to manage concurrent background Jobs in your main.
// Each Job will run in its own goroutine.
// If any of the Job encounters a failure, the other jobs will receive a cancellation signal.
type ShutdownManager struct {
	// Jobs is the list of background job that you wish to run concurrently.
	// They will act as a unit, if any of them fail with an error,
	// other jobs will be notified to shut down.
	Jobs []Job
	// Signals is an [OPTIONAL] parameter
	// where you can define what signals you want to consider as a shutdown signal.
	// If not defined, then the default signal values are INT, HUB and TERM.
	Signals []os.Signal
}

func (m ShutdownManager) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan os.Signal)
	defer close(ch)

	internal.SignalNotify(ch, m.signals()...)
	defer internal.SignalStop(ch)

	go func() {
		for range ch {
			cancel()
		}
	}()

	return m.runJobs(ctx)
}

func (m ShutdownManager) signals() []os.Signal {
	if 0 < len(m.Signals) {
		return m.Signals
	}
	return defaultShutdownSignals()
}

func defaultShutdownSignals() []os.Signal {
	return []os.Signal{
		syscall.SIGINT,
		syscall.SIGHUP,
		syscall.SIGTERM,
	}
}

func (m ShutdownManager) runJobs(ctx context.Context) error {
	var (
		wwg, cwg sync.WaitGroup
		errs     []error
		errCh    = make(chan error, len(m.Jobs))
	)
	ctx, cancelDueToError := context.WithCancel(ctx)
	defer cancelDueToError()

	wwg.Add(len(m.Jobs))
	for _, job := range m.Jobs {
		go func(j Job) {
			defer wwg.Done()
			errCh <- j(ctx)
		}(job)
	}

	cwg.Add(1)
	go func() {
		defer cwg.Done()
		for err := range errCh {
			if err == nil { // shutdown with no error is OK
				continue
			}

			cancelDueToError() // if one fails, all will shut down

			if errors.Is(err, context.Canceled) { // we don't report back context cancellation error
				continue
			}

			errs = append(errs, err)
		}
	}()

	wwg.Wait()
	close(errCh)
	cwg.Wait()
	return errorutil.Merge(errs...)
}
