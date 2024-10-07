package memory_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/tasker/taskercontracts"
)

func TestTasker(t *testing.T) {
	taskercontracts.SchedulerStateRepository(memory.NewTaskerSchedulerStateRepository()).Test(t)
	taskercontracts.SchedulerLocks(memory.NewTaskerSchedulerLocks()).Test(t)

	scheduler := memory.Scheduler()
	taskercontracts.SchedulerStateRepository(scheduler.States).Test(t)
	taskercontracts.SchedulerLocks(scheduler.Locks).Test(t)
}
