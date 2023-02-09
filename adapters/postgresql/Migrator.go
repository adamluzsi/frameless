package postgresql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/adamluzsi/frameless/ports/comproto"
	_ "github.com/lib/pq" // side effect loading
)

type Migrator struct {
	DB     *sql.DB
	Config MigratorConfig
}

type MigratorConfig struct {
	Namespace string
	Steps     []MigratorStep
}

type MigratorStep interface {
	MigrateUp(ctx context.Context, tx *sql.Tx) error
	MigrateDown(ctx context.Context, tx *sql.Tx) error
}

type Migratable interface {
	Migrate(context.Context) error
}

func (m Migrator) Up(ctx context.Context) (rErr error) {
	if m.Config.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}

	if err := m.ensureMigrationTable(ctx); err != nil {
		return err
	}

	schemaTx, err := m.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer comproto.FinishTx(&rErr, schemaTx.Commit, schemaTx.Rollback)

	stepTx, err := m.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer comproto.FinishTx(&rErr, stepTx.Commit, stepTx.Rollback)

	for version, step := range m.Config.Steps {
		if err := m.upNamespace(ctx, schemaTx, stepTx, m.Config.Namespace, version, step); err != nil {
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

func (m Migrator) upNamespace(ctx context.Context, schemaTx, stepTx *sql.Tx, namespace string, version int, step MigratorStep) error {
	var dirty sql.NullBool
	err := schemaTx.QueryRowContext(ctx, queryMigratorGetStepState, namespace, version).Scan(&dirty)
	if err == sql.ErrNoRows {
		if err := step.MigrateUp(ctx, stepTx); err != nil {
			return err
		}
		_, err := schemaTx.ExecContext(ctx, queryMigratorCreateStepState, namespace, version, false)
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
	_, err := m.DB.ExecContext(ctx, queryEnsureSchemaMigrationsTable)
	return err
}

type MigrationStep struct {
	Up      func(ctx context.Context, tx *sql.Tx) error
	UpQuery string

	Down      func(ctx context.Context, tx *sql.Tx) error
	DownQuery string
}

func (m MigrationStep) MigrateUp(ctx context.Context, tx *sql.Tx) error {
	if m.Up != nil {
		return m.Up(ctx, tx)
	}
	if m.UpQuery != "" {
		_, err := tx.ExecContext(ctx, m.UpQuery)
		return err
	}
	return nil
}

func (m MigrationStep) MigrateDown(ctx context.Context, tx *sql.Tx) error {
	if m.Down != nil {
		return m.Down(ctx, tx)
	}
	if m.DownQuery != "" {
		_, err := tx.ExecContext(ctx, m.DownQuery)
		return err
	}
	return nil
}
