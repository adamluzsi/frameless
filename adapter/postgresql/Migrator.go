package postgresql

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/adapter/postgresql/internal"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/port/migration"
)

func makeMigrator(conn Connection, namespace string, steps migration.Steps[Connection]) migration.Migrator[Connection] {
	return migration.Migrator[Connection]{
		Namespace:       namespace,
		Resource:        conn,
		StateRepository: NewMigrationStateRepository(conn),
		EnsureStateRepository: func(ctx context.Context) error {
			return EnsureStateRepository(ctx, conn)
		},
		Steps: steps,
	}
}

const schemaMigrationsTableName = "frameless_schema_migrations"

func EnsureStateRepository(ctx context.Context, conn Connection) error {
	query, err := internal.QueryEnsureSchemaMigrationsTable(schemaMigrationsTableName)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, query)
	return err
}

func NewMigrationStateRepository(conn Connection) Repository[migration.State, migration.StateID] {
	return Repository[migration.State, migration.StateID]{
		Connection: conn,
		Mapping: flsql.Mapping[migration.State, migration.StateID]{
			TableName: schemaMigrationsTableName,
			ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[migration.State]) {
				return []flsql.ColumnName{"namespace", "version", "dirty"},
					func(v *migration.State, s flsql.Scanner) error {
						return s.Scan(&v.ID.Namespace, &v.ID.Version, &v.Dirty)
					}
			},
			QueryID: func(id migration.StateID) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{
					"namespace": id.Namespace,
					"version":   id.Version,
				}, nil
			},

			ToArgs: func(s migration.State) (flsql.QueryArgs, error) {
				return flsql.QueryArgs{
					"namespace": s.ID.Namespace,
					"version":   s.ID.Version,
					"dirty":     s.Dirty,
				}, nil
			},

			CreatePrepare: func(ctx context.Context, s *migration.State) error {
				if s.ID.Namespace == "" {
					return fmt.Errorf("MigrationStateRepository requires a non-empty namespace for Create")
				}
				if s.ID.Version == "" {
					return fmt.Errorf("MigrationStateRepository requires a non-empty version for Create")
				}
				return nil
			},

			ID: func(s *migration.State) *migration.StateID { return &s.ID },
		},
	}
}
