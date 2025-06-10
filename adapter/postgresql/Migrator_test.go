package postgresql_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/adapter/postgresql/internal"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/port/migration/migrationcontracts"
	"go.llib.dev/testcase/assert"
)

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
	migrationcontracts.StateRepository(repo).Test(t)
}
