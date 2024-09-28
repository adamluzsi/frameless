package taskercontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/guard/guardcontracts"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func Repository(subject tasker.SchedulerRepository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	s.Context(".Locks", guardcontracts.LockerFactory[tasker.ScheduleStateID](subject.Locks(),
		guardcontracts.LockerFactoryConfig[tasker.ScheduleStateID]{MakeContext: c.MakeContext}).Spec)

	s.Context(".States", stateRepository(subject.States(), opts...).Spec)

	return s.AsSuite("tasker.Repository")
}

func stateRepository(subject tasker.ScheduleStateRepository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	crudConfig := crudcontracts.Config[tasker.ScheduleState, tasker.ScheduleStateID]{
		SupportIDReuse:  false,
		SupportRecreate: false,
		MakeContext:     c.MakeContext,
		ChangeEntity: func(tb testing.TB, ptr *tasker.ScheduleState) {
			ptr.Timestamp = testcase.ToT(&tb).Random.Time()
		},
		MakeEntity: c.MakeScheduleState,
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[tasker.ScheduleState, tasker.ScheduleStateID](subject, crudConfig),
		crudcontracts.Updater[tasker.ScheduleState, tasker.ScheduleStateID](subject, crudConfig),
		crudcontracts.ByIDFinder[tasker.ScheduleState, tasker.ScheduleStateID](subject, crudConfig),
		crudcontracts.ByIDDeleter[tasker.ScheduleState, tasker.ScheduleStateID](subject, crudConfig),
	)
	return s.AsSuite("tasker.ScheduleStateRepository")
}

type stateRepositorySubject struct {
	StateRepository   tasker.ScheduleStateRepository
	MakeContext       func() context.Context
	MakeScheduleState func() tasker.ScheduleState
}

type Option interface {
	option.Option[Config]
}

type Config struct {
	MakeContext       func(testing.TB) context.Context
	MakeScheduleState func(testing.TB) tasker.ScheduleState
}

func (c *Config) Init() {
	c.MakeContext = func(t testing.TB) context.Context {
		return context.Background()
	}
	c.MakeScheduleState = func(tb testing.TB) tasker.ScheduleState {
		t := testcase.ToT(&tb)
		return tasker.ScheduleState{
			ID:        tasker.ScheduleStateID(t.Random.String() + t.Random.StringNC(5, random.CharsetDigit())),
			Timestamp: t.Random.Time(),
		}
	}
}
