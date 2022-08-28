package cache

import (
	"context"
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/pubsub"
)

func (m *Manager[Ent, ID]) subscribe(ctx context.Context) error {
	subscriber := &managerSubscriber[Ent, ID]{Manager: m}

	subscription, err := m.Source.SubscribeToCreatorEvents(ctx, subscriber)
	if err != nil {
		return err
	}
	m.trap(func() { _ = subscription.Close() })

	subscription, err = m.Source.SubscribeToDeleterEvents(ctx, subscriber)
	if err != nil {
		return err
	}
	m.trap(func() { _ = subscription.Close() })

	if src, ok := m.Source.(ExtendedSource[Ent, ID]); ok {
		subscription, err := src.SubscribeToUpdaterEvents(ctx, subscriber)
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

func (m subscriber) HandleError(ctx context.Context, err error) error {
	if m.ErrorFunc != nil {
		return m.ErrorFunc(ctx, err)
	}

	return nil
}

type managerSubscriber[Ent, ID any] struct {
	Manager *Manager[Ent, ID]
}

func (sub *managerSubscriber[Ent, ID]) HandleCreateEvent(ctx context.Context, event pubsub.CreateEvent[Ent]) error {
	return sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx)
}

func (sub *managerSubscriber[Ent, ID]) HandleUpdateEvent(ctx context.Context, event pubsub.UpdateEvent[Ent]) error {
	if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	id, _ := extid.Lookup[ID](event.Entity)
	return sub.Manager.deleteCachedEntity(ctx, id)
}

func (sub *managerSubscriber[Ent, ID]) HandleDeleteByIDEvent(ctx context.Context, event pubsub.DeleteByIDEvent[ID]) error {
	// TODO: why is this not triggered on Manager.DeleteByID ?
	if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return sub.Manager.deleteCachedEntity(ctx, event.ID)
}

func (sub *managerSubscriber[Ent, ID]) HandleDeleteAllEvent(ctx context.Context, event pubsub.DeleteAllEvent) error {
	if err := sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return sub.Manager.Storage.CacheEntity(ctx).DeleteAll(ctx)
}

func (sub *managerSubscriber[Ent, ID]) HandleError(ctx context.Context, err error) error {
	// TODO: log.Println("ERROR", err.Error())
	_ = sub.Manager.Storage.CacheHit(ctx).DeleteAll(ctx)
	_ = sub.Manager.Storage.CacheEntity(ctx).DeleteAll(ctx)
	return nil
}
