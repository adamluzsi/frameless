package postgresql_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/spechelper"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/postgresql"
	psh "github.com/adamluzsi/frameless/postgresql/spechelper"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/testcase"
)

func TestNewStorage_smoke(t *testing.T) {
	storage := NewStorage(t)

	ctx := context.Background()

	ent := &psh.TestEntity{
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	require.NoError(t, storage.Create(ctx, ent))
	require.NotEmpty(t, ent.ID)

	var ent2 psh.TestEntity
	found, err := storage.FindByID(ctx, &ent2, ent.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, *ent, ent2)

	require.NoError(t, storage.DeleteByID(ctx, ent.ID))
	found, err = storage.FindByID(ctx, &psh.TestEntity{}, ent.ID)
	require.NoError(t, err)
	require.False(t, found, `should be deleted`)
}

func TestStorage(t *testing.T) {
	T := psh.TestEntity{}
	mapping := psh.TestEntityMapping()

	cm := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	sm, err := postgresql.NewListenNotifySubscriptionManager(T, mapping, psh.DatabaseURL(t), cm)
	require.NoError(t, err)

	subject := &postgresql.Storage{
		T:                   T,
		ConnectionManager:   cm,
		SubscriptionManager: sm,
		Mapping:             mapping,
	}

	psh.MigrateTestEntity(t, cm)

	fff := func(tb testing.TB) frameless.FixtureFactory {
		return fixtures.NewFactory(tb)
	}
	cf := func(testing.TB) context.Context { return context.Background() }

	testcase.RunContract(t,
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: fff, Context: cf},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return cm, subject }, FixtureFactory: fff, Context: cf},
		contracts.Publisher{T: T, Subject: func(tb testing.TB) contracts.PublisherSubject { return subject }, FixtureFactory: fff, Context: cf},
		contracts.MetaAccessor{T: T, V: "string",
			Subject: func(tb testing.TB) contracts.MetaAccessorSubject {
				return contracts.MetaAccessorSubject{
					MetaAccessor: subject,
					Resource:     subject,
					Publisher:    subject,
				}
			},
			FixtureFactory: fff,
			Context:        cf,
		},
	)
}

func TestStorage_contracts(t *testing.T) {
	s := testcase.NewSpec(t)
	T := psh.TestEntity{}
	storage := NewStorage(t)

	spechelper.Contract{T: T, V: "string",
		Subject: func(tb testing.TB) spechelper.ContractSubject {
			return spechelper.ContractSubject{
				MetaAccessor:           storage,
				OnePhaseCommitProtocol: storage,
				CRUD:                   storage,
			}
		},
		FixtureFactory: func(tb testing.TB) frameless.FixtureFactory {
			return fixtures.NewFactory(tb)
		},
		Context: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Spec(s)
}

func TestStorage_mappingHasSchemaInTableName(t *testing.T) {
	T := psh.TestEntity{}
	cm := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	psh.MigrateTestEntity(t, cm)

	mapper := psh.TestEntityMapping()
	mapper.Table = `public.` + mapper.Table

	subject := NewStorage(t)

	fff := func(tb testing.TB) frameless.FixtureFactory {
		return fixtures.NewFactory(tb)
	}
	cf := func(testing.TB) context.Context {
		return context.Background()
	}
	testcase.RunContract(t,
		contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
		contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
		contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: fff, Context: cf},
		contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
		contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return cm, subject }, FixtureFactory: fff, Context: cf},
		contracts.Publisher{T: T, Subject: func(tb testing.TB) contracts.PublisherSubject { return subject }, FixtureFactory: fff, Context: cf},
	)
}
