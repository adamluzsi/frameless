package postgresql_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestSplitSchemaTable(t *testing.T) {
	var schema, table pgx.Identifier

	schema, table = postgresql.SplitSchemaTable(pgx.Identifier{})
	assert.Equal(t, schema, nil)
	assert.Equal(t, table, nil)

	schema, table = postgresql.SplitSchemaTable(pgx.Identifier{"foo"})
	assert.Equal(t, schema, nil)
	assert.Equal(t, table, pgx.Identifier{"foo"})

	schema, table = postgresql.SplitSchemaTable(pgx.Identifier{"bar", "baz"})
	assert.Equal(t, schema, pgx.Identifier{"bar"})
	assert.Equal(t, table, pgx.Identifier{"baz"})

	schema, table = postgresql.SplitSchemaTable(pgx.Identifier{"qux", "quux", "corge"})
	assert.Equal(t, schema, pgx.Identifier{"qux"})
	assert.Equal(t, table, pgx.Identifier{"quux", "corge"})
}

func randomName(tb testing.TB, prefix string) string {
	tb.Helper()
	return prefix + "_" + rnd.StringNWithCharset(10, strings.ToLower(random.CharsetAlpha()))
}

func mustExec(tb testing.TB, c postgresql.Connection, query string, args ...any) {
	tb.Helper()
	_, err := c.ExecContext(tb.Context(), query, args...)
	assert.NoError(tb, err, assert.MessageF("query: %s", query))
}

func dropTable(tb testing.TB, c postgresql.Connection, name string) {
	tb.Helper()
	tb.Cleanup(func() {
		// t.Context() is already canceled by the time Cleanup runs, so use a
		// fresh background context for teardown.
		_, err := c.ExecContext(context.Background(),
			fmt.Sprintf(`DROP TABLE IF EXISTS %s CASCADE`, pgx.Identifier{name}.Sanitize()))
		assert.Should(tb).NoError(err)
	})
}

func createSchema(tb testing.TB, c postgresql.Connection, name string) {
	tb.Helper()
	mustExec(tb, c, fmt.Sprintf(`CREATE SCHEMA %s`, pgx.Identifier{name}.Sanitize()))
	tb.Cleanup(func() {
		// t.Context() is already canceled by the time Cleanup runs, so use a
		// fresh background context for teardown. CASCADE removes the tables and
		// indexes created inside the schema during the test.
		_, err := c.ExecContext(context.Background(),
			fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, pgx.Identifier{name}.Sanitize()))
		assert.Should(tb).NoError(err)
	})
}

func indexExists(tb testing.TB, c postgresql.Connection, id pgx.Identifier) bool {
	tb.Helper()
	var exists bool
	const q = `
SELECT EXISTS (
	SELECT 1 FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relkind = 'i' AND c.relname = $2 AND n.nspname = COALESCE($1, current_schema())
)`

	table := id[len(id)-1]
	schema := sql.NullString{}

	if len(id) == 2 {
		schema.String = id[0]
		schema.Valid = true
	}

	assert.NoError(tb, c.QueryRowContext(tb.Context(), q, schema, table).Scan(&exists))
	return exists
}

func TestDropTableIndexes(t *testing.T) {
	c := GetConnection(t)
	ctx := t.Context()

	useRandomSchema := rnd.Bool()
	useRandomSchema = false

	schema := randomName(t, "test_schema")
	if useRandomSchema {
		createSchema(t, c, schema)
	}

	table := randomName(t, "test_disable_idx")
	tableID := pgx.Identifier{table}

	if useRandomSchema {
		slicekit.Unshift(&tableID, schema)
	}

	dropTable(t, c, table)
	mustExec(t, c, fmt.Sprintf(`CREATE TABLE %s (id int PRIMARY KEY, val text, tag text)`,
		tableID.Sanitize()))

	valIdxName := table + "_val_idx"
	tagIdxName := table + "_tag_idx"

	valIdx := pgx.Identifier{valIdxName}
	tagIdx := pgx.Identifier{tagIdxName}
	pkIdx := pgx.Identifier{table + "_pkey"}

	if useRandomSchema {
		slicekit.Unshift(&valIdx, schema)
		slicekit.Unshift(&tagIdx, schema)
		slicekit.Unshift(&pkIdx, schema)
	}

	// An index name cannot be schema-qualified in CREATE INDEX; the index is
	// always created in the parent table's schema. Use the bare relation name
	// here and keep the schema-qualified identifiers for the existence checks.
	mustExec(t, c, fmt.Sprintf(`CREATE INDEX %s ON %s (val)`,
		pgx.Identifier{valIdxName}.Sanitize(), tableID.Sanitize()))
	mustExec(t, c, fmt.Sprintf(`CREATE INDEX %s ON %s (tag)`,
		pgx.Identifier{tagIdxName}.Sanitize(), tableID.Sanitize()))

	// sanity: the two secondary indexes and the primary key all exist.
	assert.True(t, indexExists(t, c, valIdx))
	assert.True(t, indexExists(t, c, tagIdx))
	assert.True(t, indexExists(t, c, pkIdx))

	recreate, err := postgresql.DropTableIndexes(ctx, c, tableID)
	assert.NoError(t, err)
	assert.NotNil(t, recreate)

	t.Run("secondary indexes are dropped, the primary key is preserved", func(t *testing.T) {
		assert.False(t, indexExists(t, c, valIdx))
		assert.False(t, indexExists(t, c, tagIdx))
		assert.True(t, indexExists(t, c, pkIdx))
	})

	t.Run("recreate restores the secondary indexes", func(t *testing.T) {
		assert.NoError(t, recreate())
		assert.True(t, indexExists(t, c, valIdx))
		assert.True(t, indexExists(t, c, tagIdx))
		assert.True(t, indexExists(t, c, pkIdx))
	})
}

// TestDropTableIndexes_exclusionConstraintIndexIsPreserved verifies that an index
// backing an exclusion constraint is NOT dropped by DropTableIndexes.
//
// Such an index is neither unique nor primary, but it is owned by a constraint,
// so DropTableIndexes must leave it in place (like unique/primary indexes):
// PostgreSQL forbids DROP INDEX on a constraint-owned index, and dropping it
// would stop enforcing the constraint during the load. The regular secondary
// index must still be disabled and recreated.
func TestDropTableIndexes_exclusionConstraintIndexIsPreserved(t *testing.T) {
	c := GetConnection(t)
	ctx := t.Context()

	table := randomName(t, "test_disable_idx_excl")
	tableID := pgx.Identifier{table}

	dropTable(t, c, table)

	// GiST on range types is built-in, so the exclusion constraint needs no
	// extension. The named constraint yields an index of the same name, which
	// keeps the existence check deterministic.
	exclName := table + "_excl"
	mustExec(t, c, fmt.Sprintf(
		`CREATE TABLE %s (id int PRIMARY KEY, val text, r int4range, `+
			`CONSTRAINT %s EXCLUDE USING gist (r WITH &&))`,
		tableID.Sanitize(), pgx.Identifier{exclName}.Sanitize()))

	valIdxName := table + "_val_idx"
	mustExec(t, c, fmt.Sprintf(`CREATE INDEX %s ON %s (val)`,
		pgx.Identifier{valIdxName}.Sanitize(), tableID.Sanitize()))

	valIdx := pgx.Identifier{valIdxName}
	exclIdx := pgx.Identifier{exclName}
	pkIdx := pgx.Identifier{table + "_pkey"}

	// sanity: regular secondary index, exclusion-constraint index and the
	// primary key all exist.
	assert.True(t, indexExists(t, c, valIdx))
	assert.True(t, indexExists(t, c, exclIdx))
	assert.True(t, indexExists(t, c, pkIdx))

	recreate, err := postgresql.DropTableIndexes(ctx, c, tableID)
	assert.NoError(t, err)
	assert.NotNil(t, recreate)

	// The constraint-owned index and the primary key must survive; only the
	// plain secondary index should be dropped.
	assert.True(t, indexExists(t, c, exclIdx))
	assert.True(t, indexExists(t, c, pkIdx))
	assert.False(t, indexExists(t, c, valIdx))

	// And the regular secondary index round-trips as usual.
	assert.NoError(t, recreate())
	assert.True(t, indexExists(t, c, valIdx))
	assert.True(t, indexExists(t, c, exclIdx))
	assert.True(t, indexExists(t, c, pkIdx))
}

// TestDropTableIndexes_bareUniqueIndexIsPreserved verifies that a standalone
// unique index (created with CREATE UNIQUE INDEX, not backing a constraint) is
// left in place. Such an index has no internal pg_depend row, so the pg_depend
// guard does not catch it; the NOT uniqueness predicate is what preserves it.
// This locks in that half of the filter independently of the constraint guard.
func TestDropTableIndexes_bareUniqueIndexIsPreserved(t *testing.T) {
	c := GetConnection(t)
	ctx := t.Context()

	table := randomName(t, "test_disable_idx_uniq")
	tableID := pgx.Identifier{table}

	dropTable(t, c, table)
	mustExec(t, c, fmt.Sprintf(`CREATE TABLE %s (id int PRIMARY KEY, val text, tag text)`,
		tableID.Sanitize()))

	uniqIdxName := table + "_val_uniq_idx"
	tagIdxName := table + "_tag_idx"

	mustExec(t, c, fmt.Sprintf(`CREATE UNIQUE INDEX %s ON %s (val)`,
		pgx.Identifier{uniqIdxName}.Sanitize(), tableID.Sanitize()))
	mustExec(t, c, fmt.Sprintf(`CREATE INDEX %s ON %s (tag)`,
		pgx.Identifier{tagIdxName}.Sanitize(), tableID.Sanitize()))

	uniqIdx := pgx.Identifier{uniqIdxName}
	tagIdx := pgx.Identifier{tagIdxName}
	pkIdx := pgx.Identifier{table + "_pkey"}

	// sanity: the unique index, the plain secondary index and the primary key
	// all exist.
	assert.True(t, indexExists(t, c, uniqIdx))
	assert.True(t, indexExists(t, c, tagIdx))
	assert.True(t, indexExists(t, c, pkIdx))

	recreate, err := postgresql.DropTableIndexes(ctx, c, tableID)
	assert.NoError(t, err)
	assert.NotNil(t, recreate)

	// The unique index and the primary key must survive; only the plain
	// secondary index should be dropped.
	assert.True(t, indexExists(t, c, uniqIdx))
	assert.True(t, indexExists(t, c, pkIdx))
	assert.False(t, indexExists(t, c, tagIdx))

	// And the plain secondary index round-trips as usual.
	assert.NoError(t, recreate())
	assert.True(t, indexExists(t, c, tagIdx))
	assert.True(t, indexExists(t, c, uniqIdx))
	assert.True(t, indexExists(t, c, pkIdx))
}

// TestDropTableIndexes_outerTransactionRollbackUndoesDrop verifies the documented
// contract that wrapping DropTableIndexes in a caller-managed transaction makes
// the drop participate in that transaction: rolling the outer transaction back
// restores the dropped indexes.
//
// The txkit connection collapses a BeginTx issued while a transaction is
// already active into the active one (no real nested BEGIN), so the inner
// commit DropTableIndexes performs is a no-op and the drop is only durable once
// the outer transaction commits.
func TestDropTableIndexes_outerTransactionRollbackUndoesDrop(t *testing.T) {
	c := GetConnection(t)

	table := randomName(t, "test_disable_idx_outer_tx")
	tableID := pgx.Identifier{table}

	dropTable(t, c, table)
	mustExec(t, c, fmt.Sprintf(`CREATE TABLE %s (id int PRIMARY KEY, val text)`,
		tableID.Sanitize()))

	valIdxName := table + "_val_idx"
	mustExec(t, c, fmt.Sprintf(`CREATE INDEX %s ON %s (val)`,
		pgx.Identifier{valIdxName}.Sanitize(), tableID.Sanitize()))

	valIdx := pgx.Identifier{valIdxName}
	assert.True(t, indexExists(t, c, valIdx))

	// Open a caller-managed transaction and run DropTableIndexes inside it.
	txCtx, err := c.BeginTx(t.Context())
	assert.NoError(t, err)

	recreate, err := postgresql.DropTableIndexes(txCtx, c, tableID)
	assert.NoError(t, err)
	assert.NotNil(t, recreate)

	// Within the transaction's own connection the index is already gone; the
	// drop is uncommitted, so it must be queried through txCtx to be visible.
	assert.False(t, indexExistsInTx(t, c, txCtx, valIdxName))

	// Rolling the outer transaction back must undo the drop.
	assert.NoError(t, c.RollbackTx(txCtx))
	assert.True(t, indexExists(t, c, valIdx))
}

// indexExistsInTx reports whether an index exists in the current schema, using
// the supplied (possibly transaction-bound) context so that uncommitted DDL is
// visible.
func indexExistsInTx(tb testing.TB, c postgresql.Connection, ctx context.Context, name string) bool {
	tb.Helper()
	var exists bool
	const q = `
SELECT EXISTS (
	SELECT 1 FROM pg_class c
	JOIN pg_namespace n ON n.oid = c.relnamespace
	WHERE c.relkind = 'i' AND c.relname = $1 AND n.nspname = current_schema()
)`
	assert.NoError(tb, c.QueryRowContext(ctx, q, name).Scan(&exists))
	return exists
}
