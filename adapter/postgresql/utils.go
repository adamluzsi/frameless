package postgresql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/comproto"
)

func SplitSchemaTable(id pgx.Identifier) (schema, table pgx.Identifier) {
	if len(id) == 0 {
		return nil, nil
	}
	if len(id) == 1 {
		return nil, id
	}
	return id[0:1], id[1:]
}

// queryTableSecondaryIndexDefs lists the plain secondary indexes of a relation
// along with their full CREATE INDEX definition. The relation is matched by
// name ($1) within a schema ($2); when $2 is NULL the current schema
// (search_path) is used.
//
// "Plain" means: skip primary, unique, and any constraint-owned index. The last
// category matters for exclusion constraints, whose backing index is neither
// unique nor primary: PostgreSQL forbids DROP INDEX on a constraint-owned index
// ("... because constraint ... requires it"), and even if it were droppable,
// removing it would stop enforcing the constraint during the load. Constraint
// ownership is detected via an internal pg_depend dependency (deptype = 'i').
const queryTableSecondaryIndexDefs = `
SELECT c.relname, pg_catalog.pg_get_indexdef(x.indexrelid)
FROM pg_index x
JOIN pg_class c ON c.oid = x.indexrelid
JOIN pg_class t ON t.oid = x.indrelid
JOIN pg_namespace n ON n.oid = t.relnamespace
WHERE t.relname = $1
  AND n.nspname = COALESCE($2, current_schema())
  AND NOT x.indisunique
  AND NOT x.indisprimary
  AND NOT EXISTS (
    SELECT 1 FROM pg_depend d
    WHERE d.classid = 'pg_class'::regclass
      AND d.objid   = x.indexrelid
      AND d.deptype = 'i'
  )
`

// DropTableIndexes drops every non-unique, non-primary index on the given table
// and returns a function that recreates them from their original definitions.
//
// It is meant to speed up bulk loads: drop the secondary indexes, load the
// data, then call the returned function to rebuild each index in a single pass
// (much cheaper than maintaining them row-by-row during the load). Primary,
// unique, and other constraint-owned indexes (e.g. those backing exclusion
// constraints) are left in place because they back constraints.
//
// The drop and the rebuild each run in their own transaction, so each is atomic
// on its own: either all secondary indexes are dropped or none, and likewise
// for the rebuild. They are NOT atomic with respect to each other; to make the
// whole drop/load/rebuild sequence atomic, begin a transaction on the
// connection before calling DropTableIndexes and commit it after the rebuild.
//
// The returned recreate function must be called at most once: the recreated
// CREATE INDEX statements have no IF NOT EXISTS clause, so a second call (or a
// call after the indexes already exist) fails with "relation already exists".
func DropTableIndexes(ctx context.Context, c Connection, table pgx.Identifier) (_restore func() error, rErr error) {
	tx, err := c.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, c, tx)

	schemaID, tableID := SplitSchemaTable(table)

	if len(tableID) == 0 {
		return nil, fmt.Errorf("unable to determine table identifier: %#v", table)
	}

	// When the identifier carries a schema, scope the lookup to it; otherwise
	// pass SQL NULL (not an empty string) so the query falls back to
	// current_schema() via COALESCE.
	var schemaArg sql.NullString
	if schemaID != nil && len(schemaID) == 1 {
		schemaArg.String = schemaID[0]
		schemaArg.Valid = true
	}

	type indexDef struct {
		Name       string
		Definition string
	}

	scan := func(s flsql.Scanner) (indexDef, error) {
		var ix indexDef
		err := s.Scan(&ix.Name, &ix.Definition)
		return ix, err
	}

	// Capture the definitions first, both to drain the cursor before issuing DDL
	// and so the indexes can be recreated later.
	var indexes []indexDef
	for ix, err := range flsql.QueryMany(c, tx, scan, queryTableSecondaryIndexDefs, tableID[0], schemaArg) {
		if err != nil {
			return nil, fmt.Errorf("failed to list indexes for %s: %w", table.Sanitize(), err)
		}
		indexes = append(indexes, ix)
	}

	for _, ix := range indexes {
		indexIdent := pgx.Identifier{ix.Name}
		if schemaID != nil {
			indexIdent = slicekit.Merge(schemaID, pgx.Identifier{ix.Name})
		}
		logger.Debug(tx, "dropping index", logging.Field("index", ix.Name))
		dropQuery := fmt.Sprintf(`DROP INDEX IF EXISTS %s`, indexIdent.Sanitize())
		if _, err := c.ExecContext(tx, dropQuery); err != nil {
			return nil, fmt.Errorf("failed to drop index %s: %w", ix.Name, err)
		}
	}

	return func() (rErr error) {
		tx, err := c.BeginTx(ctx)
		if err != nil {
			return err
		}
		defer comproto.FinishOnePhaseCommit(&rErr, c, tx)

		for _, ix := range indexes {
			logger.Debug(tx, "recreating index", logging.Field("index", ix.Name))
			// pg_get_indexdef returns a fully schema-qualified CREATE INDEX
			// statement, so it can be executed as-is.
			if _, err := c.ExecContext(tx, ix.Definition); err != nil {
				return fmt.Errorf("failed to recreate index %s: %w", ix.Name, err)
			}
		}
		return nil
	}, nil
}
