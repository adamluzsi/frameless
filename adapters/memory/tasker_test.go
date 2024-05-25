package memory_test

import (
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/tasker/schedule/schedulecontracts"
)

func TestTaskerScheduleRepository(t *testing.T) {
	repo := &memory.TaskerScheduleRepository{}

	schedulecontracts.Repository(repo).Test(t)
}
