package postgresql_test

import (
	"context"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	frmetacontracts "github.com/adamluzsi/frameless/ports/meta/metacontracts"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"

	"github.com/adamluzsi/frameless/ports/pubsub"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/pkg/doubles"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/lib/pq"
)

func TestRepositoryWithCUDEvents(t *testing.T) {
	mapping := spechelper.TestEntityMapping()
	cm, err := postgresql.NewConnectionManagerWithDSN(spechelper.DatabaseURL(t))
	assert.NoError(t, err)

	repo := postgresql.Repository[spechelper.TestEntity, string]{
		Mapping:           mapping,
		ConnectionManager: cm,
	}
	subject := &postgresql.RepositoryWithCUDEvents[spechelper.TestEntity, string]{
		Repository:          repo,
		SubscriptionManager: postgresql.NewListenNotifySubscriptionManager[spechelper.TestEntity, string](mapping, spechelper.DatabaseURL(t), cm),
	}

	spechelper.MigrateTestEntity(t, cm)

	testcase.RunSuite(t,
		crudcontracts.Creator[spechelper.TestEntity, string]{
			MakeSubject:    func(tb testing.TB) crudcontracts.CreatorSubject[spechelper.TestEntity, string] { return subject },
			MakeEntity:     spechelper.MakeTestEntity,
			MakeContext:    spechelper.MakeContext,
			SupportIDReuse: true,
		},
		crudcontracts.Finder[spechelper.TestEntity, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[spechelper.TestEntity, string] {
				return any(subject).(crudcontracts.FinderSubject[spechelper.TestEntity, string])
			},
			MakeEntity:  spechelper.MakeTestEntity,
			MakeContext: spechelper.MakeContext,
		},
		crudcontracts.Updater[spechelper.TestEntity, string]{MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[spechelper.TestEntity, string] { return subject },
			MakeEntity:  spechelper.MakeTestEntity,
			MakeContext: spechelper.MakeContext,
		},
		crudcontracts.Deleter[spechelper.TestEntity, string]{MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[spechelper.TestEntity, string] { return subject },
			MakeEntity:  spechelper.MakeTestEntity,
			MakeContext: spechelper.MakeContext,
		},
		crudcontracts.OnePhaseCommitProtocol[spechelper.TestEntity, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[spechelper.TestEntity, string] {
				return crudcontracts.OnePhaseCommitProtocolSubject[spechelper.TestEntity, string]{
					Resource:      subject,
					CommitManager: cm,
				}
			},
			MakeEntity:  spechelper.MakeTestEntity,
			MakeContext: spechelper.MakeContext,
		},
		pubsubcontracts.Publisher[spechelper.TestEntity, string]{MakeSubject: func(tb testing.TB) pubsubcontracts.PublisherSubject[spechelper.TestEntity, string] { return subject },
			MakeEntity:  spechelper.MakeTestEntity,
			MakeContext: spechelper.MakeContext,
		},
		frmetacontracts.MetaAccessor[spechelper.TestEntity, string, string]{
			MakeSubject: func(tb testing.TB) frmetacontracts.MetaAccessorSubject[spechelper.TestEntity, string, string] {
				return frmetacontracts.MetaAccessorSubject[spechelper.TestEntity, string, string]{
					MetaAccessor: subject,
					Resource:     subject,
					Publisher:    subject,
				}
			},
			MakeEntity:  spechelper.MakeTestEntity,
			MakeContext: spechelper.MakeContext,
			MakeV:       spechelper.MakeString,
		},
	)
}

func TestListenerSubscriptionManager_publishWithMappingWhereTableRefIncludesSchemaName(t *testing.T) {
	ctx := context.Background()
	dsn := spechelper.DatabaseURL(t)
	mapping := spechelper.TestEntityMapping()
	mapping.Table = "public." + mapping.Table

	cm, err := postgresql.NewConnectionManagerWithDSN(dsn)
	assert.NoError(t, err)
	deferClose(t, cm)

	sm := postgresql.NewListenNotifySubscriptionManager[spechelper.TestEntity, string](mapping, dsn, cm)
	deferClose(t, sm)

	var last spechelper.TestEntity
	sub, err := sm.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber[spechelper.TestEntity, string]{
		HandleFunc: func(ctx context.Context, event interface{}) error {
			ce := event.(pubsub.CreateEvent[spechelper.TestEntity])
			last = ce.Entity
			return nil
		},
	})
	assert.NoError(t, err)
	deferClose(t, sub)

	expected := spechelper.TestEntity{
		ID:  "42",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}
	assert.NoError(t, sm.PublishCreateEvent(ctx, pubsub.CreateEvent[spechelper.TestEntity]{Entity: expected}))

	assert.EventuallyWithin(time.Second).Assert(t, func(it assert.It) {
		it.Must.Equal(expected, last)
	})
}

func TestListenerSubscriptionManager_reuseListenerAcrossInstances(t *testing.T) {
	ctx := context.Background()
	dsn := spechelper.DatabaseURL(t)

	cm, err := postgresql.NewConnectionManagerWithDSN(dsn)
	assert.NoError(t, err)
	deferClose(t, cm)

	callback := func(event pq.ListenerEventType, err error) { assert.Must(t).Nil(err) }
	listener := pq.NewListener(dsn, 10*time.Second, time.Minute, callback)
	deferClose(t, listener)

	sm1 := postgresql.ListenNotifySubscriptionManager[spechelper.TestEntity, string]{
		Mapping:           spechelper.TestEntityMapping(),
		DSN:               "", // empty intentionally
		ConnectionManager: cm,
		Listener:          listener,
	}
	// no defer close intentionally

	var last spechelper.TestEntity
	sub, err := sm1.SubscribeToCreatorEvents(ctx, doubles.StubSubscriber[spechelper.TestEntity, string]{
		HandleFunc: func(ctx context.Context, event interface{}) error {
			ce := event.(pubsub.CreateEvent[spechelper.TestEntity])
			last = ce.Entity
			return nil
		},
	})
	assert.NoError(t, err)
	deferClose(t, sub)

	expected := spechelper.TestEntity{
		ID:  "42",
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	sm2 := postgresql.ListenNotifySubscriptionManager[spechelper.TestEntity, string]{
		Mapping:           spechelper.TestEntityMapping(),
		DSN:               "", // empty intentionally
		ConnectionManager: cm,
		Listener:          listener,
	}
	assert.NoError(t, sm2.PublishCreateEvent(ctx, pubsub.CreateEvent[spechelper.TestEntity]{Entity: expected}))

	testcase.Eventually{RetryStrategy: testcase.Waiter{Timeout: time.Second}}.Assert(t, func(it assert.It) {
		it.Must.Equal(expected, last)
	})
}
