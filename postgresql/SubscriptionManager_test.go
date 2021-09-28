package postgresql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/doubles"
	"github.com/adamluzsi/frameless/postgresql"
	psh "github.com/adamluzsi/frameless/postgresql/spechelper"
	"github.com/adamluzsi/testcase"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListenerSubscriptionManager_publishWithMappingWhereTableRefIncludesSchemaName(t *testing.T) {
	ctx := context.Background()
	dsn := psh.DatabaseURL(t)
	mapping := psh.TestEntityMapping()
	mapping.Table = "public." + mapping.Table

	cm := postgresql.NewConnectionManager(dsn)
	deferClose(t, cm)

	sm, err := postgresql.NewListenNotifySubscriptionManager(psh.TestEntity{}, mapping, dsn, cm)
	require.NoError(t, err)
	deferClose(t, sm)

	var last psh.TestEntity
	sub, err := sm.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber{
		HandleFunc: func(ctx context.Context, event interface{}) error {
			ce := event.(frameless.CreateEvent)
			last = ce.Entity.(psh.TestEntity)
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
	require.NoError(t, sm.PublishCreateEvent(ctx, frameless.CreateEvent{Entity: expected}))

	testcase.Retry{Strategy: testcase.Waiter{WaitTimeout: time.Second}}.Assert(t, func(tb testing.TB) {
		require.Equal(tb, expected, last)
	})
}

func TestListenerSubscriptionManager_reuseListenerAcrossInstances(t *testing.T) {
	ctx := context.Background()
	dsn := psh.DatabaseURL(t)

	cm := postgresql.NewConnectionManager(dsn)
	deferClose(t, cm)

	callback := func(event pq.ListenerEventType, err error) { assert.NoError(t, err) }
	listener := pq.NewListener(dsn, 10*time.Second, time.Minute, callback)
	deferClose(t, listener)

	sm1 := postgresql.ListenNotifySubscriptionManager{
		T:                 psh.TestEntity{},
		Mapping:           psh.TestEntityMapping(),
		DSN:               "", // empty intentionally
		ConnectionManager: cm,
		Listener:          listener,
	}
	// no defer close intentionally

	var last psh.TestEntity
	sub, err := sm1.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber{
		HandleFunc: func(ctx context.Context, event interface{}) error {
			ce := event.(frameless.CreateEvent)
			last = ce.Entity.(psh.TestEntity)
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

	sm2 := postgresql.ListenNotifySubscriptionManager{
		T:                 psh.TestEntity{},
		Mapping:           psh.TestEntityMapping(),
		DSN:               "", // empty intentionally
		ConnectionManager: cm,
		Listener:          listener,
	}
	require.NoError(t, sm2.PublishCreateEvent(ctx, frameless.CreateEvent{Entity: expected}))

	testcase.Retry{Strategy: testcase.Waiter{WaitTimeout: time.Second}}.Assert(t, func(tb testing.TB) {
		require.Equal(tb, expected, last)
	})
}
