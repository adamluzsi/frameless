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
	Locks  SchedulerLocks
	States ScheduleStateRepository
}

type SchedulerLocks interface {
	guard.LockerFactory[ScheduleID]
	guard.NonBlockingLockerFactory[ScheduleID]
}

type ScheduleStateRepository interface {
	crud.Creator[ScheduleState]
	crud.Updater[ScheduleState]
	crud.ByIDDeleter[ScheduleID]
	crud.ByIDFinder[ScheduleState, ScheduleID]
}

func (s Scheduler) WithSchedule(id ScheduleID, interval Interval, job Task) Task {
	if job == nil {
		return nil
	}

	if s.Locks == nil {
		panic("")
	}

	lock := s.Locks.LockerFor(id)

	next := func(ctx context.Context) (_ time.Duration, rErr error) {
		ctx, err := lock.Lock(ctx)
		if err != nil {
			return 0, err
		}
		defer errorkit.Finish(&rErr, func() error { return lock.Unlock(ctx) })

		state, found, err := s.States.FindByID(ctx, id)
		if err != nil {
			return 0, err
		}
		if !found {
			if err := job(ctx); err != nil {
				return 0, err
			}
			state = ScheduleState{
				ID:        id,
				Timestamp: clock.Now().UTC(),
			}
			if err := s.States.Create(ctx, &state); err != nil {
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
		return interval.UntilNext(state.Timestamp), s.States.Update(ctx, &state)
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

func WithNoOverlap(lock guard.NonBlockingLocker, job Task) Task {
	if job == nil {
		return nil
	}
	return func(ctx context.Context) (rErr error) {
		ctx, isLocked, err := lock.TryLock(ctx)
		if err != nil {
			return err
		}
		if !isLocked {
			return nil
		}
		defer errorkit.Finish(&rErr, func() error {
			return lock.Unlock(ctx)
		})
		return job.Run(ctx)
	}
}

type ScheduleState struct {
	ID        ScheduleID `ext:"id"`
	Timestamp time.Time
}

type ScheduleID string
