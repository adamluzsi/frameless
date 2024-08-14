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
	repo := &postgresql.TaskerScheduleRepository{Connection: cm}
	assert.NoError(t, repo.Migrate(context.Background()))
	taskercontracts.Repository(repo).Test(t)
}

func ExampleTaskerScheduleRepository() {
	c, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err.Error())
	}

	s := tasker.Scheduler{
		Repository: postgresql.TaskerScheduleRepository{Connection: c},
	}

	maintenance := s.WithSchedule("maintenance", tasker.Monthly{Day: 1, Hour: 12, Location: time.UTC},
		func(ctx context.Context) error {
			// The monthly maintenance task
			return nil
		})

	// form your main func
	_ = tasker.Main(context.Background(), maintenance)
}
