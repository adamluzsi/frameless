package tasker

import (
	"context"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/testcase/clock"
)

type Scheduler struct {
	Locks           SchedulerLocks
	StateRepository SchedulerStateRepository
}

type SchedulerLocks interface {
	guard.LockerFactory[ScheduleStateID]
}

type SchedulerStateRepository interface {
	crud.Creator[ScheduleState]
	crud.Updater[ScheduleState]
	crud.ByIDDeleter[ScheduleStateID]
	crud.ByIDFinder[ScheduleState, ScheduleStateID]
}

func (s Scheduler) WithSchedule(id ScheduleStateID, interval Interval, job Task) Task {
	locker := s.Locks.LockerFor(id)

	next := func(ctx context.Context) (_ time.Duration, rErr error) {
		ctx, err := locker.Lock(ctx)
		if err != nil {
			return 0, err
		}
		defer func() { rErr = errorkit.Merge(rErr, locker.Unlock(ctx)) }()

		state, found, err := s.StateRepository.FindByID(ctx, id)
		if err != nil {
			return 0, err
		}
		if !found {
			state = ScheduleState{
				ID:        id,
				Timestamp: time.Time{}.UTC(),
			}
			if err := s.StateRepository.Create(ctx, &state); err != nil {
				return 0, err
			}
		}

		if nextAt := interval.UntilNext(state.Timestamp); 0 < nextAt {
			return nextAt, nil
		}
		if err := job(ctx); err != nil {
			return 0, err
		}

		state.Timestamp = clock.Now().UTC()
		return interval.UntilNext(state.Timestamp), s.StateRepository.Update(ctx, &state)
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

type ScheduleState struct {
	ID        ScheduleStateID `ext:"id"`
	Timestamp time.Time
}

type ScheduleStateID string
