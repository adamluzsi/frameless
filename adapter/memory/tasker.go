package memory

import (
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/guard"
)

func NewTaskerSchedulerStateRepository() *Repository[tasker.ScheduleState, tasker.ScheduleStateID] {
	return NewRepository[tasker.ScheduleState, tasker.ScheduleStateID](NewMemory())
}

func NewTaskerSchedulerLocks() *LockerFactory[tasker.ScheduleStateID] {
	return NewLockerFactory[tasker.ScheduleStateID]()
}

type TaskerScheduleRepository struct {
	locks  *LockerFactory[tasker.ScheduleStateID]
	states *Repository[tasker.ScheduleState, tasker.ScheduleStateID]
}

func (r *TaskerScheduleRepository) Locks() guard.LockerFactory[tasker.ScheduleStateID] {
	return zerokit.Init(&r.locks, func() *LockerFactory[tasker.ScheduleStateID] {
		return NewLockerFactory[tasker.ScheduleStateID]()
	})
}

func (r *TaskerScheduleRepository) States() tasker.SchedulerStateRepository {
	return zerokit.Init(&r.states, func() *Repository[tasker.ScheduleState, tasker.ScheduleStateID] {
		return NewRepository[tasker.ScheduleState, tasker.ScheduleStateID](NewMemory())
	})
}
