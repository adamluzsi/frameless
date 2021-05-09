package contracts

import (
	"context"
	"database/sql"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/postgresql"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/fixtures"
	"github.com/stretchr/testify/require"
	"testing"
)

type Pool struct {
	Subject    func(tb testing.TB) (postgresql.Pool, contracts.CRD)
	DriverName string
	contracts.FixtureFactory

	// CreateTable to create a dummy table with a specific name.
	// This is used to confirm transaction behaviors.
	CreateTable func(ctx context.Context, client postgresql.SQLClient, name string) error
	// DeleteTable to delete a previously made dummy table.
	DeleteTable func(ctx context.Context, client postgresql.SQLClient, name string) error
	// HasTable reports if a table exist with a given name.
	HasTable func(ctx context.Context, client postgresql.SQLClient, name string) (bool, error)
}

func (spec Pool) Test(t *testing.T) {
	spec.Spec(t)
}

func (spec Pool) Benchmark(b *testing.B) {
	spec.Spec(b)
}

func (spec Pool) pool() testcase.Var {
	return testcase.Var{
		Name: "Pool",
		Init: func(t *testcase.T) interface{} {
			pool, resource := spec.Subject(t)
			spec.resource().Set(t, resource)
			return pool
		},
	}
}

func (spec Pool) poolGet(t *testcase.T) postgresql.Pool {
	return spec.pool().Get(t).(postgresql.Pool)
}

func (spec Pool) resource() testcase.Var {
	return testcase.Var{
		Name: "resource",
		Init: func(t *testcase.T) interface{} {
			_ = spec.pool().Get(t)
			return spec.resource().Get(t)
		},
	}
}

func (spec Pool) resourceGet(t *testcase.T) contracts.CRD {
	return spec.resource().Get(t).(contracts.CRD)
}

func (spec Pool) Spec(tb testing.TB) {
	s := testcase.NewSpec(tb)

	s.Describe(`.GetDSN`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) string {
			return spec.poolGet(t).GetDSN()
		}

		s.Then(`it should return data source name that is usable with sql.Open`, func(t *testcase.T) {
			db, err := sql.Open(spec.DriverName, subject(t))
			require.NoError(t, err)
			t.Defer(db.Close)
			require.NotNil(t, db)
			require.Nil(t, db.Ping())
		})
	})

	s.Describe(`.GetClient`, func(s *testcase.Spec) {
		ctx := s.Let(`context`, func(t *testcase.T) interface{} {
			return spec.Context()
		})
		subject := func(t *testcase.T) (postgresql.SQLClient, func(), error) {
			client, free, err := spec.poolGet(t).GetClient(ctx.Get(t).(context.Context))
			if free != nil {
				t.Defer(free)
			}
			return client, free, err
		}

		s.Then(`it returns a client without an error`, func(t *testcase.T) {
			client, free, err := subject(t)
			require.NoError(t, err)
			require.NotNil(t, client)
			free()
		})

		s.Then(`calling multiple times the returned free func should be okay`, func(t *testcase.T) {
			_, free, err := subject(t)
			require.NoError(t, err)
			free()
			free()
			free()
		})
	})

	s.Test(`.BeginTx + .GetClient = transaction`, func(t *testcase.T) {
		p := spec.poolGet(t)

		tx, err := p.BeginTx(spec.Context())
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		client, free, err := p.GetClient(tx)
		require.NoError(t, err)
		defer free()

		name := spec.makeTestTableName()
		require.Nil(t, spec.CreateTable(tx, client, name))
		defer spec.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))

		free() // free client

		ctx := spec.Context()
		client, free, err = p.GetClient(ctx)
		require.NoError(t, err)
		defer free()

		has, err := spec.HasTable(ctx, client, name)
		require.NoError(t, err)
		require.False(t, has, `it wasn't expected that the created dummy table present after rollback`)
	})

	s.Test(`.GetClient is in no transaction without context from a .BeginTx`, func(t *testcase.T) {
		p := spec.poolGet(t)

		ctx := spec.Context()

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		client, free, err := p.GetClient(ctx) // ctx -> no transaction
		require.NoError(t, err)
		defer free()

		name := spec.makeTestTableName()
		require.Nil(t, spec.CreateTable(tx, client, name))
		defer spec.cleanupTable(t, name)

		require.NoError(t, p.RollbackTx(tx))
		free()

		client, free, err = p.GetClient(ctx)
		require.NoError(t, err)
		defer free()

		has, err := spec.HasTable(ctx, client, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present`)
		free()

		spec.cleanupTable(t, name)
	})

	s.Test(`.BeginTx + .GetClient + .CommitTx`, func(t *testcase.T) {
		p := spec.poolGet(t)

		ctx := spec.Context()

		tx, err := p.BeginTx(ctx)
		require.NoError(t, err)
		t.Defer(p.RollbackTx, tx)

		client, free, err := p.GetClient(tx)
		require.NoError(t, err)
		defer free()

		name := spec.makeTestTableName()
		require.Nil(t, spec.CreateTable(tx, client, name))
		defer spec.cleanupTable(t, name)
		free()

		client, free, err = p.GetClient(ctx) // in no tx
		require.NoError(t, err)
		defer free()

		has, err := spec.HasTable(ctx, client, name)
		require.NoError(t, err)
		require.False(t, has, `it was expected that the created dummy table is not observable outside of the transaction`)
		free()

		require.NoError(t, p.CommitTx(tx))

		client, free, err = p.GetClient(ctx)
		require.NoError(t, err)
		defer free()

		has, err = spec.HasTable(ctx, client, name)
		require.NoError(t, err)
		require.True(t, has, `it was expected that the created dummy table present after commit`)
		free()

		spec.cleanupTable(t, name)
	})
}

func (spec Pool) makeTestTableName() string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	return `test_` + fixtures.Random.StringNWithCharset(6, charset)
}

func (spec Pool) cleanupTable(t *testcase.T, name string) {
	ctx := spec.Context()
	client, free, err := spec.poolGet(t).GetClient(ctx)
	require.NoError(t, err)
	defer free()

	has, err := spec.HasTable(ctx, client, name)
	require.NoError(t, err)
	if !has {
		return
	}

	require.Nil(t, spec.DeleteTable(ctx, client, name))
}
