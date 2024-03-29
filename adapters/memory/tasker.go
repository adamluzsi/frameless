package memory

import (
	"go.llib.dev/frameless/pkg/tasker/schedule"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/ports/guard"
)

type TaskerScheduleRepository struct {
	locks  *LockerFactory[schedule.StateID]
	states *Repository[schedule.State, schedule.StateID]
}

func (r *TaskerScheduleRepository) Locks() guard.LockerFactory[schedule.StateID] {
	return zerokit.Init(&r.locks, func() *LockerFactory[schedule.StateID] {
		return NewLockerFactory[schedule.StateID]()
	})
}

func (r *TaskerScheduleRepository) States() schedule.StateRepository {
	return zerokit.Init(&r.states, func() *Repository[schedule.State, schedule.StateID] {
		return NewRepository[schedule.State, schedule.StateID](NewMemory())
	})
}
