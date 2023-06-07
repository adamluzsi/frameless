package postgresql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/ports/comproto"
)

type Migrator struct {
	CM     ConnectionManager
	Config MigratorConfig
}

type MigratorConfig struct {
	Namespace string
	Steps     []MigratorStep
}

type MigratorStep interface {
	MigrateUp(ConnectionManager, context.Context) error
	MigrateDown(ConnectionManager, context.Context) error
}

func (m Migrator) Up(ctx context.Context) (rErr error) {
	if m.Config.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}

	if err := m.ensureMigrationTable(ctx); err != nil {
		return err
	}

	schemaCTX, err := m.CM.BeginTx(ctx) // &sql.TxOptions{Isolation: sql.LevelSerializable}
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.CM, schemaCTX)

	stepCTX, err := m.CM.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.CM, stepCTX)

	for version, step := range m.Config.Steps {
		if err := m.upNamespace(schemaCTX, stepCTX, m.Config.Namespace, version, step); err != nil {
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
	err := m.CM.QueryRowContext(schemaTx, queryMigratorGetStepState, namespace, version).Scan(&dirty)
	if errors.Is(err, errNoRows) {
		if err := step.MigrateUp(m.CM, stepTx); err != nil {
			return err
		}
		_, err := m.CM.ExecContext(schemaTx, queryMigratorCreateStepState, namespace, version, false)
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
	_, err := m.CM.ExecContext(ctx, queryEnsureSchemaMigrationsTable)
	return err
}

type MigrationStep struct {
	Up      func(cm ConnectionManager, ctx context.Context) error
	UpQuery string

	Down      func(cm ConnectionManager, ctx context.Context) error
	DownQuery string
}

func (m MigrationStep) MigrateUp(cm ConnectionManager, ctx context.Context) error {
	if m.Up != nil {
		return m.Up(cm, ctx)
	}
	if m.UpQuery != "" {
		_, err := cm.ExecContext(ctx, m.UpQuery)
		return err
	}
	return nil
}

func (m MigrationStep) MigrateDown(cm ConnectionManager, ctx context.Context) error {
	if m.Down != nil {
		return m.Down(cm, ctx)
	}
	if m.DownQuery != "" {
		_, err := cm.ExecContext(ctx, m.DownQuery)
		return err
	}
	return nil
}
