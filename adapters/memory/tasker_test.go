package memory_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule/schedulecontracts"
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
