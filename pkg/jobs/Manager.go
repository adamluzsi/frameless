package jobs

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs/internal"
	"os"
	"syscall"
)

func Run(ctx context.Context, jobs ...Job) error {
	return Manager{Jobs: jobs}.Run(ctx)
}

// Manager helps to manage concurrent background Jobs in your main.
// Each Job will run in its own goroutine.
// If any of the Job encounters a failure, the other jobs will receive a cancellation signal.
type Manager struct {
	// Jobs is the list of background job that you wish to run concurrently.
	// They will act as a unit, if any of them fail with an error,
	// other jobs will be notified to shut down.
	Jobs []Job
	// Signals is an [OPTIONAL] parameter
	// where you can define what signals you want to consider as a shutdown signal.
	// If not defined, then the default signal values are INT, HUB and TERM.
	Signals []os.Signal
}

func (m Manager) Run(ctx context.Context) error {
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

	return append(Concurrence{}, m.Jobs...).Run(ctx)
}

func (m Manager) signals() []os.Signal {
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
