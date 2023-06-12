package schedule

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/tasker"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/locks"
	"github.com/adamluzsi/testcase/clock"
	"time"
)

type Scheduler struct{ Repository Repository }

type Repository interface {
	Locks() locks.Factory[StateID]
	States() StateRepository
}

type StateRepository interface {
	crud.Creator[State]
	crud.Updater[State]
	crud.ByIDDeleter[StateID]
	crud.ByIDFinder[State, StateID]
}

func (s Scheduler) WithSchedule(id StateID, interval tasker.Interval, job tasker.Task) tasker.Task {
	locker := s.Repository.Locks().LockerFor(id)

	next := func(ctx context.Context) (_ time.Duration, rErr error) {
		ctx, err := locker.Lock(ctx)
		if err != nil {
			return 0, err
		}
		defer func() { rErr = errorkit.Merge(rErr, locker.Unlock(ctx)) }()

		state, found, err := s.Repository.States().FindByID(ctx, id)
		if err != nil {
			return 0, err
		}
		if !found {
			state = State{
				ID:        id,
				Timestamp: time.Time{}.UTC(),
			}
			if err := s.Repository.States().Create(ctx, &state); err != nil {
				return 0, err
			}
		}

		if nextAt := interval.UntilNext(state.Timestamp); 0 < nextAt {
			return nextAt, nil
		}
		if err := job(ctx); err != nil {
			return 0, err
		}

		state.Timestamp = clock.TimeNow().UTC()
		return interval.UntilNext(state.Timestamp), s.Repository.States().Update(ctx, &state)
	}

	return func(ctx context.Context) error {
	wrk:
		for {
			untilNext, err := next(ctx)
			if err != nil {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-clock.After(untilNext):
				continue wrk
			}
		}
	}
}

type State struct {
	ID        StateID `ext:"id"`
	Timestamp time.Time
}

type StateID string
