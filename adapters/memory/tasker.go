package memory

import (
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/ports/guard"
)

type TaskerScheduleRepository struct {
	locks  *LockerFactory[tasker.StateID]
	states *Repository[tasker.State, tasker.StateID]
}

func (r *TaskerScheduleRepository) Locks() guard.LockerFactory[tasker.StateID] {
	return zerokit.Init(&r.locks, func() *LockerFactory[tasker.StateID] {
		return NewLockerFactory[tasker.StateID]()
	})
}

func (r *TaskerScheduleRepository) States() tasker.StateRepository {
	return zerokit.Init(&r.states, func() *Repository[tasker.State, tasker.StateID] {
		return NewRepository[tasker.State, tasker.StateID](NewMemory())
	})
}
