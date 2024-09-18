package postgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/iterators"
)

// Repository is a frameless external resource supplier to store a certain entity type.
// The Repository supplier itself is a stateless entity.
//
// SRP: DBA
type Repository[ENT, ID any] struct {
	Connection Connection
	Mapping    flsql.Mapping[ENT, ID]
}

func (r Repository[ENT, ID]) Create(ctx context.Context, ptr *ENT) (rErr error) {
	if ptr == nil {
		return fmt.Errorf("nil entity pointer given to Create")
	}

	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	id, ok := r.Mapping.ID.Lookup(*ptr)
	if ok {
		_, found, err := r.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if found {
			return errorkit.With(crud.ErrAlreadyExists).
				Detailf(`%T already exists with id: %v`, *new(ENT), id).
				Context(ctx).
				Unwrap()
		}
	}

	// TODO: add serialize TX level here
	if err := r.Mapping.OnCreate(ctx, ptr); err != nil {
		return err
	}

	args, err := r.Mapping.ToArgs(*ptr)
	if err != nil {
		return err
	}

	var (
		colums       []flsql.ColumnName
		valuesClause []string
		valuesArgs   []any
		nextPH       = makePrepareStatementPlaceholderGenerator()
	)
	for col, arg := range args {
		colums = append(colums, col)
		valuesClause = append(valuesClause, nextPH())
		valuesArgs = append(valuesArgs, arg)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s)\n", r.Mapping.TableName, r.quotedColumnsClause(colums))
	query += fmt.Sprintf("VALUES (%s)\n", strings.Join(valuesClause, ", "))

	logger.Debug(ctx, "postgresql.Repository.Create", logging.Field("query", query))

	if _, err := r.Connection.ExecContext(ctx, query, valuesArgs...); err != nil {
		return err
	}

	return nil
}

func (r Repository[ENT, ID]) idQuery(id ID, nextPlaceholder func() string) (whereClause []string, queryArgs []any, _ error) {
	idArgs, err := r.Mapping.QueryID(id)
	if err != nil {
		return nil, nil, err
	}
	for col, arg := range idArgs {
		whereClause = append(whereClause, fmt.Sprintf("%q = %s", col, nextPlaceholder()))
		queryArgs = append(queryArgs, arg)
	}
	return whereClause, queryArgs, nil
}

func (r Repository[ENT, ID]) FindByID(ctx context.Context, id ID) (ENT, bool, error) {
	idArgs, err := r.Mapping.QueryID(id)
	if err != nil {
		return *new(ENT), false, fmt.Errorf("QueryID: %w", err)
	}

	cols, scan := r.Mapping.ToQuery(ctx)

	query := fmt.Sprintf(`SELECT %s FROM %s`, r.quotedColumnsClause(cols), r.Mapping.TableName)

	nextPH := makePrepareStatementPlaceholderGenerator()

	var (
		whereClause []string
		queryArgs   []any
	)
	for col, arg := range idArgs {
		whereClause = append(whereClause, fmt.Sprintf("%s = %s", col, nextPH()))
		queryArgs = append(queryArgs, arg)
	}

	query += " WHERE " + strings.Join(whereClause, " AND ")

	logger.Debug(ctx, "postgresql.Repository#FindByID", logging.Field("query", query))

	row := r.Connection.QueryRowContext(ctx, query, queryArgs...)

	var v ENT
	err = scan(&v, row)

	if errors.Is(err, errNoRows) {
		return *new(ENT), false, nil
	}

	if err != nil {
		return *new(ENT), false, err
	}

	return v, true, nil
}

func (r Repository[ENT, ID]) DeleteAll(ctx context.Context) (rErr error) {
	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	var (
		tableName = r.Mapping.TableName
		query     = fmt.Sprintf(`DELETE FROM %s`, tableName)
	)

	if _, err := r.Connection.ExecContext(ctx, query); err != nil {
		return err
	}

	return nil
}

func (r Repository[ENT, ID]) DeleteByID(ctx context.Context, id ID) (rErr error) {
	idWhereClause, idQueryArgs, err := r.idQuery(id, makePrepareStatementPlaceholderGenerator())
	if err != nil {
		return err
	}

	var query = fmt.Sprintf(`DELETE FROM %s WHERE %s`, r.Mapping.TableName, strings.Join(idWhereClause, " AND "))

	ctx, err = r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	logger.Debug(ctx, "postgresql.Repository#DeleteByID", logging.Field("query", query))

	result, err := r.Connection.ExecContext(ctx, query, idQueryArgs...)
	if err != nil {
		return err
	}

	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count == 0 {
		return crud.ErrNotFound
	}

	return nil
}

func (r Repository[ENT, ID]) Update(ctx context.Context, ptr *ENT) (rErr error) {
	if ptr == nil {
		return fmt.Errorf("Update: nil entity pointer received")
	}

	var (
		query           = fmt.Sprintf("UPDATE %s", r.Mapping.TableName)
		nextPlaceholder = makePrepareStatementPlaceholderGenerator()

		querySetClause   []string
		queryWhereClause []string
		queryArgs        []any
	)

	id, ok := r.Mapping.ID.Lookup(*ptr)
	if !ok {
		return fmt.Errorf("missing entity id for Update")
	}

	idWhere, idArgs, err := r.idQuery(id, nextPlaceholder)
	if err != nil {
		return err
	}
	queryWhereClause = append(queryWhereClause, idWhere...)
	queryArgs = append(queryArgs, idArgs...)

	setArgs, err := r.Mapping.ToArgs(*ptr)
	if err != nil {
		return err
	}

	for col, arg := range setArgs {
		querySetClause = append(querySetClause, fmt.Sprintf(`%q = %s`, col, nextPlaceholder()))
		queryArgs = append(queryArgs, arg)
	}

	if len(querySetClause) > 0 {
		query += fmt.Sprintf("\nSET %s", strings.Join(querySetClause, `, `))
	}

	query += fmt.Sprintf("\nWHERE %s", strings.Join(queryWhereClause, ", "))

	ctx, err = r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	if res, err := r.Connection.ExecContext(ctx, query, queryArgs...); err != nil {
		return err
	} else {
		if affected, err := res.RowsAffected(); err != nil {
			return err
		} else if affected == 0 {
			return crud.ErrNotFound
		}
	}

	return nil
}

func (r Repository[ENT, ID]) FindAll(ctx context.Context) (iterators.Iterator[ENT], error) {
	cols, scan := r.Mapping.ToQuery(ctx)
	query := fmt.Sprintf(`SELECT %s FROM %s`, r.quotedColumnsClause(cols), r.Mapping.TableName)

	rows, err := r.Connection.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}

	return flsql.MakeSQLRowsIterator[ENT](rows, scan), nil
}

func (r Repository[ENT, ID]) FindByIDs(ctx context.Context, ids ...ID) (iterators.Iterator[ENT], error) {
	if len(ids) == 0 {
		return iterators.Empty[ENT](), nil
	}

	var (
		whereClause []string
		queryArgs   []any
	)

	nextPlaceholder := makePrepareStatementPlaceholderGenerator()
	for _, id := range ids {
		idWhere, idArgs, err := r.idQuery(id, nextPlaceholder)
		if err != nil {
			return nil, err
		}
		whereClause = append(whereClause, fmt.Sprintf("(%s)", strings.Join(idWhere, " AND ")))
		queryArgs = append(queryArgs, idArgs...)
	}

	selectClause, scan := r.Mapping.ToQuery(ctx)

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s`,
		r.quotedColumnsClause(selectClause), r.Mapping.TableName, strings.Join(whereClause, " OR "))

	var count int
	coundQuery := fmt.Sprintf(`SELECT COUNT(*) FROM (%s) AS src`, query)
	if err := r.Connection.QueryRowContext(ctx, coundQuery, queryArgs...).Scan(&count); err != nil {
		return nil, err
	}
	if count != len(ids) {
		return nil, crud.ErrNotFound
	}

	rows, err := r.Connection.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}

	return flsql.MakeSQLRowsIterator[ENT](rows, scan), nil
}

// Upsert
//
// DEPRECATED: use Repository.Save instead
func (r Repository[ENT, ID]) Upsert(ctx context.Context, ptrs ...*ENT) (rErr error) {
	var (
		ptrWithID    []*ENT
		ptrWithoutID []*ENT
	)
	for _, ptr := range ptrs {
		id, _ := extid.Lookup[ID](ptr)
		if any(id) == any(*new(ID)) {
			ptrWithoutID = append(ptrWithoutID, ptr)
		} else {
			ptrWithID = append(ptrWithID, ptr)
		}
	}

	ctx, err := r.Connection.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r.Connection, ctx)
	return errorkit.Merge(r.upsertWithID(ctx, ptrWithID...), r.upsertWithoutID(ctx, ptrWithoutID...))
}

func (r Repository[ENT, ID]) Save(ctx context.Context, ptr *ENT) (rErr error) {
	if ptr == nil {
		return fmt.Errorf("nil %T received", ptr)
	}
	if _, ok := r.Mapping.ID.Lookup(*ptr); ok {
		return r.upsertWithID(ctx, ptr)
	}
	return r.upsertWithoutID(ctx, ptr)
}

func (r Repository[ENT, ID]) upsertWithoutID(ctx context.Context, ptrs ...*ENT) error {
	for _, ptr := range ptrs {
		if err := r.Create(ctx, ptr); err != nil {
			return err
		}
	}
	return nil
}
func (r Repository[ENT, ID]) upsertWithID(ctx context.Context, ptrs ...*ENT) error {
	if len(ptrs) == 0 {
		return nil
	}

	var args []any

	nextPH := makePrepareStatementPlaceholderGenerator()

	var valuesElems []flsql.QueryArgs
	for _, ptr := range ptrs {
		valueElem, err := r.Mapping.ToArgs(*ptr)
		if err != nil {
			return err
		}
		valuesElems = append(valuesElems, valueElem)
	}

	var columns []flsql.ColumnName
	for _, value := range valuesElems {
		columns = slicekit.Unique(append(columns, mapkit.Keys(value)...))
	}

	var idColumns []flsql.ColumnName
	var valuesClause []string
	for _, ptr := range ptrs {

		id, _ := r.Mapping.ID.Lookup(*ptr)

		idArgs, err := r.Mapping.QueryID(id)
		if err != nil {
			return err
		}

		idColumns = slicekit.Unique(append(idColumns, mapkit.Keys(idArgs)...))

		setClauseArgs, err := r.Mapping.ToArgs(*ptr)
		if err != nil {
			return err
		}

		var valueClause []string
		for _, col := range columns {
			valueClause = append(valueClause, nextPH())
			// on no value, it will be NULL which is the expected behaviour
			// so no need to do a `arg, ok := setClauseArgs[col]`
			args = append(args, setClauseArgs[col])
		}

		valuesClause = append(valuesClause, fmt.Sprintf("(%s)", strings.Join(valueClause, ", ")))
	}

	var onConflictUpdateSetClause []string
	for _, col := range columns {
		onConflictUpdateSetClause = append(onConflictUpdateSetClause,
			fmt.Sprintf("\t\t%s = EXCLUDED.%s", col, col))
	}

	var query string
	query += fmt.Sprintf("INSERT INTO %s (%s)\n", r.Mapping.TableName, r.quotedColumnsClause(columns))
	query += fmt.Sprintf("VALUES \n\t%s\n", strings.Join(valuesClause, ",\n\t"))
	query += fmt.Sprintf("ON CONFLICT (%s) DO\n", flsql.JoinColumnName(idColumns, "%q", ", "))
	query += fmt.Sprintf("\tUPDATE SET\n%s\n", strings.Join(onConflictUpdateSetClause, ",\n"))

	if _, err := r.Connection.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return nil
}

func (r Repository[ENT, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return r.Connection.BeginTx(ctx)
}

func (r Repository[ENT, ID]) CommitTx(ctx context.Context) error {
	return r.Connection.CommitTx(ctx)
}

func (r Repository[ENT, ID]) RollbackTx(ctx context.Context) error {
	return r.Connection.RollbackTx(ctx)
}

func (r Repository[ENT, ID]) quotedColumnsClause(cols []flsql.ColumnName) string {
	return flsql.JoinColumnName(cols, "%q", ", ")
}
