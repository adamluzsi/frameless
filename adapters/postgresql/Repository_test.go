package postgresql_test

import (
	"database/sql"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql/internal/spechelper"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

func Test_postgresConnection(t *testing.T) {
	it := assert.MakeIt(t)

	t.Log(spechelper.DatabaseURL(t))

	db, err := sql.Open("postgres", spechelper.DatabaseURL(t))
	it.Must.Nil(err)
	it.Must.Nil(db.Ping())

	var b bool
	it.Must.Nil(db.QueryRow("SELECT TRUE").Scan(&b))
	it.Must.True(b)
}

func TestRepository(t *testing.T) {
	mapping := spechelper.TestEntityMapping()
	db, err := sql.Open("postgres", spechelper.DatabaseURL(t))
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	cm := postgresql.NewConnectionManagerWithDB(db)
	subject := &postgresql.Repository[spechelper.TestEntity, string]{
		ConnectionManager: cm,
		Mapping:           mapping,
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
	)
}

func TestRepository_mappingHasSchemaInTableName(t *testing.T) {
	cm := NewConnectionManager(t)
	spechelper.MigrateTestEntity(t, cm)

	mapper := spechelper.TestEntityMapping()
	mapper.Table = `public.` + mapper.Table

	subject := postgresql.Repository[spechelper.TestEntity, string]{
		Mapping:           mapper,
		ConnectionManager: cm,
	}

	testcase.RunSuite(t, crudcontracts.Creator[spechelper.TestEntity, string]{
		MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[spechelper.TestEntity, string] { return subject },
		MakeContext: spechelper.MakeContext,
		MakeEntity:  spechelper.MakeTestEntity,

		SupportIDReuse: true,
	})
}
