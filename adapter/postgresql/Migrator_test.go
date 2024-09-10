package postgresql_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/port/migration/migrationcontracts"
)

const queryStateRepoCreate = `
CREATE TABLE IF NOT EXISTS frameless_schema_migrations_test (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    namespace TEXT    NOT NULL,
	version   TEXT    NOT NULL,
	dirty     BOOLEAN NOT NULL
);
`

func TestMigrationStateRepository(t *testing.T) {
	logger.Testing(t)
	conn := GetConnection(t)
	repo := postgresql.NewMigrationStateRepository(conn)
	repo.Mapping.TableName = "frameless_schema_migrations_test"
	ctx := context.Background()
	conn.ExecContext(ctx, queryStateRepoCreate)
	t.Cleanup(func() { conn.ExecContext(ctx, `DROP TABLE IF EXISTS frameless_schema_migrations_test`) })
	migrationcontracts.StateRepository(repo).Test(t)
}
