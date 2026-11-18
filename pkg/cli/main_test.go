package cli_test

import (
	"context"
	"os"
	"runtime"
	"slices"
	"testing"
	"testing/synctest"

	"go.llib.dev/frameless/internal/sandbox"
	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/internal/signalint"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestMain_earlyInterrupt(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Before(func(t *testcase.T) {
		logger.Stub(t)
		osint.StubArgs(t, []string{"cmd"})
		osint.StubExit(t, func(int) { runtime.Goexit() })
		t.Cleanup(signalint.StubStop(func(chan<- os.Signal) {}))
	})

	act := func(t *testcase.T) error {
		var handlerErr error
		synctest.Test(t.TB.(*testing.T), func(t *testing.T) {
			h := cli.HandlerFunc(func(_ cli.ResponseWriter, r *cli.Request) {
				// Let the signal receiver run before checking, without waiting forever for a lost signal.
				synctest.Wait()
				handlerErr = r.Context().Err()
			})
			exit := sandbox.Run(func() { cli.Main(t.Context(), h) })
			assert.Equal(t, exit, sandbox.O{Goexit: true})
		})
		return handlerErr
	}

	s.When("an interrupt arrives during signal registration", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.Cleanup(signalint.StubNotify(func(c chan<- os.Signal, signals ...os.Signal) {
				if len(signals) != 0 && !slices.Contains(signals, os.Interrupt) {
					return
				}
				select {
				case c <- os.Interrupt:
				default:
				}
			}))
		})

		s.Then("it cancels the handler's request context", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), context.Canceled)
		})
	})
}
