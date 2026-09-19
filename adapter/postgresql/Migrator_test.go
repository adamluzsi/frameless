package postgresql_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/adapter/postgresql/internal"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/port/migration/migrationcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func TestMakeMigrator(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ctx = let.Var(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			t.Cleanup(cancel)
			return ctx
		})
		connection = let.Var(s, func(t *testcase.T) postgresql.Connection {
			config := GetConnection(t).DB.Config()
			config.MinConns = 0
			config.MinIdleConns = 0
			config.MaxConns = 1
			pool, err := pgxpool.NewWithConfig(ctx.Get(t), config)
			assert.Must(t).NoError(err)
			t.Cleanup(pool.Close)
			assert.Must(t).NoError(pool.Ping(ctx.Get(t)))
			return postgresql.MakeConnectionFromPGXPool(pool)
		})
		table = let.Var(s, func(t *testcase.T) string {
			return pgx.Identifier{"test_migrator_" + t.Random.UUID()}.Sanitize()
		})
		stateID = let.Var(s, func(t *testcase.T) migration.StateID {
			return migration.StateID{Namespace: "test_make_migrator:" + t.Random.UUID(), Version: "1"}
		})
	)
	subject := let.Var(s, func(t *testcase.T) migration.Migrator[postgresql.Connection] {
		return postgresql.MakeMigrator(connection.Get(t), stateID.Get(t).Namespace,
			migration.Steps[postgresql.Connection]{
				stateID.Get(t).Version: flsql.MigrationStep[postgresql.Connection]{
					UpQuery:   "CREATE TABLE " + table.Get(t) + " (id TEXT PRIMARY KEY)",
					DownQuery: "DROP TABLE " + table.Get(t),
				},
			})
	})

	s.Before(func(t *testcase.T) {
		conn, table, id := connection.Get(t), table.Get(t), stateID.Get(t)
		assert.Must(t).NoError(postgresql.EnsureStateRepository(ctx.Get(t), conn))
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := conn.ExecContext(ctx, "DROP TABLE IF EXISTS "+table)
			assert.Should(t).NoError(err)
			_, err = conn.ExecContext(ctx, "DELETE FROM frameless_schema_migrations WHERE namespace = $1", id.Namespace)
			assert.Should(t).NoError(err)
		})
	})

	hasTable := func(t *testcase.T) bool {
		t.Helper()
		var exists bool
		err := connection.Get(t).QueryRowContext(ctx.Get(t), "SELECT to_regclass($1) IS NOT NULL", table.Get(t)).Scan(&exists)
		assert.Must(t).NoError(err)
		return exists
	}

	s.Describe("#Migrate", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).Migrate(ctx.Get(t))
		}

		s.Then("creates the table and records a clean migration with only one pool connection", func(t *testcase.T) {
			assert.False(t, hasTable(t))
			assert.NoError(t, act(t))
			assert.True(t, hasTable(t))

			state, found, err := subject.Get(t).StateRepository.FindByID(ctx.Get(t), stateID.Get(t))
			assert.NoError(t, err)
			assert.True(t, found)
			assert.Equal(t, state, migration.State{ID: stateID.Get(t), Dirty: false})
		})
	})

	s.Describe("#MigrateDown", func(s *testcase.Spec) {
		act := func(t *testcase.T) error {
			return subject.Get(t).MigrateDown(ctx.Get(t), "")
		}

		s.Before(func(t *testcase.T) {
			assert.Must(t).NoError(subject.Get(t).Migrate(ctx.Get(t)))
		})

		s.Then("drops the table and removes its migration state with only one pool connection", func(t *testcase.T) {
			assert.True(t, hasTable(t))
			assert.NoError(t, act(t))
			assert.False(t, hasTable(t))

			_, found, err := subject.Get(t).StateRepository.FindByID(ctx.Get(t), stateID.Get(t))
			assert.NoError(t, err)
			assert.False(t, found)
		})
	})
}

func TestMigrationStateRepository(t *testing.T) {
	logger.Testing(t)
	conn := GetConnection(t)
	repo := postgresql.NewMigrationStateRepository(conn)
	repo.Mapping.TableName = "frameless_schema_migrations_test"
	ctx := context.Background()
	queryStateRepoCreate, err := internal.QueryEnsureSchemaMigrationsTable(repo.Mapping.TableName)
	assert.NoError(t, err)
	_, err = conn.ExecContext(ctx, queryStateRepoCreate)
	assert.NoError(t, err)
	t.Cleanup(func() { conn.ExecContext(ctx, `DROP TABLE IF EXISTS frameless_schema_migrations_test`) })
	migrationcontract.StateRepository(repo).Test(t)
}
