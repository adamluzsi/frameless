package postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/ports/crud"
	"io"
	"strings"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/pubsub"

	"github.com/adamluzsi/frameless/ports/crud/extid"

	"github.com/adamluzsi/frameless/ports/iterators"
)

func NewRepositoryWithDSN[Entity, ID any](dsn string, m Mapping[Entity]) *Repository[Entity, ID] {
	cm := NewConnectionManager(dsn)
	sm := NewListenNotifySubscriptionManager[Entity, ID](m, dsn, cm)
	return &Repository[Entity, ID]{
		Mapping:             m,
		ConnectionManager:   cm,
		SubscriptionManager: sm,
	}
}

// Repository is a frameless external resource supplier to store a certain entity type.
// The Repository supplier itself is a stateless entity.
//
// SRP: DBA
type Repository[Entity, ID any] struct {
	Mapping             Mapping[Entity]
	ConnectionManager   ConnectionManager
	SubscriptionManager SubscriptionManager[Entity, ID]
	MetaAccessor
}

func (pg *Repository[Entity, ID]) Close() error {
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

func (pg *Repository[Entity, ID]) Create(ctx context.Context, ptr *Entity) (rErr error) {
	query := fmt.Sprintf("INSERT INTO %s (%s)\n", pg.Mapping.TableRef(), pg.queryColumnList())
	query += fmt.Sprintf("VALUES (%s)\n", pg.queryColumnPlaceHolders(pg.newPrepareStatementPlaceholderGenerator()))

	ctx, err := pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, pg, ctx)

	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return err
	}

	if id, ok := extid.Lookup[ID](ptr); !ok {
		// TODO: add serialize TX level here

		id, err := pg.Mapping.NewID(ctx)
		if err != nil {
			return err
		}

		if err := extid.Set(ptr, id); err != nil {
			return err
		}
	} else {
		_, found, err := pg.FindByID(ctx, id)
		if err != nil {
			return err
		}
		if found {
			return errorutil.With(crud.ErrAlreadyExists).
				Detailf(`%T already exists with id: %v`, *new(Entity), id).
				Context(ctx).
				Unwrap()
		}
	}

	args, err := pg.Mapping.ToArgs(ptr)
	if err != nil {
		return err
	}
	if _, err := c.ExecContext(ctx, query, args...); err != nil {
		return err
	}

	return pg.SubscriptionManager.PublishCreateEvent(ctx, pubsub.CreateEvent[Entity]{Entity: *ptr})
}

func (pg *Repository[Entity, ID]) FindByID(ctx context.Context, id ID) (Entity, bool, error) {
	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return *new(Entity), false, err
	}

	query := fmt.Sprintf(`SELECT %s FROM %s WHERE %q = $1`, pg.queryColumnList(), pg.Mapping.TableRef(), pg.Mapping.IDRef())

	v, err := pg.Mapping.Map(c.QueryRowContext(ctx, query, id))
	if err == sql.ErrNoRows {
		return *new(Entity), false, nil
	}
	if err != nil {
		return *new(Entity), false, err
	}

	return v, true, nil
}

func (pg *Repository[Entity, ID]) DeleteAll(ctx context.Context) (rErr error) {
	ctx, err := pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, pg, ctx)

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

	if err := pg.SubscriptionManager.PublishDeleteAllEvent(ctx, pubsub.DeleteAllEvent{}); err != nil {
		return err
	}

	return nil
}

func (pg *Repository[Entity, ID]) DeleteByID(ctx context.Context, id ID) (rErr error) {
	var query = fmt.Sprintf(`DELETE FROM %s WHERE "id" = $1`, pg.Mapping.TableRef())

	ctx, err := pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, pg, ctx)

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
		return crud.ErrNotFound
	}

	if err := pg.SubscriptionManager.PublishDeleteByIDEvent(ctx, pubsub.DeleteByIDEvent[ID]{ID: id}); err != nil {
		return err
	}

	return nil
}

func (pg *Repository[Entity, ID]) newPrepareStatementPlaceholderGenerator() func() string {
	var index = 0
	return func() string {
		index++
		return fmt.Sprintf(`$%d`, index)
	}
}

func (pg *Repository[Entity, ID]) Update(ctx context.Context, ptr *Entity) (rErr error) {
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

	id, ok := extid.Lookup[ID](ptr)
	if !ok {
		return fmt.Errorf(`missing entity id`)
	}

	args = append([]interface{}{id}, args...)

	ctx, err = pg.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, pg, ctx)

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
			return crud.ErrNotFound
		}
	}

	return pg.SubscriptionManager.PublishUpdateEvent(ctx, pubsub.UpdateEvent[Entity]{Entity: *ptr})
}

func (pg *Repository[Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
	query := fmt.Sprintf(`SELECT %s FROM %s`, pg.queryColumnList(), pg.Mapping.TableRef())

	c, err := pg.ConnectionManager.Connection(ctx)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	rows, err := c.QueryContext(ctx, query)
	if err != nil {
		return iterators.Error[Entity](err)
	}

	return iterators.SQLRows[Entity](rows, pg.Mapping)
}

func (pg *Repository[Entity, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return pg.ConnectionManager.BeginTx(ctx)
}

func (pg *Repository[Entity, ID]) CommitTx(ctx context.Context) error {
	return pg.ConnectionManager.CommitTx(ctx)
}

func (pg *Repository[Entity, ID]) RollbackTx(ctx context.Context) error {
	return pg.ConnectionManager.RollbackTx(ctx)
}

func (pg *Repository[Entity, ID]) queryColumnPlaceHolders(nextPlaceholder func() string) string {
	var phs []string
	for range pg.Mapping.ColumnRefs() {
		phs = append(phs, nextPlaceholder())
	}
	return strings.Join(phs, `, `)
}

func (pg *Repository[Entity, ID]) queryColumnList() string {
	var (
		src = pg.Mapping.ColumnRefs()
		dst = make([]string, 0, len(src))
	)
	for _, name := range src {
		// TODO: replace with the commented out version
		// dst = append(dst, fmt.Sprintf(`%q`, name))
		dst = append(dst, fmt.Sprintf(`%s`, name))
	}
	return strings.Join(dst, `, `)
}

func (pg *Repository[Entity, ID]) SubscribeToCreatorEvents(ctx context.Context, s pubsub.CreatorSubscriber[Entity]) (pubsub.Subscription, error) {
	return pg.SubscriptionManager.SubscribeToCreatorEvents(ctx, s)
}

func (pg *Repository[Entity, ID]) SubscribeToUpdaterEvents(ctx context.Context, s pubsub.UpdaterSubscriber[Entity]) (pubsub.Subscription, error) {
	return pg.SubscriptionManager.SubscribeToUpdaterEvents(ctx, s)
}

func (pg *Repository[Entity, ID]) SubscribeToDeleterEvents(ctx context.Context, s pubsub.DeleterSubscriber[ID]) (pubsub.Subscription, error) {
	return pg.SubscriptionManager.SubscribeToDeleterEvents(ctx, s)
}
