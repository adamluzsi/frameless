package memory

import "go.llib.dev/frameless/pkg/tasker"

func Scheduler() tasker.Scheduler {
	return tasker.Scheduler{
		Locks:  NewTaskerSchedulerLocks(),
		States: NewTaskerSchedulerStateRepository(),
	}
}

func NewTaskerSchedulerStateRepository() *Repository[tasker.ScheduleState, tasker.ScheduleID] {
	return NewRepository[tasker.ScheduleState, tasker.ScheduleID](NewMemory())
}

func NewTaskerSchedulerLocks() *LockerFactory[tasker.ScheduleID] {
	return NewLockerFactory[tasker.ScheduleID]()
}
