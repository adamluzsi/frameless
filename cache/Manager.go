package cache

import (
	"context"
	"fmt"
	"sync"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
)

func NewManager[Ent, ID any](storage Storage[Ent, ID], source Source[Ent, ID]) (*Manager[Ent, ID], error) {
	var r = &Manager[Ent, ID]{Storage: storage, Source: source}
	return r, r.Init(context.Background())
}

type Manager[Ent, ID any] struct {
	// Source is the location of the original data
	Source Source[Ent, ID]
	// Storage is the storage that keeps the cached data.
	Storage Storage[Ent, ID]

	init  sync.Once
	close func()
}

// Source is the minimum expected interface that is expected from a Source resources that needs caching.
// On top of this, cache.Manager also supports Updater, CreatorPublisher, UpdaterPublisher and DeleterPublisher.
type Source[Ent, ID any] interface {
	frameless.Creator[Ent]
	frameless.Finder[Ent, ID]
	frameless.Deleter[ID]
	frameless.CreatorPublisher[Ent]
	frameless.DeleterPublisher[ID]
}

type ExtendedSource[Ent, ID any] interface {
	frameless.Updater[Ent]
	frameless.UpdaterPublisher[Ent]
}

func (m *Manager[Ent, ID]) Init(ctx context.Context) error {
	var rErr error
	m.init.Do(func() { rErr = m.subscribe(ctx) })
	return rErr
}

func (m *Manager[Ent, ID]) trap(next func()) {
	if m.close == nil {
		m.close = func() {}
	}

	prev := m.close
	m.close = func() {
		prev()
		next()
	}
}

func (m *Manager[Ent, ID]) Close() error {
	if m.close == nil {
		return nil
	}
	m.close()
	return nil
}

func (m *Manager[Ent, ID]) entityTypeName() string {
	return reflects.SymbolicName(*new(Ent))
}

func (m *Manager[Ent, ID]) deleteCachedEntity(ctx context.Context, id ID) (rErr error) {
	s := m.Storage.CacheEntity(ctx)
	ctx, err := m.Storage.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if rErr != nil {
			_ = m.Storage.RollbackTx(ctx)
			return
		}
		rErr = m.Storage.CommitTx(ctx)
	}()
	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return s.DeleteByID(ctx, id)
}

func (m *Manager[Ent, ID]) CacheQueryMany(
	ctx context.Context,
	name string,
	query func() frameless.Iterator[Ent],
) frameless.Iterator[Ent] {
	// TODO: double check
	if ctx != nil && ctx.Err() != nil {
		return iterators.Error[Ent](ctx.Err())
	}

	queryID := fmt.Sprintf(`0:%T/%s`, *new(Ent), name) // add version epoch
	hit, found, err := m.Storage.CacheHit(ctx).FindByID(ctx, queryID)
	if err != nil {
		return iterators.Error[Ent](err)
	}
	if found {
		// TODO: make sure that in case entity ids point to empty cache data
		//       we invalidate the hit and try again
		return m.Storage.CacheEntity(ctx).FindByIDs(ctx, hit.EntityIDs...)
	}

	// this naive MVP approach might take a big burden on the memory.
	// If this becomes the case, it should be possible to change this into a streaming approach
	// where iterator being iterated element by element,
	// and records being created during then in the Storage
	var ids []ID
	res, err := iterators.Collect(query())
	if err != nil {
		return iterators.Error[Ent](err)
	}
	for _, v := range res {
		id, _ := extid.Lookup[ID](v)
		ids = append(ids, id)
	}

	var vs []*Ent
	for _, ent := range res {
		vs = append(vs, &ent)
	}

	if err := m.Storage.CacheEntity(ctx).Upsert(ctx, vs...); err != nil {
		return iterators.Error[Ent](err)
	}

	if err := m.Storage.CacheHit(ctx).Create(ctx, &Hit[ID]{
		QueryID:   queryID,
		EntityIDs: ids,
	}); err != nil {
		return iterators.Error[Ent](err)
	}

	return iterators.Slice[Ent](res)
}

func (m *Manager[Ent, ID]) CacheQueryOne(
	ctx context.Context,
	queryID string,
	query func() (ent Ent, found bool, err error),
) (_ent Ent, _found bool, _err error) {
	iter := m.CacheQueryMany(ctx, queryID, func() frameless.Iterator[Ent] {
		ent, found, err := query()
		if err != nil {
			return iterators.Error[Ent](err)
		}
		if !found {
			return iterators.Empty[Ent]()
		}
		return iterators.Slice[Ent]([]Ent{ent})
	})

	ent, found, err := iterators.First[Ent](iter)
	if err != nil {
		return ent, false, err
	}
	if !found {
		return ent, false, nil
	}
	return ent, true, nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (m *Manager[Ent, ID]) Create(ctx context.Context, ptr *Ent) error {
	if err := m.Source.Create(ctx, ptr); err != nil {
		return err
	}
	return m.Storage.CacheEntity(ctx).Create(ctx, ptr)
}

func (m *Manager[Ent, ID]) FindByID(ctx context.Context, id ID) (Ent, bool, error) {
	// fast path
	ent, found, err := m.Storage.CacheEntity(ctx).FindByID(ctx, id)
	if err != nil {
		return ent, false, err
	}
	if found {
		return ent, true, nil
	}
	// slow path
	return m.CacheQueryOne(ctx, fmt.Sprintf(`FindByID#%v`, id), func() (ent Ent, found bool, err error) {
		return m.Source.FindByID(ctx, id)
	})
}

func (m *Manager[Ent, ID]) FindAll(ctx context.Context) frameless.Iterator[Ent] {
	return m.CacheQueryMany(ctx, `FindAll`, func() frameless.Iterator[Ent] {
		return m.Source.FindAll(ctx)
	})
}

func (m *Manager[Ent, ID]) Update(ctx context.Context, ptr *Ent) error {
	switch src := m.Source.(type) {
	case ExtendedSource[Ent, ID]:
		if err := src.Update(ctx, ptr); err != nil {
			return err
		}
		return m.Storage.CacheEntity(ctx).Upsert(ctx, ptr)
	default:
		return frameless.Error(`not implemented`)
	}
}

func (m *Manager[Ent, ID]) DeleteByID(ctx context.Context, id ID) error {
	if err := m.Source.DeleteByID(ctx, id); err != nil {
		return err
	}

	// TODO: unsafe without additional tx layer
	dataStorage := m.Storage.CacheEntity(ctx)
	_, found, err := dataStorage.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := dataStorage.DeleteByID(ctx, id); err != nil {
		return err
	}
	if err := m.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return nil
}

func (m *Manager[Ent, ID]) DeleteAll(ctx context.Context) error {
	if err := m.Source.DeleteAll(ctx); err != nil {
		return err
	}
	return m.Storage.CacheEntity(ctx).DeleteAll(ctx)
}

func (m *Manager[Ent, ID]) SubscribeToCreatorEvents(ctx context.Context, creatorSubscriber frameless.CreatorSubscriber[Ent]) (frameless.Subscription, error) {
	return m.Source.SubscribeToCreatorEvents(ctx, creatorSubscriber)
}

func (m *Manager[Ent, ID]) SubscribeToDeleterEvents(ctx context.Context, s frameless.DeleterSubscriber[ID]) (frameless.Subscription, error) {
	return m.Source.SubscribeToDeleterEvents(ctx, s)
}

func (m *Manager[Ent, ID]) SubscribeToUpdaterEvents(ctx context.Context, s frameless.UpdaterSubscriber[Ent]) (frameless.Subscription, error) {
	es, ok := m.Source.(ExtendedSource[Ent, ID])
	if !ok {
		return nil, fmt.Errorf("%T doesn't implement frameless.UpdaterPublisher", m.Source)
	}
	return es.SubscribeToUpdaterEvents(ctx, s)
}
