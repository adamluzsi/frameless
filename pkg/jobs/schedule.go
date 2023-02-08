package jobs

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/locks"
	"github.com/adamluzsi/testcase/clock"
	"time"
)

type Scheduler struct {
	LockerFactory locks.Factory[ /*name*/ string]
	Repository    ScheduleStateRepository
}

func (s Scheduler) WithSchedule(jobid string, interval time.Duration, job Job) Job {
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
			state = ScheduleState{
				JobID:     jobid,
				Timestamp: time.Time{}.UTC(),
			}
			if err := s.Repository.Create(ctx, &state); err != nil {
				return 0, err
			}
		}
		var (
			now    = clock.TimeNow().UTC()
			nextAt = state.Timestamp.Add(interval)
		)
		if nextAt.After(now) {
			return nextAt.Sub(now), nil
		}
		if err := job(ctx); err != nil {
			return 0, err
		}

		state.Timestamp = now
		return now.Sub(now.Add(interval)), s.Repository.Update(ctx, &state)
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
	JobID     string `ext:"id"`
	Timestamp time.Time
}

type ScheduleStateRepository interface {
	crud.Creator[ScheduleState]
	crud.Updater[ScheduleState]
	crud.ByIDDeleter[string]
	crud.ByIDFinder[ScheduleState, string]
}
