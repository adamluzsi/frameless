package schedulecontracts

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule"
	"github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

type StateRepository func(tb testing.TB) StateRepositorySubject

type StateRepositorySubject struct {
	StateRepository   schedule.StateRepository
	MakeContext       func() context.Context
	MakeScheduleState func() schedule.State
}

func (c StateRepository) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[schedule.State, string](func(tb testing.TB) crudcontracts.CreatorSubject[schedule.State, string] {
			sub := c(tb)
			return crudcontracts.CreatorSubject[schedule.State, string]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,

				SupportIDReuse:  false,
				SupportRecreate: false,
			}
		}),
		crudcontracts.Updater[schedule.State, string](func(tb testing.TB) crudcontracts.UpdaterSubject[schedule.State, string] {
			sub := c(tb)
			return crudcontracts.UpdaterSubject[schedule.State, string]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,
				ChangeEntity: func(ptr *schedule.State) {
					ptr.Timestamp = testcase.ToT(&tb).Random.Time()
				},
			}
		}),
		crudcontracts.ByIDFinder[schedule.State, string](func(tb testing.TB) crudcontracts.ByIDFinderSubject[schedule.State, string] {
			sub := c(tb)
			return crudcontracts.ByIDFinderSubject[schedule.State, string]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,
			}
		}),
		crudcontracts.ByIDDeleter[schedule.State, string](func(tb testing.TB) crudcontracts.ByIDDeleterSubject[schedule.State, string] {
			sub := c(tb)
			return crudcontracts.ByIDDeleterSubject[schedule.State, string]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,
			}
		}),
	)
}

func (c StateRepository) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c StateRepository) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}
