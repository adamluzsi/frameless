package postgresql

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/migration"
)

type TaskerSchedulerLocks struct{ Connection Connection }

func (lf TaskerSchedulerLocks) factory() LockerFactory[tasker.ScheduleID] {
	return LockerFactory[tasker.ScheduleID](lf)
}

func (lf TaskerSchedulerLocks) LockerFor(id tasker.ScheduleID) guard.Locker {
	return lf.factory().LockerFor(id)
}

func (lf TaskerSchedulerLocks) NonBlockingLockerFor(id tasker.ScheduleID) guard.NonBlockingLocker {
	return lf.factory().NonBlockingLockerFor(id)
}

type TaskerSchedulerStateRepository struct{ Connection Connection }

func (r TaskerSchedulerStateRepository) repository() Repository[tasker.ScheduleState, tasker.ScheduleID] {
	return Repository[tasker.ScheduleState, tasker.ScheduleID]{
		Mapping:    taskerScheduleStateRepositoryMapping,
		Connection: r.Connection,
	}
}

var taskerScheduleStateRepositoryMapping = flsql.Mapping[tasker.ScheduleState, tasker.ScheduleID]{
	TableName: "frameless_tasker_schedule_states",

	ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[tasker.ScheduleState]) {
		return []flsql.ColumnName{"id", "timestamp"},
			func(state *tasker.ScheduleState, s flsql.Scanner) error {
				if err := s.Scan(&state.ID, &state.Timestamp); err != nil {
					return err
				}
				state.Timestamp = state.Timestamp.UTC()
				return nil
			}
	},

	QueryID: func(si tasker.ScheduleID) (flsql.QueryArgs, error) {
		return flsql.QueryArgs{"id": si}, nil
	},

	ToArgs: func(s tasker.ScheduleState) (flsql.QueryArgs, error) {
		return flsql.QueryArgs{
			"id":        s.ID,
			"timestamp": s.Timestamp,
		}, nil
	},

	Prepare: func(ctx context.Context, s *tasker.ScheduleState) error {
		if s.ID == "" {
			return fmt.Errorf("tasker.ScheduleState.ID is required to be supplied externally")
		}
		return nil
	},

	ID: func(s *tasker.ScheduleState) *tasker.ScheduleID {
		return &s.ID
	},
}

func (r TaskerSchedulerStateRepository) Migrate(ctx context.Context) error {
	return MakeMigrator(r.Connection, "frameless_tasker_schedule_states", migration.Steps[Connection]{
		"0": flsql.MigrationStep[Connection]{
			UpQuery:   "CREATE TABLE IF NOT EXISTS frameless_tasker_schedule_states ( id TEXT PRIMARY KEY, timestamp TIMESTAMP WITH TIME ZONE NOT NULL );",
			DownQuery: "DROP TABLE IF EXISTS frameless_tasker_schedule_states;",
		},
	}).Migrate(ctx)
}

func (r TaskerSchedulerStateRepository) Create(ctx context.Context, ptr *tasker.ScheduleState) error {
	return r.repository().Create(ctx, ptr)
}

func (r TaskerSchedulerStateRepository) Update(ctx context.Context, ptr *tasker.ScheduleState) error {
	return r.repository().Update(ctx, ptr)
}

func (r TaskerSchedulerStateRepository) DeleteByID(ctx context.Context, id tasker.ScheduleID) error {
	return r.repository().DeleteByID(ctx, id)
}

func (r TaskerSchedulerStateRepository) FindByID(ctx context.Context, id tasker.ScheduleID) (ent tasker.ScheduleState, found bool, err error) {
	return r.repository().FindByID(ctx, id)
}
