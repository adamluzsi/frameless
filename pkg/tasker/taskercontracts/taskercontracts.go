package taskercontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/ports/contract"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/guard/guardcontracts"
	"go.llib.dev/frameless/ports/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func Repository(subject tasker.Repository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	s.Context(".Locks", guardcontracts.LockerFactory[tasker.StateID](subject.Locks(),
		guardcontracts.LockerFactoryConfig[tasker.StateID]{MakeContext: c.MakeContext}).Spec)

	s.Context(".States", stateRepository(subject.States(), opts...).Spec)

	return s.AsSuite("tasker.Repository")
}

func stateRepository(subject tasker.StateRepository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	crudConfig := crudcontracts.Config[tasker.State, tasker.StateID]{
		SupportIDReuse:  false,
		SupportRecreate: false,
		MakeContext:     c.MakeContext,
		ChangeEntity: func(tb testing.TB, ptr *tasker.State) {
			ptr.Timestamp = testcase.ToT(&tb).Random.Time()
		},
		MakeEntity: c.MakeScheduleState,
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[tasker.State, tasker.StateID](subject, crudConfig),
		crudcontracts.Updater[tasker.State, tasker.StateID](subject, crudConfig),
		crudcontracts.ByIDFinder[tasker.State, tasker.StateID](subject, crudConfig),
		crudcontracts.ByIDDeleter[tasker.State, tasker.StateID](subject, crudConfig),
	)
	return s.AsSuite("tasker.StateRepository")
}

type stateRepositorySubject struct {
	StateRepository   tasker.StateRepository
	MakeContext       func() context.Context
	MakeScheduleState func() tasker.State
}

type Option interface {
	option.Option[Config]
}

type Config struct {
	MakeContext       func() context.Context
	MakeScheduleState func(testing.TB) tasker.State
}

func (c *Config) Init() {
	c.MakeContext = context.Background
	c.MakeScheduleState = func(tb testing.TB) tasker.State {
		t := testcase.ToT(&tb)
		return tasker.State{
			ID:        tasker.StateID(t.Random.String() + t.Random.StringNC(5, random.CharsetDigit())),
			Timestamp: t.Random.Time(),
		}
	}
}
