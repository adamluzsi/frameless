package postgresql_test

import (
	"context"
	"go.llib.dev/frameless/adapters/postgresql"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/pkg/tasker/schedule"
	"go.llib.dev/frameless/pkg/tasker/schedule/schedulecontracts"
	"github.com/adamluzsi/testcase/assert"
	"os"
	"testing"
	"time"
)

func TestTaskerScheduleRepository(t *testing.T) {
	cm := GetConnection(t)
	schedulecontracts.Repository(func(tb testing.TB) schedulecontracts.RepositorySubject {
		repo := &postgresql.TaskerScheduleRepository{Connection: cm}
		assert.NoError(tb, repo.Migrate(context.Background()))
		return schedulecontracts.RepositorySubject{
			Repository:  repo,
			MakeContext: context.Background,
		}
	}).Test(t)
}

func ExampleTaskerScheduleRepository() {
	c, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err.Error())
	}

	s := schedule.Scheduler{
		Repository: postgresql.TaskerScheduleRepository{Connection: c},
	}

	maintenance := s.WithSchedule("maintenance", schedule.Monthly{Day: 1, Hour: 12, Location: time.UTC},
		func(ctx context.Context) error {
			// The monthly maintenance task
			return nil
		})

	// form your main func
	_ = tasker.Main(context.Background(), maintenance)
}
