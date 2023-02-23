package schedule

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/tasker"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/locks"
	"github.com/adamluzsi/testcase/clock"
	"time"
)

type Scheduler struct {
	LockerFactory locks.Factory[ /*name*/ string]
	Repository    StateRepository
}

func (s Scheduler) WithSchedule(jobid string, interval tasker.Interval, job tasker.Task) tasker.Task {
	locker := s.LockerFactory.LockerFor(jobid)

	next := func(ctx context.Context) (_ time.Duration, rErr error) {
		ctx, err := locker.Lock(ctx)
		if err != nil {
			return 0, err
		}
		defer func() { rErr = errorutil.Merge(rErr, locker.Unlock(ctx)) }()

		state, found, err := s.Repository.FindByID(ctx, jobid)
		if err != nil {
			return 0, err
		}
		if !found {
			state = State{
				ID:        jobid,
				Timestamp: time.Time{}.UTC(),
			}
			if err := s.Repository.Create(ctx, &state); err != nil {
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
		return interval.UntilNext(state.Timestamp), s.Repository.Update(ctx, &state)
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
	ID        string `ext:"id"`
	Timestamp time.Time
}

type StateRepository interface {
	crud.Creator[State]
	crud.Updater[State]
	crud.ByIDDeleter[string]
	crud.ByIDFinder[State, string]
}
