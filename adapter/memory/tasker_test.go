package memory_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/tasker/taskercontract"
)

func TestTasker(t *testing.T) {
	taskercontract.ScheduleStateRepository(memory.NewTaskerSchedulerStateRepository()).Test(t)
	taskercontract.SchedulerLocks(memory.NewTaskerSchedulerLocks()).Test(t)

	scheduler := memory.Scheduler()
	taskercontract.ScheduleStateRepository(scheduler.States).Test(t)
	taskercontract.SchedulerLocks(scheduler.Locks).Test(t)
}
