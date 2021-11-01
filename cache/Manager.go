package cache

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
)

func NewManager(T T, storage Storage, source Source) (*Manager, error) {
	var r = &Manager{T: T, Storage: storage, Source: source}
	return r, r.Init(context.Background())
}

type Manager struct {
	T T

	// Source is the location of the original data
	Source Source
	// Storage is the storage that keeps the cached data.
	Storage Storage

	init  sync.Once
	close func()
}

// Source is the minimum expected interface that is expected from a Source resources that needs caching.
// On top of this, cache.Manager also supports Updater, CreatorPublisher, UpdaterPublisher and DeleterPublisher.
type Source interface {
	frameless.Creator
	frameless.Finder
	frameless.Deleter
	frameless.CreatorPublisher
	frameless.DeleterPublisher
}

type ExtendedSource interface {
	frameless.Updater
	frameless.UpdaterPublisher
}

func (m *Manager) Init(ctx context.Context) error {
	var rErr error
	m.init.Do(func() { rErr = m.subscribe(ctx) })
	return rErr
}

func (m *Manager) trap(next func()) {
	if m.close == nil {
		m.close = func() {}
	}

	prev := m.close
	m.close = func() {
		prev()
		next()
	}
}

func (m *Manager) Close() error {
	if m.close == nil {
		return nil
	}
	m.close()
	return nil
}

func (m *Manager) entityTypeName() string {
	return reflects.SymbolicName(m.T)
}

func (m *Manager) deleteCachedEntity(ctx context.Context, id interface{}) (rErr error) {
	ptr := m.newRT().Interface()
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
	found, err := s.FindByID(ctx, ptr, id)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	return s.DeleteByID(ctx, id)
}

func (m *Manager) CacheQueryMany(
	ctx context.Context,
	name string,
	query func() frameless.Iterator,
) frameless.Iterator {
	// TODO: double check
	if ctx != nil && ctx.Err() != nil {
		return iterators.NewError(ctx.Err())
	}

	queryID := fmt.Sprintf(`0:%T/%s`, m.T, name) // add version epoch
	var hit Hit
	found, err := m.Storage.CacheHit(ctx).FindByID(ctx, &hit, queryID)
	if err != nil {
		return iterators.NewError(err)
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
	var vs, ids []interface{}
	if err := iterators.Collect(query(), &vs); err != nil {
		return iterators.NewError(err)
	}
	for _, v := range vs {
		id, _ := extid.Lookup(v)
		ids = append(ids, id)
	}

	if err := m.Storage.CacheEntity(ctx).Upsert(ctx, vs...); err != nil {
		return iterators.NewError(err)
	}

	if err := m.Storage.CacheHit(ctx).Create(ctx, &Hit{
		QueryID:   queryID,
		EntityIDs: ids,
	}); err != nil {
		return iterators.NewError(err)
	}

	return iterators.NewSlice(vs)
}

func (m *Manager) CacheQueryOne(
	ctx context.Context,
	queryID string,
	ptr interface{},
	query func(ptr interface{}) (found bool, err error),
) (_found bool, _err error) {
	return iterators.First(m.CacheQueryMany(ctx, queryID, func() iterators.Interface {
		ptr := m.newRT()
		found, err := query(ptr.Interface())
		if err != nil {
			return iterators.NewError(err)
		}
		if !found {
			return iterators.NewEmpty()
		}
		return iterators.NewSlice([]interface{}{ptr.Elem().Interface()})
	}), ptr)
}

func (m *Manager) newRT() reflect.Value {
	return reflect.New(reflect.TypeOf(m.T))
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func (m *Manager) Create(ctx context.Context, ptr interface{}, opts ...interface{}) error {
	if err := m.Source.Create(ctx, ptr); err != nil {
		return err
	}
	return m.Storage.CacheEntity(ctx).Create(ctx, ptr)
}

func (m *Manager) FindByID(ctx context.Context, ptr, id interface{}) (bool, error) {
	// fast path
	found, err := m.Storage.CacheEntity(ctx).FindByID(ctx, ptr, id)
	if err != nil {
		return false, err
	}
	if found {
		return true, nil
	}
	// slow path
	return m.CacheQueryOne(ctx, fmt.Sprintf(`FindByID#%v`, id), ptr, func(ptr interface{}) (found bool, err error) {
		return m.Source.FindByID(ctx, ptr, id)
	})
}

func (m *Manager) FindAll(ctx context.Context) frameless.Iterator {
	return m.CacheQueryMany(ctx, `FindAll`, func() frameless.Iterator {
		return m.Source.FindAll(ctx)
	})
}

func (m *Manager) Update(ctx context.Context, ptr interface{}) error {
	switch src := m.Source.(type) {
	case ExtendedSource:
		if err := src.Update(ctx, ptr); err != nil {
			return err
		}
		return m.Storage.CacheEntity(ctx).Upsert(ctx, ptr)
	default:
		return frameless.Error(`not implemented`)
	}
}

func (m *Manager) DeleteByID(ctx context.Context, id interface{}) error {
	if err := m.Source.DeleteByID(ctx, id); err != nil {
		return err
	}

	// TODO: unsafe without additional tx layer
	dataStorage := m.Storage.CacheEntity(ctx)
	found, err := dataStorage.FindByID(ctx, m.newRT().Interface(), id)
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

func (m *Manager) DeleteAll(ctx context.Context) error {
	if err := m.Source.DeleteAll(ctx); err != nil {
		return err
	}
	return m.Storage.CacheEntity(ctx).DeleteAll(ctx)
}

func (m *Manager) SubscribeToCreatorEvents(ctx context.Context, creatorSubscriber frameless.CreatorSubscriber) (frameless.Subscription, error) {
	return m.Source.SubscribeToCreatorEvents(ctx, creatorSubscriber)
}

func (m *Manager) SubscribeToDeleterEvents(ctx context.Context, s frameless.DeleterSubscriber) (frameless.Subscription, error) {
	return m.Source.SubscribeToDeleterEvents(ctx, s)
}

func (m *Manager) SubscribeToUpdaterEvents(ctx context.Context, s frameless.UpdaterSubscriber) (frameless.Subscription, error) {
	es, ok := m.Source.(ExtendedSource)
	if !ok {
		return nil, fmt.Errorf("%T doesn't implement frameless.UpdaterPublisher", m.Source)
	}
	return es.SubscribeToUpdaterEvents(ctx, s)
}
