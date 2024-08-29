package mysql_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/mysql"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func TestRepository(t *testing.T) {
	cm, err := mysql.Connect(DatabaseDSN(t))
	assert.NoError(t, err)
	assert.NoError(t, cm.DB.Ping())
	t.Cleanup(func() { assert.NoError(t, cm.Close()) })

	subject := &mysql.Repository[Entity, EntityID]{
		Connection: cm,
		Mapping:    EntityMapping(),
	}

	MigrateEntity(t, cm)

	config := crudcontracts.Config[Entity, EntityID]{
		MakeContext:     context.Background,
		SupportIDReuse:  true,
		SupportRecreate: true,
		ChangeEntity:    nil, // test entity can be freely changed
	}

	testcase.RunSuite(t,
		crudcontracts.Creator[Entity, EntityID](subject, config),
		crudcontracts.Finder[Entity, EntityID](subject, config),
		crudcontracts.Updater[Entity, EntityID](subject, config),
		crudcontracts.Deleter[Entity, EntityID](subject, config),
		crudcontracts.OnePhaseCommitProtocol[Entity, EntityID](subject, subject.Connection),
	)
}
