package postgresql

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/migration"
)

type TaskerScheduleRepository struct{ Connection Connection }

func (r TaskerScheduleRepository) Migrate(ctx context.Context) error {
	if m, ok := r.States().(migration.Migratable); ok {
		if err := m.Migrate(ctx); err != nil {
			return err
		}
	}
	if m, ok := r.Locks().(migration.Migratable); ok {
		if err := m.Migrate(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r TaskerScheduleRepository) Locks() guard.LockerFactory[tasker.StateID] {
	return LockerFactory[tasker.StateID]{Connection: r.Connection}
}

func (r TaskerScheduleRepository) States() tasker.StateRepository {
	return TaskerScheduleStateRepository{Connection: r.Connection}
}

var migratorConfigTaskerScheduleStateRepository = MigratorGroup{
	ID: "frameless_tasker_schedule_states",
	Steps: []MigratorStep{
		MigrationStep{
			UpQuery:   "CREATE TABLE IF NOT EXISTS frameless_tasker_schedule_states ( id TEXT PRIMARY KEY, timestamp TIMESTAMP WITH TIME ZONE NOT NULL );",
			DownQuery: "DROP TABLE IF EXISTS frameless_tasker_schedule_states;",
		},
	},
}

type TaskerScheduleStateRepository struct{ Connection Connection }

func (r TaskerScheduleStateRepository) repository() Repository[tasker.State, tasker.StateID] {
	return Repository[tasker.State, tasker.StateID]{
		Mapping:    taskerScheduleStateRepositoryMapping,
		Connection: r.Connection,
	}
}

var taskerScheduleStateRepositoryMapping = flsql.Mapping[tasker.State, tasker.StateID]{
	TableName: "frameless_tasker_schedule_states",

	ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[tasker.State]) {
		return []flsql.ColumnName{"id", "timestamp"},
			func(state *tasker.State, scan flsql.ScanFunc) error {
				if err := scan(&state.ID, &state.Timestamp); err != nil {
					return err
				}
				state.Timestamp = state.Timestamp.UTC()
				return nil
			}
	},

	ToID: func(si tasker.StateID) (map[flsql.ColumnName]any, error) {
		return map[flsql.ColumnName]any{"id": si}, nil
	},

	ToArgs: func(s tasker.State) (map[flsql.ColumnName]any, error) {
		return map[flsql.ColumnName]any{
			"id":        s.ID,
			"timestamp": s.Timestamp,
		}, nil
	},

	CreatePrepare: func(ctx context.Context, s *tasker.State) error {
		if s.ID == "" {
			return fmt.Errorf("tasker.State.ID is required to be supplied externally")
		}
		return nil
	},

	GetID: func(s tasker.State) tasker.StateID {
		return s.ID
	},
}

func (r TaskerScheduleStateRepository) Migrate(ctx context.Context) error {
	return Migrator{
		Connection: r.Connection,
		Group:      migratorConfigTaskerScheduleStateRepository,
	}.Migrate(ctx)
}

func (r TaskerScheduleStateRepository) Create(ctx context.Context, ptr *tasker.State) error {
	return r.repository().Create(ctx, ptr)
}

func (r TaskerScheduleStateRepository) Update(ctx context.Context, ptr *tasker.State) error {
	return r.repository().Update(ctx, ptr)
}

func (r TaskerScheduleStateRepository) DeleteByID(ctx context.Context, id tasker.StateID) error {
	return r.repository().DeleteByID(ctx, id)
}

func (r TaskerScheduleStateRepository) FindByID(ctx context.Context, id tasker.StateID) (ent tasker.State, found bool, err error) {
	return r.repository().FindByID(ctx, id)
}
