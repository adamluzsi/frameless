package memory_test

import (
	"context"
	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/pkg/tasker/schedule/schedulecontracts"
	"testing"
)

func TestTaskerScheduleRepository(t *testing.T) {
	repo := &memory.TaskerScheduleRepository{}
	
	schedulecontracts.Repository(func(tb testing.TB) schedulecontracts.RepositorySubject {
		return schedulecontracts.RepositorySubject{
			Repository:        repo,
			MakeContext:       context.Background,
		}
	}).Test(t)
}
