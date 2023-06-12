package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule/schedulecontracts"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestTaskerScheduleRepository(t *testing.T) {
	cm := GetConnectionManager(t)
	schedulecontracts.Repository(func(tb testing.TB) schedulecontracts.RepositorySubject {
		repo := &postgresql.TaskerScheduleRepository{CM: cm}
		assert.NoError(tb, repo.Migrate(context.Background()))
		return schedulecontracts.RepositorySubject{
			Repository: repo, 
			MakeContext:     context.Background,
		}
	}).Test(t)
}
