package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/tasks/schedule"
	"github.com/adamluzsi/frameless/ports/iterators"
	"sync"
)

type JobsScheduleStateRepository struct {
	DB *sql.DB

	once sync.Once
	repo Repository[schedule.State, string]
}

var migratorConfigJobScheduleStateRepository = MigratorConfig{
	Namespace: "frameless_job_schedule_states",
	Steps: []MigratorStep{
		MigrationStep{
			UpQuery:   "CREATE TABLE IF NOT EXISTS frameless_job_schedule_states ( jobid TEXT PRIMARY KEY, timestamp TIMESTAMP WITH TIME ZONE NOT NULL );",
			DownQuery: "DROP TABLE IF EXISTS frameless_job_schedule_states;",
		},
	},
}

func (r *JobsScheduleStateRepository) init() {
	r.once.Do(func() {
		r.repo = Repository[schedule.State, string]{
			Mapping: Mapper[schedule.State, string]{
				Table: "frameless_job_schedule_states",
				ID:    "jobid",
				NewIDFn: func(context.Context) (string, error) {
					return "", fmt.Errorf("jobID is required to be supplied externally")
				},
				Columns: []string{"jobid", "timestamp"},
				ToArgsFn: func(ptr *schedule.State) ([]any, error) {
					return []any{
						ptr.JobID,
						ptr.Timestamp,
					}, nil
				},
				MapFn: func(scanner iterators.SQLRowScanner) (schedule.State, error) {
					var state schedule.State
					err := scanner.Scan(&state.JobID, &state.Timestamp)
					state.Timestamp = state.Timestamp.UTC()
					return state, err
				},
			},
			ConnectionManager: NewConnectionManagerWithDB(r.DB),
		}
	})

}
func (r *JobsScheduleStateRepository) Migrate(ctx context.Context) error {
	return Migrator{
		DB:     r.DB,
		Config: migratorConfigJobScheduleStateRepository,
	}.Up(ctx)
}

func (r *JobsScheduleStateRepository) Create(ctx context.Context, ptr *schedule.State) error {
	r.init()
	return r.repo.Create(ctx, ptr)
}

func (r *JobsScheduleStateRepository) Update(ctx context.Context, ptr *schedule.State) error {
	r.init()
	return r.repo.Update(ctx, ptr)
}

func (r *JobsScheduleStateRepository) DeleteByID(ctx context.Context, jobID string) error {
	r.init()
	return r.repo.DeleteByID(ctx, jobID)
}

func (r *JobsScheduleStateRepository) FindByID(ctx context.Context, jobID string) (ent schedule.State, found bool, err error) {
	r.init()
	return r.repo.FindByID(ctx, jobID)
}
