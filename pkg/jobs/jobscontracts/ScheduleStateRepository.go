package jobscontracts

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/jobs"
	"github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

type ScheduleStateRepository struct {
	MakeSubject       func(tb testing.TB) jobs.ScheduleStateRepository
	MakeContext       func(tb testing.TB) context.Context
	MakeScheduleState func(tb testing.TB) jobs.ScheduleState
}

func (c ScheduleStateRepository) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[jobs.ScheduleState, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[jobs.ScheduleState, string] {
				return c.MakeSubject(tb)
			},
			MakeContext:     c.MakeContext,
			MakeEntity:      c.MakeScheduleState,
			SupportIDReuse:  true,
			SupportRecreate: true,
		},
		crudcontracts.Updater[jobs.ScheduleState, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[jobs.ScheduleState, string] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeScheduleState,
			ChangeEntity: func(tb testing.TB, ptr *jobs.ScheduleState) {
				ptr.Timestamp = testcase.ToT(&tb).Random.Time()
			},
		},
		crudcontracts.ByIDFinder[jobs.ScheduleState, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.ByIDFinderSubject[jobs.ScheduleState, string] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeScheduleState,
		},
		crudcontracts.ByIDDeleter[jobs.ScheduleState, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.ByIDDeleterSubject[jobs.ScheduleState, string] {
				return c.MakeSubject(tb)
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeScheduleState,
		},
	)
}

func (c ScheduleStateRepository) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c ScheduleStateRepository) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}
