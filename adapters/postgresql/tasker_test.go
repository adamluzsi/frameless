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
	cm := GetConnectionManager(t)
	schedulecontracts.stateRepository(func(tb testing.TB) schedulecontracts.stateRepositorySubject {
		repo := &postgresql.TaskerScheduleStateRepository{CM: cm}
		assert.NoError(tb, repo.Migrate(context.Background()))
		return schedulecontracts.stateRepositorySubject{
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
