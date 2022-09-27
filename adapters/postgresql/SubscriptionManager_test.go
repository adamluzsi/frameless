package postgresql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/ports/pubsub"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	psh "github.com/adamluzsi/frameless/adapters/postgresql/spechelper"
	"github.com/adamluzsi/frameless/pkg/doubles"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func TestListenerSubscriptionManager_publishWithMappingWhereTableRefIncludesSchemaName(t *testing.T) {
	ctx := context.Background()
	dsn := psh.DatabaseURL(t)
	mapping := psh.TestEntityMapping()
	mapping.Table = "public." + mapping.Table

	cm := postgresql.NewConnectionManager(dsn)
	deferClose(t, cm)

	sm := postgresql.NewListenNotifySubscriptionManager[psh.TestEntity, string](mapping, dsn, cm)
	deferClose(t, sm)

	var last psh.TestEntity
	sub, err := sm.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber[psh.TestEntity, string]{
		HandleFunc: func(ctx context.Context, event interface{}) error {
			ce := event.(pubsub.CreateEvent[psh.TestEntity])
			last = ce.Entity
			return nil
		},
	})
	require.NoError(t, err)
	deferClose(t, sub)

	expected := psh.TestEntity{
		ID:  "42",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}
	require.NoError(t, sm.PublishCreateEvent(ctx, pubsub.CreateEvent[psh.TestEntity]{Entity: expected}))

	testcase.Eventually{RetryStrategy: testcase.Waiter{Timeout: time.Second}}.Assert(t, func(it assert.It) {
		it.Must.Equal(expected, last)
	})
}

func TestListenerSubscriptionManager_reuseListenerAcrossInstances(t *testing.T) {
	ctx := context.Background()
	dsn := psh.DatabaseURL(t)

	cm := postgresql.NewConnectionManager(dsn)
	deferClose(t, cm)

	callback := func(event pq.ListenerEventType, err error) { assert.Must(t).Nil(err) }
	listener := pq.NewListener(dsn, 10*time.Second, time.Minute, callback)
	deferClose(t, listener)

	sm1 := postgresql.ListenNotifySubscriptionManager[psh.TestEntity, string]{
		Mapping:           psh.TestEntityMapping(),
		DSN:               "", // empty intentionally
		ConnectionManager: cm,
		Listener:          listener,
	}
	// no defer close intentionally

	var last psh.TestEntity
	sub, err := sm1.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber[psh.TestEntity, string]{
		HandleFunc: func(ctx context.Context, event interface{}) error {
			ce := event.(pubsub.CreateEvent[psh.TestEntity])
			last = ce.Entity
			return nil
		},
	})
	require.NoError(t, err)
	deferClose(t, sub)

	expected := psh.TestEntity{
		ID:  "42",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	sm2 := postgresql.ListenNotifySubscriptionManager[psh.TestEntity, string]{
		Mapping:           psh.TestEntityMapping(),
		DSN:               "", // empty intentionally
		ConnectionManager: cm,
		Listener:          listener,
	}
	require.NoError(t, sm2.PublishCreateEvent(ctx, pubsub.CreateEvent[psh.TestEntity]{Entity: expected}))

	testcase.Eventually{RetryStrategy: testcase.Waiter{Timeout: time.Second}}.Assert(t, func(it assert.It) {
		it.Must.Equal(expected, last)
	})
}
