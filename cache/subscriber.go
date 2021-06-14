package cache

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
)

func (m *Manager) subscribe(ctx context.Context) error {
	subscribe := func(blk func() (frameless.Subscription, error)) error {
		sub, err := blk()
		if err != nil {
			return err
		}
		m.trap(func() { _ = sub.Close() })
		return nil
	}
	if err := subscribe(func() (frameless.Subscription, error) {
		return m.Source.SubscribeToCreate(ctx, m.getSubscriberCreate())
	}); err != nil {
		return err
	}
	if err := subscribe(func() (frameless.Subscription, error) {
		return m.Source.SubscribeToDeleteByID(ctx, m.getSubscriberDeleteByID())
	}); err != nil {
		return err
	}
	if err := subscribe(func() (frameless.Subscription, error) {
		return m.Source.SubscribeToDeleteAll(ctx, m.getSubscriberDeleteAll())
	}); err != nil {
		return err
	}
	if src, ok := m.Source.(ExtendedSource); ok {
		if err := subscribe(func() (frameless.Subscription, error) {
			return src.SubscribeToUpdate(ctx, m.getSubscriberUpdate())
		}); err != nil {
			return err
		}
	}
	return nil
}

type subscriber struct {
	HandleFunc func(ctx context.Context, ent interface{}) error
	ErrorFunc  func(ctx context.Context, err error) error
}

func (m subscriber) Handle(ctx context.Context, ent interface{}) error {
	if m.HandleFunc != nil {
		return m.HandleFunc(ctx, ent)
	}

	return nil
}

func (m subscriber) Error(ctx context.Context, err error) error {
	if m.ErrorFunc != nil {
		return m.ErrorFunc(ctx, err)
	}

	return nil
}

func (m *Manager) getSubscriberCreate() frameless.Subscriber {
	// deleting cache hits is enough,
	// as they will be lazy evaluated
	return subscriber{
		HandleFunc: func(ctx context.Context, ent interface{}) error {
			return m.Storage.CacheHit(ctx).DeleteAll(ctx)
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			return m.Storage.CacheHit(ctx).DeleteAll(ctx)
		},
	}
}

func (m *Manager) getSubscriberUpdate() frameless.Subscriber {
	return subscriber{
		HandleFunc: func(ctx context.Context, ent interface{}) error {
			if err := m.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
				return err
			}
			id, _ := extid.Lookup(ent)
			return m.deleteCachedEntity(ctx, id)
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			_ = m.Storage.CacheHit(ctx).DeleteAll(ctx)
			_ = m.Storage.CacheEntity(ctx).DeleteAll(ctx)
			return nil
		},
	}
}

func (m *Manager) getSubscriberDeleteAll() frameless.Subscriber {
	return subscriber{
		HandleFunc: func(ctx context.Context, ent interface{}) error {
			if err := m.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
				return err
			}
			return m.Storage.CacheEntity(ctx).DeleteAll(ctx)
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			_ = m.Storage.CacheHit(ctx).DeleteAll(ctx)
			_ = m.Storage.CacheEntity(ctx).DeleteAll(ctx)
			return nil
		},
	}
}

func (m *Manager) getSubscriberDeleteByID() frameless.Subscriber {
	return subscriber{
		HandleFunc: func(ctx context.Context, ent interface{}) error {
			// TODO: why is this not triggered on Manager.DeleteByID ?
			if err := m.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
				return err
			}
			id, _ := extid.Lookup(ent)
			return m.deleteCachedEntity(ctx, id)
		},
		ErrorFunc: func(ctx context.Context, err error) error {
			_ = m.Storage.CacheHit(ctx).DeleteAll(ctx)
			_ = m.Storage.CacheEntity(ctx).DeleteAll(ctx)
			return nil
		},
	}
}
