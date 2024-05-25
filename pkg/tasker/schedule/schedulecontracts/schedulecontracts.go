package schedulecontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/tasker/schedule"
	"go.llib.dev/frameless/ports/contract"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/guard/guardcontracts"
	"go.llib.dev/frameless/ports/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func Repository(subject schedule.Repository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	s.Context(".Locks", guardcontracts.LockerFactory[schedule.StateID](subject.Locks(),
		guardcontracts.LockerFactoryConfig[schedule.StateID]{MakeContext: c.MakeContext}).Spec)

	s.Context(".States", stateRepository(subject.States(), opts...).Spec)

	return s.AsSuite("schedule.Repository")
}

func stateRepository(subject schedule.StateRepository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	crudConfig := crudcontracts.Config[schedule.State, schedule.StateID]{
		SupportIDReuse:  false,
		SupportRecreate: false,
		MakeContext:     c.MakeContext,
		ChangeEntity: func(tb testing.TB, ptr *schedule.State) {
			ptr.Timestamp = testcase.ToT(&tb).Random.Time()
		},
		MakeEntity: c.MakeScheduleState,
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[schedule.State, schedule.StateID](subject, crudConfig),
		crudcontracts.Updater[schedule.State, schedule.StateID](subject, crudConfig),
		crudcontracts.ByIDFinder[schedule.State, schedule.StateID](subject, crudConfig),
		crudcontracts.ByIDDeleter[schedule.State, schedule.StateID](subject, crudConfig),
	)
	return s.AsSuite("schedule.StateRepository")
}

type stateRepositorySubject struct {
	StateRepository   schedule.StateRepository
	MakeContext       func() context.Context
	MakeScheduleState func() schedule.State
}

type Option interface {
	option.Option[Config]
}

type Config struct {
	MakeContext       func() context.Context
	MakeScheduleState func(testing.TB) schedule.State
}

func (c *Config) Init() {
	c.MakeContext = context.Background
	c.MakeScheduleState = func(tb testing.TB) schedule.State {
		t := testcase.ToT(&tb)
		return schedule.State{
			ID:        schedule.StateID(t.Random.String() + t.Random.StringNC(5, random.CharsetDigit())),
			Timestamp: t.Random.Time(),
		}
	}
}
