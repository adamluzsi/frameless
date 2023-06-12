package schedulecontracts

import (
	"context"
	"github.com/adamluzsi/frameless/internal/suites"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule"
	"github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/frameless/ports/locks/lockscontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
	"testing"
)

func Repository(mk func(testing.TB) RepositorySubject) suites.Suite {
	s := testcase.NewSpec(nil, testcase.AsSuite("schedule.Repository"))

	s.Context(".Locks", lockscontracts.Factory[schedule.StateID](func(tb testing.TB) lockscontracts.FactorySubject[schedule.StateID] {
		t := testcase.ToT(&tb)
		subject := mk(tb)
		return lockscontracts.FactorySubject[schedule.StateID]{
			Factory:     subject.Repository.Locks(),
			MakeContext: subject.MakeContext,
			MakeKey: func() schedule.StateID {
				return schedule.StateID(t.Random.String())
			},
		}
	}).Spec)

	s.Context(".States", stateRepository(func(tb testing.TB) stateRepositorySubject {
		t := testcase.ToT(&tb)
		subject := mk(tb)
		return stateRepositorySubject{
			StateRepository: subject.Repository.States(),
			MakeContext:     subject.MakeContext,
			MakeScheduleState: func() schedule.State {
				return schedule.State{
					ID:        schedule.StateID(t.Random.String() + t.Random.StringNC(5, random.CharsetDigit())),
					Timestamp: t.Random.Time(),
				}
			},
		}
	}).Spec)

	return s.AsSuite()
}

type RepositorySubject struct {
	Repository  schedule.Repository
	MakeContext func() context.Context
}

func stateRepository(mk func(tb testing.TB) stateRepositorySubject) suites.Suite {
	s := testcase.NewSpec(nil, testcase.AsSuite("schedule.StateRepository"))
	testcase.RunSuite(s,
		crudcontracts.Creator[schedule.State, schedule.StateID](func(tb testing.TB) crudcontracts.CreatorSubject[schedule.State, schedule.StateID] {
			sub := mk(tb)
			return crudcontracts.CreatorSubject[schedule.State, schedule.StateID]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,

				SupportIDReuse:  false,
				SupportRecreate: false,
			}
		}),
		crudcontracts.Updater[schedule.State, schedule.StateID](func(tb testing.TB) crudcontracts.UpdaterSubject[schedule.State, schedule.StateID] {
			sub := mk(tb)
			return crudcontracts.UpdaterSubject[schedule.State, schedule.StateID]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,
				ChangeEntity: func(ptr *schedule.State) {
					ptr.Timestamp = testcase.ToT(&tb).Random.Time()
				},
			}
		}),
		crudcontracts.ByIDFinder[schedule.State, schedule.StateID](func(tb testing.TB) crudcontracts.ByIDFinderSubject[schedule.State, schedule.StateID] {
			sub := mk(tb)
			return crudcontracts.ByIDFinderSubject[schedule.State, schedule.StateID]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,
			}
		}),
		crudcontracts.ByIDDeleter[schedule.State, schedule.StateID](func(tb testing.TB) crudcontracts.ByIDDeleterSubject[schedule.State, schedule.StateID] {
			sub := mk(tb)
			return crudcontracts.ByIDDeleterSubject[schedule.State, schedule.StateID]{
				Resource:    sub.StateRepository,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeScheduleState,
			}
		}),
	)
	return s.AsSuite()
}

type stateRepositorySubject struct {
	StateRepository   schedule.StateRepository
	MakeContext       func() context.Context
	MakeScheduleState func() schedule.State
}
