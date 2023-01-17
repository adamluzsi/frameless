package postgresql_test

import (
	"context"
	"database/sql"
	"testing"

	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"
	frmetacontracts "github.com/adamluzsi/frameless/ports/meta/contracts"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/contracts"
	contracts2 "github.com/adamluzsi/frameless/spechelper/resource"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/testcase/assert"

	psh "github.com/adamluzsi/frameless/adapters/postgresql/spechelper"

	"github.com/adamluzsi/testcase"
)

func TestPostgresConnection(t *testing.T) {
	it := assert.MakeIt(t)

	t.Log(psh.DatabaseURL(t))

	db, err := sql.Open("postgres", psh.DatabaseURL(t))
	it.Must.Nil(err)
	it.Must.Nil(db.Ping())

	var b bool
	it.Must.Nil(db.QueryRow("SELECT TRUE").Scan(&b))
	it.Must.True(b)
}

func TestNewRepository_smoke(t *testing.T) {
	repository := NewTestEntityRepository(t)

	ctx := context.Background()

	ent := &psh.TestEntity{
		Foo: "foo",
		Bar: "bar",
		Baz: "baz",
	}

	assert.NoError(t, repository.Create(ctx, ent))
	assert.NotEmpty(t, ent.ID)

	ent2, found, err := repository.FindByID(ctx, ent.ID)
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, *ent, ent2)

	assert.NoError(t, repository.DeleteByID(ctx, ent.ID))
	_, found, err = repository.FindByID(ctx, ent.ID)
	assert.NoError(t, err)
	assert.False(t, found, `should be deleted`)
}

func TestRepository(t *testing.T) {
	mapping := psh.TestEntityMapping()
	cm := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	sm := postgresql.NewListenNotifySubscriptionManager[psh.TestEntity, string](mapping, psh.DatabaseURL(t), cm)
	subject := &postgresql.Repository[psh.TestEntity, string]{
		ConnectionManager:   cm,
		SubscriptionManager: sm,
		Mapping:             mapping,
	}

	psh.MigrateTestEntity(t, cm)

	testcase.RunSuite(t,
		crudcontracts.Creator[psh.TestEntity, string]{
			MakeSubject:    func(tb testing.TB) crudcontracts.CreatorSubject[psh.TestEntity, string] { return subject },
			MakeEntity:     psh.MakeTestEntity,
			MakeContext:    psh.MakeContext,
			SupportIDReuse: true,
		},
		crudcontracts.Finder[psh.TestEntity, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[psh.TestEntity, string] {
				return any(subject).(crudcontracts.FinderSubject[psh.TestEntity, string])
			},
			MakeEntity:  psh.MakeTestEntity,
			MakeContext: psh.MakeContext,
		},
		crudcontracts.Updater[psh.TestEntity, string]{MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[psh.TestEntity, string] { return subject },
			MakeEntity:  psh.MakeTestEntity,
			MakeContext: psh.MakeContext,
		},
		crudcontracts.Deleter[psh.TestEntity, string]{MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[psh.TestEntity, string] { return subject },
			MakeEntity:  psh.MakeTestEntity,
			MakeContext: psh.MakeContext,
		},
		crudcontracts.OnePhaseCommitProtocol[psh.TestEntity, string]{
			MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[psh.TestEntity, string] {
				return crudcontracts.OnePhaseCommitProtocolSubject[psh.TestEntity, string]{
					Resource:      subject,
					CommitManager: cm,
				}
			},
			MakeEntity:  psh.MakeTestEntity,
			MakeContext: psh.MakeContext,
		},
		pubsubcontracts.Publisher[psh.TestEntity, string]{MakeSubject: func(tb testing.TB) pubsubcontracts.PublisherSubject[psh.TestEntity, string] { return subject },
			MakeEntity:  psh.MakeTestEntity,
			MakeContext: psh.MakeContext,
		},
		frmetacontracts.MetaAccessor[psh.TestEntity, string, string]{
			MakeSubject: func(tb testing.TB) frmetacontracts.MetaAccessorSubject[psh.TestEntity, string, string] {
				return frmetacontracts.MetaAccessorSubject[psh.TestEntity, string, string]{
					MetaAccessor: subject,
					Resource:     subject,
					Publisher:    subject,
				}
			},
			MakeEntity:  psh.MakeTestEntity,
			MakeContext: psh.MakeContext,
			MakeV:       psh.MakeString,
		},
	)
}

func TestRepository_contracts(t *testing.T) {
	s := testcase.NewSpec(t)
	repository := NewTestEntityRepository(t)

	contracts2.Contract[psh.TestEntity, string, string]{
		MakeSubject: func(tb testing.TB) contracts2.ContractSubject[psh.TestEntity, string] {
			return contracts2.ContractSubject[psh.TestEntity, string]{
				MetaAccessor:  repository,
				CommitManager: repository,
				Resource:      repository,
			}
		},
		MakeEntity:  psh.MakeTestEntity,
		MakeContext: psh.MakeContext,
		MakeV:       psh.MakeString,
	}.Spec(s)
}

func TestRepository_mappingHasSchemaInTableName(t *testing.T) {
	cm := postgresql.NewConnectionManager(psh.DatabaseURL(t))
	psh.MigrateTestEntity(t, cm)

	mapper := psh.TestEntityMapping()
	mapper.Table = `public.` + mapper.Table

	subject := NewTestEntityRepository(t)

	testcase.RunSuite(t, crudcontracts.Creator[psh.TestEntity, string]{
		MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[psh.TestEntity, string] { return subject },
		MakeContext: psh.MakeContext,
		MakeEntity:  psh.MakeTestEntity,

		SupportIDReuse: true,
	})
}
