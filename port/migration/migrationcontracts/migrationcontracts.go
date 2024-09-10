package migrationcontracts

import (
	"context"
	"testing"

	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func StateRepository(subject migration.StateRepository) contract.Contract {
	s := testcase.NewSpec(nil)

	namespace := testcase.Let(s, func(t *testcase.T) string {
		return t.Random.String()
	})

	config := crudcontracts.Config[migration.State, migration.StateID]{
		MakeContext:     context.Background,
		SupportIDReuse:  true,
		SupportRecreate: true,
		ChangeEntity:    nil, // test entity can be freely changed

		MakeEntity: func(t testing.TB) migration.State {
			tc := t.(*testcase.T)
			return migration.State{
				ID: migration.StateID{
					Namespace: namespace.Get(tc),
					Version:   migration.Version(tc.Random.StringNWithCharset(5, random.CharsetDigit())),
				},
				Dirty: tc.Random.Bool(),
			}
		},
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[migration.State, migration.StateID](subject, config),
		crudcontracts.ByIDFinder[migration.State, migration.StateID](subject, config),
		crudcontracts.ByIDDeleter[migration.State, migration.StateID](subject, config),
		crudcontracts.OnePhaseCommitProtocol[migration.State, migration.StateID](subject, subject, config),
	)

	return s.AsSuite("migration.StateRepository")
}
