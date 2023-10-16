package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/migration"
)

type Migrator struct {
	Connection Connection
	Group      MigratorGroup
}

var _ migration.Migratable = Migrator{}

type (
	MigratorGroup = migration.Group[Connection]
	MigratorStep  = migration.Step[Connection]
)

func (m Migrator) Migrate(ctx context.Context) (rErr error) {
	if m.Group.ID == "" {
		return fmt.Errorf("missing namespace")
	}

	if err := m.ensureMigrationTable(ctx); err != nil {
		return err
	}

	schemaCTX, err := m.Connection.BeginTx(ctx) // &sql.TxOptions{Isolation: sql.LevelSerializable}
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.Connection, schemaCTX)

	stepCTX, err := m.Connection.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.Connection, stepCTX)

	for version, step := range m.Group.Steps {
		if err := m.upNamespace(schemaCTX, stepCTX, m.Group.ID, version, step); err != nil {
			return err
		}
	}

	return nil
}

const queryMigratorGetStepState = `
SELECT dirty 
FROM frameless_schema_migrations
WHERE namespace = $1
  AND version = $2
`

const queryMigratorCreateStepState = `
INSERT INTO frameless_schema_migrations (namespace, version, dirty) 
VALUES ($1, $2, $3)
`

func (m Migrator) upNamespace(schemaTx, stepTx context.Context, namespace string, version int, step MigratorStep) error {
	var dirty sql.NullBool
	err := m.Connection.QueryRowContext(schemaTx, queryMigratorGetStepState, namespace, version).Scan(&dirty)
	if errors.Is(err, errNoRows) {
		if err := step.MigrateUp(m.Connection, stepTx); err != nil {
			return err
		}
		_, err := m.Connection.ExecContext(schemaTx, queryMigratorCreateStepState, namespace, version, false)
		return err
	}
	if err != nil {
		return err
	}
	if dirty.Valid && dirty.Bool {
		return fmt.Errorf("namespace:%q / version:%d is in a dirty state", namespace, version)
	}
	return nil
}

const queryEnsureSchemaMigrationsTable = `
CREATE TABLE IF NOT EXISTS frameless_schema_migrations (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    namespace TEXT NOT NULL,
	version INT NOT NULL,
	dirty BOOLEAN NOT NULL
);
`

func (m Migrator) ensureMigrationTable(ctx context.Context) error {
	_, err := m.Connection.ExecContext(ctx, queryEnsureSchemaMigrationsTable)
	return err
}

type MigrationStep struct {
	Up      func(cm Connection, ctx context.Context) error
	UpQuery string

	Down      func(cm Connection, ctx context.Context) error
	DownQuery string
}

func (m MigrationStep) MigrateUp(cm Connection, ctx context.Context) error {
	if m.Up != nil {
		return m.Up(cm, ctx)
	}
	if m.UpQuery != "" {
		_, err := cm.ExecContext(ctx, m.UpQuery)
		return err
	}
	return nil
}

func (m MigrationStep) MigrateDown(cm Connection, ctx context.Context) error {
	if m.Down != nil {
		return m.Down(cm, ctx)
	}
	if m.DownQuery != "" {
		_, err := cm.ExecContext(ctx, m.DownQuery)
		return err
	}
	return nil
}
