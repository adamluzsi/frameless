package cache

import (
	"context"

	"github.com/adamluzsi/frameless/ports/crud/extid"
	"github.com/adamluzsi/frameless/ports/pubsub"
)

func (m *Manager[Entity, ID]) subscribe(ctx context.Context) error {
	subscriber := &managerSubscriber[Entity, ID]{Manager: m}

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

	if src, ok := m.Source.(ExtendedSource[Entity, ID]); ok {
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

type managerSubscriber[Entity, ID any] struct {
	Manager *Manager[Entity, ID]
}

func (sub *managerSubscriber[Entity, ID]) HandleCreateEvent(ctx context.Context, event pubsub.CreateEvent[Entity]) error {
	return sub.Manager.Repository.CacheHit(ctx).DeleteAll(ctx)
}

func (sub *managerSubscriber[Entity, ID]) HandleUpdateEvent(ctx context.Context, event pubsub.UpdateEvent[Entity]) error {
	if err := sub.Manager.Repository.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	id, _ := extid.Lookup[ID](event.Entity)
	return sub.Manager.deleteCachedEntity(ctx, id)
}

func (sub *managerSubscriber[Entity, ID]) HandleDeleteByIDEvent(ctx context.Context, event pubsub.DeleteByIDEvent[ID]) error {
	// TODO: why is this not triggered on Manager.DeleteByID ?
	if err := sub.Manager.Repository.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return sub.Manager.deleteCachedEntity(ctx, event.ID)
}

func (sub *managerSubscriber[Entity, ID]) HandleDeleteAllEvent(ctx context.Context, event pubsub.DeleteAllEvent) error {
	if err := sub.Manager.Repository.CacheHit(ctx).DeleteAll(ctx); err != nil {
		return err
	}
	return sub.Manager.Repository.CacheEntity(ctx).DeleteAll(ctx)
}

func (sub *managerSubscriber[Entity, ID]) HandleError(ctx context.Context, err error) error {
	// TODO: log.Println("ERROR", err.Error())
	_ = sub.Manager.Repository.CacheHit(ctx).DeleteAll(ctx)
	_ = sub.Manager.Repository.CacheEntity(ctx).DeleteAll(ctx)
	return nil
}
