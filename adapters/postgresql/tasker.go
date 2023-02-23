package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule"
	"github.com/adamluzsi/frameless/ports/iterators"
	"sync"
)

type TaskerScheduleStateRepository struct {
	DB *sql.DB

	once sync.Once
	repo Repository[schedule.State, string]
}

var migratorConfigTaskerScheduleStateRepository = MigratorConfig{
	Namespace: "frameless_tasker_schedule_states",
	Steps: []MigratorStep{
		MigrationStep{
			UpQuery:   "CREATE TABLE IF NOT EXISTS frameless_tasker_schedule_states ( id TEXT PRIMARY KEY, timestamp TIMESTAMP WITH TIME ZONE NOT NULL );",
			DownQuery: "DROP TABLE IF EXISTS frameless_tasker_schedule_states;",
		},
	},
}

func (r *TaskerScheduleStateRepository) init() {
	r.once.Do(func() {
		r.repo = Repository[schedule.State, string]{
			Mapping: Mapper[schedule.State, string]{
				Table: "frameless_tasker_schedule_states",
				ID:    "id",
				NewIDFn: func(context.Context) (string, error) {
					return "", fmt.Errorf(".ID is required to be supplied externally")
				},
				Columns: []string{"id", "timestamp"},
				ToArgsFn: func(ptr *schedule.State) ([]any, error) {
					return []any{
						ptr.ID,
						ptr.Timestamp,
					}, nil
				},
				MapFn: func(scanner iterators.SQLRowScanner) (schedule.State, error) {
					var state schedule.State
					err := scanner.Scan(&state.ID, &state.Timestamp)
					state.Timestamp = state.Timestamp.UTC()
					return state, err
				},
			},
			ConnectionManager: NewConnectionManagerWithDB(r.DB),
		}
	})
}

func (r *TaskerScheduleStateRepository) Migrate(ctx context.Context) error {
	return Migrator{
		DB:     r.DB,
		Config: migratorConfigTaskerScheduleStateRepository,
	}.Up(ctx)
}

func (r *TaskerScheduleStateRepository) Create(ctx context.Context, ptr *schedule.State) error {
	r.init()
	return r.repo.Create(ctx, ptr)
}

func (r *TaskerScheduleStateRepository) Update(ctx context.Context, ptr *schedule.State) error {
	r.init()
	return r.repo.Update(ctx, ptr)
}

func (r *TaskerScheduleStateRepository) DeleteByID(ctx context.Context, id string) error {
	r.init()
	return r.repo.DeleteByID(ctx, id)
}

func (r *TaskerScheduleStateRepository) FindByID(ctx context.Context, id string) (ent schedule.State, found bool, err error) {
	r.init()
	return r.repo.FindByID(ctx, id)
}
