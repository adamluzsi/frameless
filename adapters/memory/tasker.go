package memory

import (
	"github.com/adamluzsi/frameless/pkg/tasker/schedule"
	"github.com/adamluzsi/frameless/pkg/zerokit"
	"github.com/adamluzsi/frameless/ports/locks"
)

type TaskerScheduleRepository struct {
	locks  *LockerFactory[schedule.StateID]
	states *Repository[schedule.State, schedule.StateID]
}

func (r *TaskerScheduleRepository) Locks() locks.Factory[schedule.StateID] {
	return zerokit.Init(&r.locks, func() *LockerFactory[schedule.StateID] {
		return NewLockerFactory[schedule.StateID]()
	})
}

func (r *TaskerScheduleRepository) States() schedule.StateRepository {
	return zerokit.Init(&r.states, func() *Repository[schedule.State, schedule.StateID] {
		return NewRepository[schedule.State, schedule.StateID](NewMemory())
	})
}
