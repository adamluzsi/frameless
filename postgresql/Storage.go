package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"strings"

	"github.com/adamluzsi/frameless/reflects"

	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
)

func NewStorageByDSN[Ent, ID any](m Mapping[Ent], dsn string) *Storage {
	cm := NewConnectionManager(dsn)
	sm := NewListenNotifySubscriptionManager[Ent](m, dsn, cm)
	return &Storage[Ent, ID]{
		Mapping:             m,
		ConnectionManager:   cm,
		SubscriptionManager: sm,
	}
}

// Storage is a frameless external resource supplier to store a certain entity type.
// The Storage supplier itself is a stateless entity.
//
//
// SRP: DBA
type Storage[Ent, ID any] struct {
	Mapping             Mapping[Ent]
	ConnectionManager   ConnectionManager
	SubscriptionManager SubscriptionManager
	MetaAccessor
}

func (pg *Storage[Ent, ID]) Close() error {
	cls := func(c io.Closer) error {
		if c == nil {
			return nil
		}
		return c.Close()
	}
	cmErr := cls(pg.ConnectionManager)
	smErr := cls(pg.SubscriptionManager)
	if cmErr != nil {
		return cmErr
	}
	if smErr != nil {
		return smErr
	}
	return nil
}

func (pg *Storage[Ent, ID]) Create(ctx context.Context, ptr interface{}) (rErr error) {
	query := fmt.Sprintf("INSERT INTO %s (%s)\n", pg.Mapping.TableRef(), pg.queryColumnList())
	query += fmt.Sprintf("VALUES (%s)\n", pg.queryColumnPlaceHolders(pg.newPrepareStatementPlaceholderGenerator()))

	ctx, err := pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer frameless.FinishOnePhaseCommit(&rErr, pg, ctx)

	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}

	if _, ok := extid.Lookup(ptr); !ok {
		// TODO: add serialize TX level here

		id, err := pg.Mapping.NewID(ctx)
		if err != nil {
			return err
		}

		if err := extid.Set(ptr, id); err != nil {
			return err
		}
	}

	args, err := pg.Mapping.ToArgs(ptr)
	if err != nil {
		return err
	}
	if _, err := c.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return pg.SubscriptionManager.PublishCreateEvent(ctx, frameless.CreateEvent{Entity: base(ptr)})
}

func (pg *Storage[Ent, ID]) FindByID(ctx context.Context, ptr, id interface{}) (bool, error) {
	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return false, err
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %q = $1`, pg.queryColumnList(), pg.Mapping.TableRef(), pg.Mapping.IDRef())

	err = pg.Mapping.Map(c.QueryRowContext(ctx, query, id), ptr)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (pg *Storage[Ent, ID]) DeleteAll(ctx context.Context) (rErr error) {
	ctx, err := pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer frameless.FinishOnePhaseCommit(&rErr, pg, ctx)

	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}

	var (
		tableName = pg.Mapping.TableRef()
		query     = fmt.Sprintf(`DELETE FROM %s`, tableName)
	)

	if _, err := c.ExecContext(ctx, query); err != nil {
		return err
	}

	if err := pg.SubscriptionManager.PublishDeleteAllEvent(ctx, frameless.DeleteAllEvent{}); err != nil {
		return err
	}

	return nil
}

func (pg *Storage[Ent, ID]) DeleteByID(ctx context.Context, id interface{}) (rErr error) {
	var query = fmt.Sprintf(`DELETE FROM %s WHERE "id" = $1`, pg.Mapping.TableRef())

	ctx, err := pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer frameless.FinishOnePhaseCommit(&rErr, pg, ctx)

	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}

	result, err := c.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}

	count, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if count == 0 {
		return fmt.Errorf(`ErrNotFound`)
	}

	if err := pg.SubscriptionManager.PublishDeleteByIDEvent(ctx, frameless.DeleteByIDEvent{ID: id}); err != nil {
		return err
	}

	return nil
}

func (pg *Storage[Ent, ID]) newPrepareStatementPlaceholderGenerator() func() string {
	var index = 0
	return func() string {
		index++
		return fmt.Sprintf(`$%d`, index)
	}
}

func (pg *Storage[Ent, ID]) Update(ctx context.Context, ptr interface{}) (rErr error) {
	args, err := pg.Mapping.ToArgs(ptr)
	if err != nil {
		return err
	}

	var (
		query           = fmt.Sprintf("UPDATE %s", pg.Mapping.TableRef())
		nextPlaceHolder = pg.newPrepareStatementPlaceholderGenerator()
		idPlaceHolder   = nextPlaceHolder()
		querySetParts   []string
	)
	for _, name := range pg.Mapping.ColumnRefs() {
		querySetParts = append(querySetParts, fmt.Sprintf(`%q = %s`, name, nextPlaceHolder()))
	}
	if len(querySetParts) > 0 {
		query += fmt.Sprintf("\nSET %s", strings.Join(querySetParts, `, `))
	}
	query += fmt.Sprintf("\nWHERE %q = %s", pg.Mapping.IDRef(), idPlaceHolder)

	id, ok := extid.Lookup(ptr)
	if !ok {
		return fmt.Errorf(`missing entity id`)
	}

	args = append([]interface{}{id}, args...)

	ctx, err = pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer frameless.FinishOnePhaseCommit(&rErr, pg, ctx)

	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}

	if res, err := c.ExecContext(ctx, query, args...); err != nil {
		return err
	} else {
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return fmt.Errorf(`deployment environment not found`)
		}
	}

	return pg.SubscriptionManager.PublishUpdateEvent(ctx, frameless.UpdateEvent{Entity: base(ptr)})
}

func (pg *Storage[Ent, ID]) FindAll(ctx context.Context) frameless.Iterator {
	query := fmt.Sprintf(`SELECT %s FROM %s`, pg.queryColumnList(), pg.Mapping.TableRef())

	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return iterators.NewError(err)
	}

	rows, err := c.QueryContext(ctx, query)
	if err != nil {
		return iterators.NewError(err)
	}

	return iterators.NewSQLRows(rows, pg.Mapping)
}

func (pg *Storage[Ent, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return pg.ConnectionManager.BeginTx(ctx)
}

func (pg *Storage[Ent, ID]) CommitTx(ctx context.Context) error {
	return pg.ConnectionManager.CommitTx(ctx)
}

func (pg *Storage[Ent, ID]) RollbackTx(ctx context.Context) error {
	return pg.ConnectionManager.RollbackTx(ctx)
}

func (pg *Storage[Ent, ID]) queryColumnPlaceHolders(nextPlaceholder func() string) string {
	var phs []string
	for range pg.Mapping.ColumnRefs() {
		phs = append(phs, nextPlaceholder())
	}
	return strings.Join(phs, `, `)
}

func (pg *Storage[Ent, ID]) queryColumnList() string {
	var (
		src = pg.Mapping.ColumnRefs()
		dst = make([]string, 0, len(src))
	)
	for _, name := range src {
		dst = append(dst, fmt.Sprintf(`%s`, name))
	}
	return strings.Join(dst, `, `)
}

func base(ptr interface{}) interface{} {
	return reflects.BaseValueOf(ptr).Interface()
}

func (pg *Storage[Ent, ID]) SubscribeToCreatorEvents(ctx context.Context, s frameless.CreatorSubscriber) (frameless.Subscription, error) {
	return pg.SubscriptionManager.SubscribeToCreatorEvents(ctx, s)
}

func (pg *Storage[Ent, ID]) SubscribeToUpdaterEvents(ctx context.Context, s frameless.UpdaterSubscriber) (frameless.Subscription, error) {
	return pg.SubscriptionManager.SubscribeToUpdaterEvents(ctx, s)
}

func (pg *Storage[Ent, ID]) SubscribeToDeleterEvents(ctx context.Context, s frameless.DeleterSubscriber) (frameless.Subscription, error) {
	return pg.SubscriptionManager.SubscribeToDeleterEvents(ctx, s)
}
