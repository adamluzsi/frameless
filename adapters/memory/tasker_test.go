package memory_test

import (
	"testing"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/tasker/taskercontracts"
)

func TestTaskerScheduleRepository(t *testing.T) {
	repo := &memory.TaskerScheduleRepository{}

	taskercontracts.Repository(repo).Test(t)
}
