package postgresql

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/tasker/schedule"
	"github.com/adamluzsi/frameless/ports/iterators"
)

type TaskerScheduleStateRepository struct{ CM ConnectionManager }

var migratorConfigTaskerScheduleStateRepository = MigratorConfig{
	Namespace: "frameless_tasker_schedule_states",
	Steps: []MigratorStep{
		MigrationStep{
			UpQuery:   "CREATE TABLE IF NOT EXISTS frameless_tasker_schedule_states ( id TEXT PRIMARY KEY, timestamp TIMESTAMP WITH TIME ZONE NOT NULL );",
			DownQuery: "DROP TABLE IF EXISTS frameless_tasker_schedule_states;",
		},
	},
}

func (r *TaskerScheduleStateRepository) repository() Repository[schedule.State, string] {
	return Repository[schedule.State, string]{
		Mapping: taskerScheduleStateRepositoryMapping,
		CM:      r.CM,
	}
}

var taskerScheduleStateRepositoryMapping = Mapper[schedule.State, string]{
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
}

func (r *TaskerScheduleStateRepository) Migrate(ctx context.Context) error {
	return Migrator{
		CM:     r.CM,
		Config: migratorConfigTaskerScheduleStateRepository,
	}.Up(ctx)
}

func (r *TaskerScheduleStateRepository) Create(ctx context.Context, ptr *schedule.State) error {
	return r.repository().Create(ctx, ptr)
}

func (r *TaskerScheduleStateRepository) Update(ctx context.Context, ptr *schedule.State) error {
	return r.repository().Update(ctx, ptr)
}

func (r *TaskerScheduleStateRepository) DeleteByID(ctx context.Context, id string) error {
	return r.repository().DeleteByID(ctx, id)
}

func (r *TaskerScheduleStateRepository) FindByID(ctx context.Context, id string) (ent schedule.State, found bool, err error) {
	return r.repository().FindByID(ctx, id)
}
