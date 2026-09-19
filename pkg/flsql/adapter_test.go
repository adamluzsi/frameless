package flsql_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/comproto/comprotocontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

var _ flsql.Connection = flsql.ConnectionAdapter[any, any]{}
var _ flsql.Queryable = flsql.QueryableAdapter{}

func ExampleConnectionAdapter() {
	db, err := sql.Open("dbname", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	_ = flsql.ConnectionAdapter[sql.DB, sql.Tx]{
		DB: db,

		DBAdapter: flsql.QueryableSQL[*sql.DB],
		TxAdapter: flsql.QueryableSQL[*sql.Tx],

		Begin: func(ctx context.Context, db *sql.DB) (*sql.Tx, error) {
			// TODO: integrate begin tx options
			return db.BeginTx(ctx, nil)
		},

		Commit: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Commit()
		},

		Rollback: func(ctx context.Context, tx *sql.Tx) error {
			return tx.Rollback()
		},
	}
}

func ExampleSQLConnectionAdapter() {
	db, err := sql.Open("dbname", os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	_ = flsql.SQLConnectionAdapter(db)
}

func TestQueryableAdapter_ExecContext(t *testing.T) {
	// Arrange

	res := &mockResult{StubRowsAffected: int64(rnd.Int())}

	mockExecFunc := func(ctx context.Context, query string, args ...interface{}) (flsql.Result, error) {
		return res, nil // mock result
	}
	adapter := flsql.QueryableAdapter{
		ExecFunc: mockExecFunc,
	}

	// Act
	ctx := context.Background()
	query := "SELECT * FROM table"
	result, err := adapter.ExecContext(ctx, query)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal[flsql.Result](t, res, result)
}

type mockResult struct{ StubRowsAffected int64 }

func (m *mockResult) RowsAffected() (int64, error) { return m.StubRowsAffected, nil }

func TestQueryableAdapter_QueryContext(t *testing.T) {
	// Arrange
	expRows := &mockRows{StubErr: rnd.Error()}
	expErr := rnd.Error()
	mockQueryFunc := func(ctx context.Context, query string, args ...interface{}) (flsql.Rows, error) {
		return expRows, expErr
	}
	adapter := flsql.QueryableAdapter{
		QueryFunc: mockQueryFunc,
	}

	// Act
	ctx := context.Background()
	query := "SELECT * FROM table"
	rows, err := adapter.QueryContext(ctx, query)

	// Assert
	assert.ErrorIs(t, err, expErr)
	assert.Equal[flsql.Rows](t, expRows, rows)
}

type mockRows struct {
	StubErr  error
	StubNext func() bool
	StubScan func(dest ...any) error
}

func (m *mockRows) Err() error {
	return m.StubErr
}

func (m *mockRows) Next() bool {
	if m.StubNext != nil {
		return m.StubNext()
	}
	return false
}

func (m *mockRows) Scan(dest ...any) error {
	if m.StubScan != nil {
		return m.StubScan(dest...)
	}
	return nil
}

func (m *mockRows) Close() error { return nil }

func TestQueryableAdapter_QueryRowContext(t *testing.T) {
	// Arrange
	expErr := rnd.Error()
	expRow := &mockRow{StubScan: func(dest ...any) error { return expErr }}
	expArgs := []any{rnd.String(), rnd.Int()}

	mockQueryRowFunc := func(ctx context.Context, query string, args ...interface{}) flsql.Row {
		assert.Equal(t, expArgs, args)
		return expRow // mock row
	}
	adapter := flsql.QueryableAdapter{
		QueryRowFunc: mockQueryRowFunc,
	}

	// Act
	ctx := context.Background()
	query := "SELECT * FROM table"
	row := adapter.QueryRowContext(ctx, query, expArgs...)

	// Assert
	assert.NotNil(t, row)
	assert.ErrorIs(t, row.Scan(), expErr)
}

type mockRow struct {
	StubScan func(dest ...any) error
}

func (m *mockRow) Scan(dest ...any) error {
	if m.StubScan != nil {
		return m.StubScan(dest...)
	}
	return nil
}

func TestConnectionAdapter(t *testing.T) {
	s := testcase.NewSpec(t)

	beginTx := func(t *testcase.T, conn flsql.Connection, ctx context.Context) context.Context {
		t.Helper()
		txCtx, err := conn.BeginTx(ctx)
		assert.Must(t).NoError(err)
		t.Cleanup(func() { _ = conn.RollbackTx(txCtx) })
		return txCtx
	}
	lookupTx := func(t *testcase.T, conn flsql.ConnectionAdapter[connectionAdapterTestDB, connectionAdapterTestTX], ctx context.Context) *connectionAdapterTestTX {
		t.Helper()
		tx, ok := conn.LookupTx(ctx)
		assert.Must(t).True(ok)
		assert.Must(t).NotNil(tx)
		return tx
	}
	readValue := func(t *testcase.T, conn flsql.Queryable, ctx context.Context) string {
		t.Helper()
		var value string
		assert.Must(t).NoError(conn.QueryRowContext(ctx, "SELECT value").Scan(&value))
		return value
	}
	writeValue := func(t *testcase.T, conn flsql.Queryable, ctx context.Context, value string) {
		t.Helper()
		_, err := conn.ExecContext(ctx, "UPDATE value", value)
		assert.Must(t).NoError(err)
	}

	var (
		ctx = let.Var(s, func(t *testcase.T) context.Context {
			return context.Background()
		})
		dbValue     = let.Var(s, func(t *testcase.T) string { return "db-" + t.Random.UUID() })
		txValue     = let.Var(s, func(t *testcase.T) string { return "tx-" + t.Random.UUID() })
		newValue    = let.Var(s, func(t *testcase.T) string { return "updated-" + t.Random.UUID() })
		peerDBValue = let.Var(s, func(t *testcase.T) string { return "peer-db-" + t.Random.UUID() })
		db          = let.Var(s, func(t *testcase.T) *connectionAdapterTestDB {
			return &connectionAdapterTestDB{store: connectionAdapterTestStore{value: dbValue.Get(t)}}
		})
		subject = let.Var(s, func(t *testcase.T) flsql.ConnectionAdapter[connectionAdapterTestDB, connectionAdapterTestTX] {
			return newTestConnectionAdapter(db.Get(t))
		})
		peer = let.Var(s, func(t *testcase.T) flsql.ConnectionAdapter[connectionAdapterTestDB, connectionAdapterTestTX] {
			return subject.Get(t) // Copy the adapter, not the DB handle.
		})
		txCtx = let.Var(s, func(t *testcase.T) context.Context {
			txCtx := beginTx(t, subject.Get(t), ctx.Get(t))
			writeValue(t, subject.Get(t), txCtx, txValue.Get(t))
			return txCtx
		})
		tx = let.Var(s, func(t *testcase.T) *connectionAdapterTestTX {
			return lookupTx(t, subject.Get(t), txCtx.Get(t))
		})
	)

	s.Test("comprotocontract.OnePhaseCommitProtocol", func(t *testcase.T) {
		comprotocontract.OnePhaseCommitProtocol(subject.Get(t)).Spec(testcase.NewSpec(t))
	})

	sharedTransactionSpec := func(s *testcase.Spec) {
		s.Describe("#InTx", func(s *testcase.Spec) {
			probeCtx := let.Var(s, txCtx.Get)
			act := func(t *testcase.T) bool {
				detector, ok := any(peer.Get(t)).(comproto.InTx)
				assert.Must(t).True(ok, "ConnectionAdapter must implement the optional comproto.InTx capability")
				return detector.InTx(probeCtx.Get(t))
			}

			s.Test("recognizes the active transaction shared by wrappers of the same DB", func(t *testcase.T) {
				assert.True(t, act(t))
				assert.Equal(t, readValue(t, peer.Get(t), txCtx.Get(t)), txValue.Get(t))
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), dbValue.Get(t))
			})

			for _, completion := range []string{"committed", "rolled back", "canceled"} {
				s.When("the shared context is "+completion, func(s *testcase.Spec) {
					probeCtx.Let(s, func(t *testcase.T) context.Context {
						ctx := txCtx.Get(t)
						switch completion {
						case "committed":
							assert.Must(t).NoError(subject.Get(t).CommitTx(ctx))
						case "rolled back":
							assert.Must(t).NoError(subject.Get(t).RollbackTx(ctx))
						default:
							derived, cancel := context.WithCancel(ctx)
							cancel()
							return derived
						}
						return context.WithoutCancel(ctx)
					})

					s.Then("reports inactive without falling back to querying the DB", func(t *testcase.T) {
						assert.False(t, act(t))
						assert.True(t, lookupTx(t, peer.Get(t), probeCtx.Get(t)) == tx.Get(t))
						var value string
						err := peer.Get(t).QueryRowContext(probeCtx.Get(t), "SELECT value").Scan(&value)
						if completion == "canceled" {
							assert.ErrorIs(t, err, context.Canceled)
						} else {
							assert.ErrorIs(t, err, sql.ErrTxDone)
						}
					})
				})
			}
		})

		s.Describe("#LookupTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) (*connectionAdapterTestTX, bool) {
				return peer.Get(t).LookupTx(txCtx.Get(t))
			}

			s.Test("finds the original adapter's transaction through the same DB handle", func(t *testcase.T) {
				got, ok := act(t)
				assert.True(t, ok)
				assert.True(t, got == tx.Get(t))
				assert.Equal(t, readValue(t, peer.Get(t), txCtx.Get(t)), txValue.Get(t))
			})
		})

		s.Describe("#BeginTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) context.Context {
				return beginTx(t, peer.Get(t), txCtx.Get(t))
			}

			s.Test("nests the same native transaction and leaves persistence to the outer commit", func(t *testcase.T) {
				nestedCtx := act(t)
				assert.True(t, lookupTx(t, peer.Get(t), nestedCtx) == tx.Get(t))
				assert.True(t, lookupTx(t, subject.Get(t), nestedCtx) == tx.Get(t))
				assert.Equal(t, readValue(t, peer.Get(t), nestedCtx), txValue.Get(t))

				writeValue(t, peer.Get(t), nestedCtx, newValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), txCtx.Get(t)), newValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), ctx.Get(t)), dbValue.Get(t))

				assert.NoError(t, peer.Get(t).CommitTx(nestedCtx))
				assert.NoError(t, txCtx.Get(t).Err())
				assert.Equal(t, readValue(t, subject.Get(t), txCtx.Get(t)), newValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), ctx.Get(t)), dbValue.Get(t))

				assert.NoError(t, subject.Get(t).CommitTx(txCtx.Get(t)))
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), newValue.Get(t))
			})

			s.Test("rolling back the nested scope discards the shared transaction", func(t *testcase.T) {
				nestedCtx := act(t)
				writeValue(t, peer.Get(t), nestedCtx, newValue.Get(t))

				assert.NoError(t, peer.Get(t).RollbackTx(nestedCtx))
				assert.Equal(t, readValue(t, subject.Get(t), ctx.Get(t)), dbValue.Get(t))
				assert.ErrorIs(t, subject.Get(t).CommitTx(txCtx.Get(t)), sql.ErrTxDone)
			})
		})
	}

	sharedTransactionSpec(s)

	s.When("the adapter is separately constructed using the same DB pointer", func(s *testcase.Spec) {
		peer.Let(s, func(t *testcase.T) flsql.ConnectionAdapter[connectionAdapterTestDB, connectionAdapterTestTX] {
			return newTestConnectionAdapter(db.Get(t))
		})

		sharedTransactionSpec(s)
	})

	s.When("the adapters use different DB pointers with the same DB and TX Go types", func(s *testcase.Spec) {
		peer.Let(s, func(t *testcase.T) flsql.ConnectionAdapter[connectionAdapterTestDB, connectionAdapterTestTX] {
			return newTestConnectionAdapter(&connectionAdapterTestDB{
				store: connectionAdapterTestStore{value: peerDBValue.Get(t)},
			})
		})

		peerTxCtx := let.Var(s, func(t *testcase.T) context.Context {
			peerCtx := beginTx(t, peer.Get(t), txCtx.Get(t))
			writeValue(t, peer.Get(t), peerCtx, newValue.Get(t))
			return peerCtx
		})

		s.Describe("#InTx", func(s *testcase.Spec) {
			probeCtx := let.Var(s, txCtx.Get)
			act := func(t *testcase.T) bool {
				detector, ok := any(peer.Get(t)).(comproto.InTx)
				assert.Must(t).True(ok, "ConnectionAdapter must implement the optional comproto.InTx capability")
				return detector.InTx(probeCtx.Get(t))
			}

			s.Test("does not recognize the other DB's transaction", func(t *testcase.T) {
				assert.False(t, act(t))
			})

			s.When("both DBs have transactions in the context chain", func(s *testcase.Spec) {
				probeCtx.Let(s, func(t *testcase.T) context.Context {
					return context.WithoutCancel(peerTxCtx.Get(t))
				})

				s.Then("tracks completion independently for each DB", func(t *testcase.T) {
					assert.True(t, act(t))
					owner, ok := any(subject.Get(t)).(comproto.InTx)
					assert.Must(t).True(ok)
					assert.True(t, owner.InTx(probeCtx.Get(t)))
					assert.NoError(t, peer.Get(t).CommitTx(peerTxCtx.Get(t)))
					assert.False(t, act(t))
					assert.True(t, owner.InTx(probeCtx.Get(t)))
					assert.True(t, owner.InTx(txCtx.Get(t)))
					assert.Equal(t, readValue(t, subject.Get(t), txCtx.Get(t)), txValue.Get(t))
					assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), newValue.Get(t))
				})
			})
		})

		assertOriginalTransactionActive := func(t *testcase.T) {
			t.Helper()
			assert.NoError(t, txCtx.Get(t).Err())
			assert.Equal(t, readValue(t, subject.Get(t), txCtx.Get(t)), txValue.Get(t))
			assert.Equal(t, readValue(t, subject.Get(t), ctx.Get(t)), dbValue.Get(t))
			assert.NoError(t, subject.Get(t).CommitTx(txCtx.Get(t)))
			assert.Equal(t, readValue(t, subject.Get(t), ctx.Get(t)), txValue.Get(t))
		}

		s.Describe("#LookupTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) (*connectionAdapterTestTX, bool) {
				return peer.Get(t).LookupTx(txCtx.Get(t))
			}

			s.Test("does not expose a transaction belonging to the other DB", func(t *testcase.T) {
				got, ok := act(t)
				assert.False(t, ok)
				assert.Nil(t, got)

				peerCtx := beginTx(t, peer.Get(t), ctx.Get(t))
				got, ok = subject.Get(t).LookupTx(peerCtx)
				assert.False(t, ok)
				assert.Nil(t, got)
			})
		})

		s.Describe("#QueryRowContext", func(s *testcase.Spec) {
			act := func(t *testcase.T) flsql.Row {
				return peer.Get(t).QueryRowContext(txCtx.Get(t), "SELECT value")
			}

			s.Test("reads its own DB rather than the other DB's transaction", func(t *testcase.T) {
				var value string
				assert.NoError(t, act(t).Scan(&value))
				assert.Equal(t, value, peerDBValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), txCtx.Get(t)), txValue.Get(t))
			})
		})

		s.Describe("#QueryContext", func(s *testcase.Spec) {
			act := func(t *testcase.T) (flsql.Rows, error) {
				return peer.Get(t).QueryContext(txCtx.Get(t), "SELECT value")
			}

			s.Test("reads its own DB rather than the other DB's transaction", func(t *testcase.T) {
				rows, err := act(t)
				assert.Must(t).NoError(err)
				t.Cleanup(func() { assert.NoError(t, rows.Close()) })

				assert.Must(t).True(rows.Next())
				var value string
				assert.NoError(t, rows.Scan(&value))
				assert.Equal(t, value, peerDBValue.Get(t))
				assert.False(t, rows.Next())
				assert.NoError(t, rows.Err())
			})
		})

		s.Describe("#ExecContext", func(s *testcase.Spec) {
			act := func(t *testcase.T) (flsql.Result, error) {
				return peer.Get(t).ExecContext(txCtx.Get(t), "UPDATE value", newValue.Get(t))
			}

			s.Test("writes directly to its own DB without changing the other DB's transaction", func(t *testcase.T) {
				_, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), newValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), txCtx.Get(t)), txValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), ctx.Get(t)), dbValue.Get(t))
			})
		})

		s.Describe("#BeginTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) context.Context {
				return beginTx(t, peer.Get(t), txCtx.Get(t))
			}

			s.Test("starts an independent transaction while keeping both identities reachable in the derived context", func(t *testcase.T) {
				peerCtx := act(t)
				assert.True(t, lookupTx(t, subject.Get(t), peerCtx) == tx.Get(t))
				assert.True(t, lookupTx(t, peer.Get(t), peerCtx) != tx.Get(t))
				got, ok := peer.Get(t).LookupTx(txCtx.Get(t))
				assert.False(t, ok)
				assert.Nil(t, got)

				assert.Equal(t, readValue(t, subject.Get(t), peerCtx), txValue.Get(t))
				assert.Equal(t, readValue(t, peer.Get(t), peerCtx), peerDBValue.Get(t))
				writeValue(t, peer.Get(t), peerCtx, newValue.Get(t))
				assert.Equal(t, readValue(t, peer.Get(t), peerCtx), newValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), peerCtx), txValue.Get(t))
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), peerDBValue.Get(t))
				assert.Equal(t, readValue(t, subject.Get(t), ctx.Get(t)), dbValue.Get(t))
			})
		})

		s.Describe("#CommitTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				return peer.Get(t).CommitTx(peerTxCtx.Get(t))
			}

			s.Test("commits only its own transaction and leaves the original transaction active", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), newValue.Get(t))
				assertOriginalTransactionActive(t)
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), newValue.Get(t))
			})

			s.When("the context contains only the other DB's transaction", func(s *testcase.Spec) {
				peerTxCtx.Let(s, func(t *testcase.T) context.Context { return txCtx.Get(t) })

				s.Then("refuses to commit the other transaction and leaves it active", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), txkit.ErrNoTx)
					assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), peerDBValue.Get(t))
					assertOriginalTransactionActive(t)
					assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), peerDBValue.Get(t))
				})
			})
		})

		s.Describe("#RollbackTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) error {
				return peer.Get(t).RollbackTx(peerTxCtx.Get(t))
			}

			s.Test("rolls back only its own transaction and leaves the original transaction active", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), peerDBValue.Get(t))
				assertOriginalTransactionActive(t)
				assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), peerDBValue.Get(t))
			})

			s.When("the context contains only the other DB's transaction", func(s *testcase.Spec) {
				peerTxCtx.Let(s, func(t *testcase.T) context.Context { return txCtx.Get(t) })

				s.Then("refuses to roll back the other transaction and leaves it active", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), txkit.ErrNoTx)
					assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), peerDBValue.Get(t))
					assertOriginalTransactionActive(t)
					assert.Equal(t, readValue(t, peer.Get(t), ctx.Get(t)), peerDBValue.Get(t))
				})
			})
		})
	})
}

type connectionAdapterTestDB struct {
	store connectionAdapterTestStore
}

type connectionAdapterTestTX struct {
	db    *connectionAdapterTestDB
	store connectionAdapterTestStore
}

func newTestConnectionAdapter(db *connectionAdapterTestDB) flsql.ConnectionAdapter[connectionAdapterTestDB, connectionAdapterTestTX] {
	return flsql.ConnectionAdapter[connectionAdapterTestDB, connectionAdapterTestTX]{
		DB: db,
		DBAdapter: func(db *connectionAdapterTestDB) flsql.Queryable {
			return &db.store
		},
		TxAdapter: func(tx *connectionAdapterTestTX) flsql.Queryable {
			return &tx.store
		},
		Begin: func(ctx context.Context, db *connectionAdapterTestDB) (*connectionAdapterTestTX, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return &connectionAdapterTestTX{db: db, store: db.store}, nil
		},
		Commit: func(ctx context.Context, tx *connectionAdapterTestTX) error {
			if err := tx.store.err(ctx); err != nil {
				return err
			}
			tx.db.store.value = tx.store.value
			tx.store.done = true
			return nil
		},
		Rollback: func(ctx context.Context, tx *connectionAdapterTestTX) error {
			if err := tx.store.err(ctx); err != nil {
				return err
			}
			tx.store.done = true
			return nil
		},
	}
}

// The fake stores one value: reads select it and writes replace it. Transactions
// work on a copy so routing is observable through pending and persisted values.
type connectionAdapterTestStore struct {
	value string
	done  bool
}

func (s *connectionAdapterTestStore) err(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.done {
		return sql.ErrTxDone
	}
	return nil
}

func (s *connectionAdapterTestStore) ExecContext(ctx context.Context, query string, args ...any) (flsql.Result, error) {
	if err := s.err(ctx); err != nil {
		return nil, err
	}
	s.value = args[0].(string)
	return &mockResult{StubRowsAffected: 1}, nil
}

func (s *connectionAdapterTestStore) QueryRowContext(ctx context.Context, query string, args ...any) flsql.Row {
	value, err := s.value, s.err(ctx)
	return &mockRow{StubScan: func(dest ...any) error {
		if err != nil {
			return err
		}
		*dest[0].(*string) = value
		return nil
	}}
}

func (s *connectionAdapterTestStore) QueryContext(ctx context.Context, query string, args ...any) (flsql.Rows, error) {
	if err := s.err(ctx); err != nil {
		return nil, err
	}
	row := s.QueryRowContext(ctx, query, args...)
	next := true
	return &mockRows{
		StubNext: func() bool {
			if !next {
				return false
			}
			next = false
			return true
		},
		StubScan: row.Scan,
	}, nil
}

func TestConnectionAdapter_LookupTx(t *testing.T) {
	type DB struct {
		N string
		V string
	}
	type TX struct {
		ID string
		V  string
	}
	type Queryable struct {
		V string
		F any
	}

	rnd := random.New(random.CryptoSeed{})
	db := &DB{
		N: rnd.Domain(),
		V: rnd.String(),
	}

	subject := flsql.ConnectionAdapter[DB, TX]{
		DB: db,
		DBAdapter: func(db *DB) flsql.Queryable {
			return flsql.QueryableAdapter{}
		},
		TxAdapter: func(tx *TX) flsql.Queryable {
			return flsql.QueryableAdapter{}
		},
		Begin: func(ctx context.Context, db *DB) (*TX, error) {
			return &TX{ID: rnd.UUID(), V: db.V}, nil
		},
		Commit: func(ctx context.Context, tx *TX) error {
			assert.Equal(t, tx.V, db.V)
			return nil
		},
		Rollback: func(ctx context.Context, tx *TX) error {
			assert.Equal(t, tx.V, db.V)
			return nil
		},
	}

	t.Run("comprotocontracts.OnePhaseCommitProtocol",
		comprotocontract.OnePhaseCommitProtocol(subject).Test)

	t.Run("smoke", func(t *testing.T) {
		ctx := context.Background()

		tx, ok := subject.LookupTx(ctx)
		var _ *TX = tx
		assert.False(t, ok)
		assert.Nil(t, tx)

		ctx, err := subject.BeginTx(ctx)
		assert.NoError(t, err)

		tx, ok = subject.LookupTx(ctx)
		assert.True(t, ok)
		assert.NotNil(t, tx)
		assert.Equal(t, tx.V, db.V)
	})

	t.Run("cancel will not cannel the context of the Rollback call", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		var subject flsql.ConnectionAdapter[DB, TX] = subject // pass by value copy
		ogRollback := subject.Rollback
		subject.Rollback = func(ctx context.Context, tx *TX) error {
			assert.NoError(t, ctx.Err())
			return ogRollback(ctx, tx)
		}

		ctx, err := subject.BeginTx(ctx)
		assert.NoError(t, err)

		cancel()
		assert.ErrorIs(t, ctx.Err(), subject.RollbackTx(ctx))
	})
}
