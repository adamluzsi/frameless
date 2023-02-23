package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/pkg/tasks/schedule"
	"github.com/adamluzsi/frameless/pkg/tasks/schedule/schedulecontracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"testing"
)

func TestJobsScheduleStateRepository(t *testing.T) {
	db := OpenDB(t)
	schedulecontracts.StateRepository{
		MakeSubject: func(tb testing.TB) schedule.StateRepository {
			repo := &postgresql.JobsScheduleStateRepository{DB: db}
			assert.NoError(tb, repo.Migrate(context.Background()))
			return repo
		},
		MakeContext: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeScheduleState: func(tb testing.TB) schedule.State {
			t := testcase.ToT(&tb)
			return schedule.State{
				JobID:     t.Random.String(),
				Timestamp: t.Random.Time().UTC(),
			}
		},
	}.Test(t)
}
