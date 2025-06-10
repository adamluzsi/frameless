package postgresql_test

import (
	"context"
	"os"
	"testing"
	"time"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/tasker/taskercontracts"
	"go.llib.dev/testcase/assert"
)

func TestTaskerScheduleRepository(t *testing.T) {
	cm := GetConnection(t)
	ctx := context.Background()

	stateRepo := postgresql.TaskerSchedulerStateRepository{Connection: cm}
	assert.NoError(t, stateRepo.Migrate(ctx))

	locks := postgresql.TaskerSchedulerLocks{Connection: cm}

	taskercontracts.ScheduleStateRepository(stateRepo).Test(t)
	taskercontracts.SchedulerLocks(locks).Test(t)
}

func ExampleTaskerSchedulerStateRepository() {
	c, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err.Error())
	}

	s := tasker.Scheduler{
		Locks:  postgresql.TaskerSchedulerLocks{Connection: c},
		States: postgresql.TaskerSchedulerStateRepository{Connection: c},
	}

	maintenance := s.WithSchedule("maintenance", tasker.Monthly{Day: 1, Hour: 12, Location: time.UTC},
		func(ctx context.Context) error {
			// The monthly maintenance task
			return nil
		})

	// form your main func
	_ = tasker.Main(context.Background(), maintenance)
}
