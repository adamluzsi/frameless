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

func SchedulerLocks(subject tasker.SchedulerLocks, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	lconf := guardcontracts.LockerFactoryConfig[tasker.ScheduleID]{
		LockerConfig: guardcontracts.LockerConfig{
			MakeContext: c.MakeContext,
		},
		MakeKey: func(tb testing.TB) tasker.ScheduleID {
			tc := tb.(*testcase.T)
			rndStr := tc.Random.String()
			return tasker.ScheduleID(rndStr)
			// should accept any kind of string as ScheduleStateID
		},
	}

	guardcontracts.LockerFactory[tasker.ScheduleID](subject, lconf).Spec(s)

	return s.AsSuite("tasker.SchedulerLocks")
}

func ScheduleStateRepository(subject tasker.ScheduleStateRepository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config](opts)

	crudConfig := crudcontracts.Config[tasker.ScheduleState, tasker.ScheduleID]{
		SupportIDReuse:  false,
		SupportRecreate: false,
		MakeContext:     c.MakeContext,
		ChangeEntity: func(tb testing.TB, ptr *tasker.ScheduleState) {
			ptr.Timestamp = testcase.ToT(&tb).Random.Time()
		},
		MakeEntity: c.MakeScheduleState,
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[tasker.ScheduleState, tasker.ScheduleID](subject, crudConfig),
		crudcontracts.Updater[tasker.ScheduleState, tasker.ScheduleID](subject, crudConfig),
		crudcontracts.ByIDFinder[tasker.ScheduleState, tasker.ScheduleID](subject, crudConfig),
		crudcontracts.ByIDDeleter[tasker.ScheduleState, tasker.ScheduleID](subject, crudConfig),
	)

	return s.AsSuite("tasker.SchedulerStateRepository")
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
			ID:        tasker.ScheduleID(t.Random.String() + t.Random.StringNC(5, random.CharsetDigit())),
			Timestamp: t.Random.Time(),
		}
	}
}
