package cache

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
)

func (m *Manager) subscribe(ctx context.Context) error {
	subscription, err := m.Source.Subscribe(ctx, &managerSubscriber{Manager: m})
	if err != nil {
		return err
	}
	m.trap(func() { _ = subscription.Close() })
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

func (sub *managerSubscriber) Handle(ctx context.Context, event interface{}) error {
	switch event := event.(type) {
	case frameless.EventCreate:
		return sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx)

	case frameless.EventUpdate:
		if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
			return err
		}
		id, _ := extid.Lookup(event.Entity)
		return sub.Manager.deleteCachedEntity(ctx, id)

	case frameless.EventDeleteByID:
		// TODO: why is this not triggered on Manager.DeleteByID ?
		if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
			return err
		}
		return sub.Manager.deleteCachedEntity(ctx, event.ID)

	case frameless.EventDeleteAll:
		if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
			return err
		}
		return sub.Manager.Storage.CacheEntity(ctx).DeleteAll(ctx)

	default:
		// ignore unknown event
		return nil
	}
}

func (sub *managerSubscriber) Error(ctx context.Context, err error) error {
	// TODO: log.Println("ERROR", err.Error())
	_ = sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx)
	_ = sub.Manager.Storage.CacheEntity(ctx).DeleteAll(ctx)
	return nil
}
