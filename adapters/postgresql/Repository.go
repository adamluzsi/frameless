package postgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/adamluzsi/frameless/pkg/lazyload"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/iterators"
)

// Repository is a frameless external resource supplier to store a certain entity type.
// The Repository supplier itself is a stateless entity.
//
// SRP: DBA
type Repository[Entity, ID any] struct {
	Mapping Mapping[Entity, ID]
	CM      ConnectionManager
}

func (r Repository[Entity, ID]) Create(ctx context.Context, ptr *Entity) (rErr error) {
	query := fmt.Sprintf("INSERT INTO %s (%s)\n", r.Mapping.TableRef(), r.queryColumnList())
	query += fmt.Sprintf("VALUES (%s)\n", r.queryColumnPlaceHolders(makePrepareStatementPlaceholderGenerator()))

	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	c, err := r.CM.Connection(ctx)
	if err != nil {
		return err
	}

	if id, ok := extid.Lookup[ID](ptr); !ok {
		// TODO: add serialize TX level here

		id, err := r.Mapping.NewID(ctx)
		if err != nil {
			return err
		}

		if err := extid.Set(ptr, id); err != nil {
			return err
		}
	} else {
		_, found, err := r.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if found {
			return errorkit.With(crud.ErrAlreadyExists).
				Detailf(`%T already exists with id: %v`, *new(Entity), id).
				Context(ctx).
				Unwrap()
		}
	}

	args, err := r.Mapping.ToArgs(ptr)
	if err != nil {
		return err
	}
	
	if _, err := c.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return nil
}

func (r Repository[Entity, ID]) FindByID(ctx context.Context, id ID) (Entity, bool, error) {
	c, err := r.CM.Connection(ctx)
	if err != nil {
		return *new(Entity), false, err
	}
	
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %q = $1`, r.queryColumnList(), r.Mapping.TableRef(), r.Mapping.IDRef())

	v, err := r.Mapping.Map(c.QueryRowContext(ctx, query, id))
	if errors.Is(err, errNoRows) {
		return *new(Entity), false, nil
	}

	if err != nil {
		return *new(Entity), false, err
	}

	return v, true, nil
}

func (r Repository[Entity, ID]) DeleteAll(ctx context.Context) (rErr error) {
	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	c, err := r.CM.Connection(ctx)
	if err != nil {
		return err
	}

	var (
		tableName = r.Mapping.TableRef()
		query     = fmt.Sprintf(`DELETE FROM %s`, tableName)
	)

	if _, err := c.ExecContext(ctx, query); err != nil {
		return err
	}

	return nil
}

func (r Repository[Entity, ID]) DeleteByID(ctx context.Context, id ID) (rErr error) {
	var query = fmt.Sprintf(`DELETE FROM %s WHERE %q = $1`, r.Mapping.TableRef(), r.Mapping.IDRef())

	ctx, err := r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	c, err := r.CM.Connection(ctx)
	if err != nil {
		return err
	}

	result, err := c.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	if count := result.RowsAffected(); count == 0 {
		return crud.ErrNotFound
	}

	return nil
}

func (r Repository[Entity, ID]) Update(ctx context.Context, ptr *Entity) (rErr error) {
	args, err := r.Mapping.ToArgs(ptr)
	if err != nil {
		return err
	}

	var (
		query           = fmt.Sprintf("UPDATE %s", r.Mapping.TableRef())
		nextPlaceHolder = makePrepareStatementPlaceholderGenerator()
		idPlaceHolder   = nextPlaceHolder()
		querySetParts   []string
	)
	for _, name := range r.Mapping.ColumnRefs() {
		querySetParts = append(querySetParts, fmt.Sprintf(`%q = %s`, name, nextPlaceHolder()))
	}
	if len(querySetParts) > 0 {
		query += fmt.Sprintf("\nSET %s", strings.Join(querySetParts, `, `))
	}
	query += fmt.Sprintf("\nWHERE %q = %s", r.Mapping.IDRef(), idPlaceHolder)

	id, ok := extid.Lookup[ID](ptr)
	if !ok {
		return fmt.Errorf(`missing entity id`)
	}

	args = append([]interface{}{id}, args...)

	ctx, err = r.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r, ctx)

	c, err := r.CM.Connection(ctx)
	if err != nil {
		return err
	}

	if res, err := c.ExecContext(ctx, query, args...); err != nil {
		return err
	} else {
		if affected := res.RowsAffected(); affected == 0 {
			return crud.ErrNotFound
		}
	}

	return nil
}

func (r Repository[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
	query := fmt.Sprintf(`SELECT %s FROM %s`, r.queryColumnList(), r.Mapping.TableRef())

	c, err := r.CM.Connection(ctx)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	rows, err := c.QueryContext(ctx, query)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	return iterators.SQLRows[Entity](rows, r.Mapping)
}

func (r Repository[Entity, ID]) FindByIDs(ctx context.Context, ids ...ID) iterators.Iterator[Entity] {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %s = ANY($1)`,
		r.queryColumnList(), r.Mapping.TableRef(), r.Mapping.IDRef())

	c, err := r.CM.Connection(ctx)
	if err != nil {
		return iterators.Error[Entity](err)
	}
	
	rows, err := c.QueryContext(ctx, query, ids)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	return &iterFindByIDs[Entity, ID]{
		Iterator:    iterators.SQLRows[Entity](rows, r.Mapping),
		expectedIDs: ids,
	}
}

type iterFindByIDs[Entity, ID any] struct {
	iterators.Iterator[Entity]
	done        bool
	expectedIDs []ID
	foundIDs    lazyload.Var[map[string]struct{}]
}

func (iter *iterFindByIDs[Entity, ID]) getFoundIDs() map[string]struct{} {
	return iter.foundIDs.Get(func() map[string]struct{} {
		return make(map[string]struct{})
	})
}

func (iter *iterFindByIDs[Entity, ID]) Err() error {
	return errorkit.Merge(iter.Iterator.Err(), iter.missingIDsErr())
}

func (iter *iterFindByIDs[Entity, ID]) missingIDsErr() error {
	if !iter.done {
		return nil
	}

	if len(iter.getFoundIDs()) == len(iter.expectedIDs) {
		return nil
	}

	var missing []ID
	for _, id := range iter.expectedIDs {
		if _, ok := iter.getFoundIDs()[iter.idFoundKey(id)]; !ok {
			missing = append(missing, id)
		}
	}

	return fmt.Errorf("not all ID is retrieved by FindByIDs: %#v", missing)
}

func (iter *iterFindByIDs[Entity, ID]) Next() bool {
	gotNext := iter.Iterator.Next()
	if gotNext {
		id, _ := extid.Lookup[ID](iter.Iterator.Value())
		iter.getFoundIDs()[iter.idFoundKey(id)] = struct{}{}
	}
	if !gotNext {
		iter.done = true
	}
	return gotNext
}

func (iter *iterFindByIDs[Entity, ID]) idFoundKey(id ID) string {
	return fmt.Sprintf("%v", id)
}

func (r Repository[Entity, ID]) Upsert(ctx context.Context, ptrs ...*Entity) (rErr error) {
	var (
		ptrWithID    []*Entity
		ptrWithoutID []*Entity
	)
	for _, ptr := range ptrs {
		id, _ := extid.Lookup[ID](ptr)
		if any(id) == any(*new(ID)) {
			ptrWithoutID = append(ptrWithoutID, ptr)
		} else {
			ptrWithID = append(ptrWithID, ptr)
		}
	}

	ctx, err := r.CM.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r.CM, ctx)
	return errorkit.Merge(r.upsertWithID(ctx, ptrWithID...), r.upsertWithoutID(ctx, ptrWithoutID...))
}

func (r Repository[Entity, ID]) upsertWithoutID(ctx context.Context, ptrs ...*Entity) error {
	for _, ptr := range ptrs {
		if err := r.Create(ctx, ptr); err != nil {
			return err
		}
	}
	return nil
}
func (r Repository[Entity, ID]) upsertWithID(ctx context.Context, ptrs ...*Entity) error {
	if len(ptrs) == 0 {
		return nil
	}

	var (
		query  string
		args   []any
		nextPH = makePrepareStatementPlaceholderGenerator()
	)
	query += fmt.Sprintf("INSERT INTO %s (%s)\n", r.Mapping.TableRef(), r.queryColumnList())
	query += "VALUES \n"

	for i, ptr := range ptrs {
		separator := ","
		if i == len(ptrs)-1 { // on last element
			separator = ""
		}

		query += fmt.Sprintf("\t(%s)%s\n", r.queryColumnPlaceHolders(nextPH), separator)

		vs, err := r.Mapping.ToArgs(ptr)
		if err != nil {
			return err
		}
		args = append(args, vs...)
	}

	query += fmt.Sprintf("ON CONFLICT (%s) DO\n", r.Mapping.IDRef())
	query += "\tUPDATE SET\n"

	columns := r.Mapping.ColumnRefs()
	for i, col := range columns {
		sep := ","
		if i == len(columns)-1 { // on last element
			sep = ""
		}

		query += fmt.Sprintf("\t\t%s = EXCLUDED.%s%s\n", col, col, sep)
	}

	c, err := r.CM.Connection(ctx)
	if err != nil {
		return err
	}

	if _, err := c.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return nil
}

func (r Repository[Entity, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return r.CM.BeginTx(ctx)
}

func (r Repository[Entity, ID]) CommitTx(ctx context.Context) error {
	return r.CM.CommitTx(ctx)
}

func (r Repository[Entity, ID]) RollbackTx(ctx context.Context) error {
	return r.CM.RollbackTx(ctx)
}

func (r Repository[Entity, ID]) queryColumnPlaceHolders(nextPlaceholder func() string) string {
	var phs []string
	for range r.Mapping.ColumnRefs() {
		phs = append(phs, nextPlaceholder())
	}
	return strings.Join(phs, `, `)
}

func (r Repository[Entity, ID]) queryColumnList() string {
	var (
		src = r.Mapping.ColumnRefs()
		dst = make([]string, 0, len(src))
	)
	for _, name := range src {
		// TODO: replace with the commented out version
		// dst = append(dst, fmt.Sprintf(`%q`, name))
		dst = append(dst, fmt.Sprintf(`%s`, name))
	}
	return strings.Join(dst, `, `)
}
