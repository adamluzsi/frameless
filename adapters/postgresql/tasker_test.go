package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule/schedulecontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestTaskerScheduleStateRepository(t *testing.T) {
	db := GetDB(t)
	schedulecontracts.StateRepository(func(tb testing.TB) schedulecontracts.StateRepositorySubject {
		repo := &postgresql.TaskerScheduleStateRepository{DB: db}
		assert.NoError(tb, repo.Migrate(context.Background()))
		return schedulecontracts.StateRepositorySubject{
			StateRepository: repo,
			MakeContext:     context.Background,
			MakeScheduleState: func() schedule.State {
				t := testcase.ToT(&tb)
				return schedule.State{
					ID:        t.Random.String(),
					Timestamp: t.Random.Time().UTC(),
				}
			},
		}
	}).Test(t)
}
