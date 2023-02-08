package schedulecontracts

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs/schedule"
	"github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

type StateRepository struct {
	MakeSubject       func(tb testing.TB) schedule.StateRepository
	MakeContext       func(tb testing.TB) context.Context
	MakeScheduleState func(tb testing.TB) schedule.State
}

func (c StateRepository) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[schedule.State, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[schedule.State, string] {
				return c.MakeSubject(tb)
			},
			MakeContext:     c.MakeContext,
			MakeEntity:      c.MakeScheduleState,
			SupportIDReuse:  true,
			SupportRecreate: true,
		},
		crudcontracts.Updater[schedule.State, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[schedule.State, string] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeScheduleState,
			ChangeEntity: func(tb testing.TB, ptr *schedule.State) {
				ptr.Timestamp = testcase.ToT(&tb).Random.Time()
			},
		},
		crudcontracts.ByIDFinder[schedule.State, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.ByIDFinderSubject[schedule.State, string] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeScheduleState,
		},
		crudcontracts.ByIDDeleter[schedule.State, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.ByIDDeleterSubject[schedule.State, string] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeScheduleState,
		},
	)
}

func (c StateRepository) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c StateRepository) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}
