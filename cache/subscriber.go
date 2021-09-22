package cache

import (
	"context"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
)

func (m *Manager) subscribe(ctx context.Context) error {
	subscriber := &managerSubscriber{Manager: m}

	subscription, err := m.Source.CreatorEvents(ctx, subscriber)
	if err != nil {
		return err
	}
	m.trap(func() { _ = subscription.Close() })

	subscription, err = m.Source.DeleterEvents(ctx, subscriber)
	if err != nil {
		return err
	}
	m.trap(func() { _ = subscription.Close() })

	if src, ok := m.Source.(ExtendedSource); ok {
		subscription, err := src.UpdaterEvents(ctx, subscriber)
		if err != nil {
			return err
		}
		m.trap(func() { _ = subscription.Close() })
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

type managerSubscriber struct {
	Manager *Manager
}

func (sub *managerSubscriber) HandleCreateEvent(ctx context.Context, event frameless.CreateEvent) error {
	return sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx)
}

func (sub *managerSubscriber) HandleUpdateEvent(ctx context.Context, event frameless.UpdateEvent) error {
	if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	id, _ := extid.Lookup(event.Entity)
	return sub.Manager.deleteCachedEntity(ctx, id)
}

func (sub *managerSubscriber) HandleDeleteByIDEvent(ctx context.Context, event frameless.DeleteByIDEvent) error {
	// TODO: why is this not triggered on Manager.DeleteByID ?
	if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return sub.Manager.deleteCachedEntity(ctx, event.ID)
}

func (sub *managerSubscriber) HandleDeleteAllEvent(ctx context.Context, event frameless.DeleteAllEvent) error {
	if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return sub.Manager.Storage.CacheEntity(ctx).DeleteAll(ctx)
}

func (sub *managerSubscriber) Error(ctx context.Context, err error) error {
	// TODO: log.Println("ERROR", err.Error())
	_ = sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx)
	_ = sub.Manager.Storage.CacheEntity(ctx).DeleteAll(ctx)
	return nil
}
