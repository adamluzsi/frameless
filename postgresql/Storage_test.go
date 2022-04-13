package postgresql_test

import (
	"context"
	"github.com/adamluzsi/frameless/resources"
	"testing"

	"github.com/adamluzsi/frameless/postgresql"
	psh "github.com/adamluzsi/frameless/postgresql/spechelper"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/contracts"
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
	sm := postgresql.NewListenNotifySubscriptionManager[psh.TestEntity{}](mapping, psh.DatabaseURL(t), cm)

	subject := &postgresql.Storage{
		ConnectionManager:   cm,
		SubscriptionManager: sm,
		Mapping:             mapping,
	}

	psh.MigrateTestEntity(t, cm)

	cf := func(testing.TB) context.Context { return context.Background() }

	// testcase.RunContract(t,
	// 	contracts.Creator{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
	// 	contracts.Finder{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
	// 	contracts.Updater{T: T, Subject: func(tb testing.TB) contracts.UpdaterSubject { return subject }, FixtureFactory: fff, Context: cf},
	// 	contracts.Deleter{T: T, Subject: func(tb testing.TB) contracts.CRD { return subject }, FixtureFactory: fff, Context: cf},
	// 	contracts.OnePhaseCommitProtocol{T: T, Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) { return cm, subject }, FixtureFactory: fff, Context: cf},
	// 	contracts.Publisher{T: T, Subject: func(tb testing.TB) contracts.PublisherSubject { return subject }, FixtureFactory: fff, Context: cf},
	// 	contracts.MetaAccessor{T: T, V: "string",
	// 		Subject: func(tb testing.TB) contracts.MetaAccessorSubject {
	// 			return contracts.MetaAccessorSubject{
	// 				MetaAccessor: subject,
	// 				Resource:     subject,
	// 				Publisher:    subject,
	// 			}
	// 		},
	// 		FixtureFactory: fff,
	// 		Context:        cf,
	// 	},
	// )
}

func TestStorage_contracts(t *testing.T) {
	s := testcase.NewSpec(t)
	storage := NewStorage(t)

	resources.Contract[psh.TestEntity, string, string]{
		Subject: func(tb testing.TB) resources.ContractSubject[psh.TestEntity, string] {
			return resources.ContractSubject[psh.TestEntity, string]{
				MetaAccessor:  storage,
				CommitManager: storage,
				Resource:      storage,
			}
		},
		MakeEnt: func(tb testing.TB) psh.TestEntity {
			t := tb.(*testcase.T)
			return t.Random.Make(psh.TestEntity{}).(psh.TestEntity)
		},
		MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
	}.Spec(s)
}

func TestStorage_mappingHasSchemaInTableName(t *testing.T) {
	cm := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	psh.MigrateTestEntity(t, cm)

	mapper := psh.TestEntityMapping()
	mapper.Table = `public.` + mapper.Table

	subject := NewStorage(t)

	testcase.RunContract(t, contracts.Creator[psh.TestEntity, string]{
		Subject: func(tb testing.TB) contracts.CreatorSubject { return subject },
		1MakeCtx: func(tb testing.TB) context.Context {
			return context.Background()
		},
		MakeEnt: func(tb testing.TB) psh.TestEntity {
			t := tb.(*testcase.T)
			return t.Random.Make(psh.TestEntity{}).(psh.TestEntity)
		},
	})
}
