package cache

import (
	"github.com/adamluzsi/frameless/ports/pubsub"
)

// LocalCache is responsible for providing a cache layer that allows developers to use a local resource
// such as in-memory or filesystem as their cache Repository.
// LocalCache ensures that the cache is kept in sync with the external resource
// using a pubsub mechanism that listens for mutation events such as create, update, or delete on the external resource.
type LocalCache[Entity, ID any] struct {
	Cache *Cache[Entity, ID]

	// EventExchange is a fan-out exchange that ensures that all local resource kept in sync using the events
	// which are fired when a mutation related event is done.
	EventExchange LocalCacheEventFanOutExchange[ID]
}

type LocalCacheEvent[ID any] struct {
	Type localCacheEventType `enum:"refresh;invalidate;drop;"`
	ID   ID
}

type localCacheEventType string

const (
	LocalCacheEventTypeRefresh    localCacheEventType = "refresh"
	LocalCacheEventTypeInvalidate localCacheEventType = "invalidate"
	LocalCacheEventTypeDrop       localCacheEventType = "drop"
)

type LocalCacheEventFanOutExchange[ID any] interface {
	Publisher() pubsub.Publisher[LocalCacheEvent[ID]]
	Subscriber() pubsub.Subscriber[LocalCacheEvent[ID]]
}
