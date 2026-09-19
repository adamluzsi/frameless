package txkit_test

import (
	"context"
	"fmt"
	"testing"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/txkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/comproto/comprotocontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

func Test(t *testing.T) {
	s := testcase.NewSpec(t)
	s.HasSideEffect()

	comprotocontract.OnePhaseCommitProtocol(CPProxy{
		BeginTxFn:    txkit.Begin,
		CommitTxFn:   txkit.Commit,
		RollbackTxFn: txkit.Rollback,
	}).Spec(s)

	s.Test("on commit, no rollback is executed", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		var i = 24
		assert.Must(t).NoError(txkit.OnRollback(tx1, func() { i = 24 }))
		i = 42
		assert.Must(t).NoError(txkit.Commit(tx1))
		assert.Must(t).Equal(42, i)
	})

	s.Test("on rollback, rollback steps are executed in LIFO order", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		ns := make([]int, 0, 2)
		assert.Must(t).NoError(txkit.OnRollback(tx1, func() { ns = append(ns, 42) }))
		assert.Must(t).NoError(txkit.OnRollback(tx1, func() { ns = append(ns, 24) }))
		assert.Must(t).NoError(txkit.Rollback(tx1))
		assert.Must(t).Equal([]int{24, 42}, ns)
	})

	s.Test("after rollback, adding further rollback steps yields a tx done error", func(t *testcase.T) {
		ctx, _ := txkit.Begin(context.Background())
		assert.Must(t).NoError(txkit.Rollback(ctx))
		assert.Must(t).ErrorIs(txkit.ErrTxDone, txkit.OnRollback(ctx, func() {}))
	})

	s.Test("on multiple rollback, rollback steps are executed only once", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		var n int
		assert.Must(t).NoError(txkit.OnRollback(tx1, func() { n += 42 }))

		assert.Must(t).NoError(txkit.Rollback(tx1))
		assert.Must(t).Equal(42, n)

		assert.Must(t).ErrorIs(txkit.ErrTxDone, txkit.Rollback(tx1))
		assert.Must(t).Equal(42, n)
	})

	s.Test("on multiple commit, the second call yields tx is already done error", func(t *testcase.T) {
		ctx, _ := txkit.Begin(context.Background())
		assert.Must(t).NoError(txkit.Commit(ctx))
		assert.Must(t).ErrorIs(txkit.ErrTxDone, txkit.Commit(ctx))
	})

	s.Test("on multiple rollback, the second call yields tx is already done error", func(t *testcase.T) {
		ctx, _ := txkit.Begin(context.Background())
		assert.Must(t).NoError(txkit.Rollback(ctx))
		assert.Must(t).ErrorIs(txkit.ErrTxDone, txkit.Rollback(ctx))
	})

	s.Test("on multi tx stage, the most outer tx rollback override the commits", func(t *testcase.T) {
		ctx := context.Background()
		tx1, _ := txkit.Begin(ctx)

		var n int
		tx2, _ := txkit.Begin(tx1)
		assert.Must(t).NoError(txkit.OnRollback(tx2, func() { n += 42 }))

		// commit in the inner layer
		assert.Must(t).NoError(txkit.Commit(tx2))

		// rollback at higher level
		assert.Must(t).NoError(txkit.Rollback(tx1))
		assert.Must(t).Equal(42, n)
	})

	s.Test("on rollback, if rollback step encounters an error, it is propagated back as Rollback results", func(t *testcase.T) {
		ctx, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		expectedErr := t.Random.Error()
		assert.Must(t).NoError(txkit.OnRollback(ctx, func(ctx context.Context) error { return expectedErr }))
		assert.Must(t).ErrorIs(expectedErr, txkit.Rollback(ctx))
	})

	s.Test("on rollback, if parent rollback step encounters an error, it is propagated back as Rollback results", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		tx2, err := txkit.Begin(tx1)
		assert.Must(t).NoError(err)
		expectedErr := t.Random.Error()
		assert.Must(t).NoError(txkit.OnRollback(tx1, func(context.Context) error { return expectedErr }))
		assert.Must(t).ErrorIs(expectedErr, txkit.Rollback(tx2))
	})

	s.Test("on rollback, if rollback step encounters an error, it is propagated back through the Finish err pointer argument", func(t *testcase.T) {
		expectedErr := t.Random.Error()
		ctx, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(txkit.OnRollback(ctx, func(context.Context) error { return expectedErr }))

		expectedOthErr := t.Random.Error()
		rErr := expectedOthErr
		txkit.Finish(&rErr, ctx)
		assert.Contains(t, rErr.Error(), expectedErr.Error())
		assert.Contains(t, rErr.Error(), expectedOthErr.Error())
	})

	s.Test("transaction allows concurrent interactions", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		defer txkit.Commit(tx1)
		tx2, err := txkit.Begin(tx1)
		assert.Must(t).NoError(err)
		makingSubTx := func() {
			var rErr error
			tx, berr := txkit.Begin(tx1)
			assert.Should(t).NoError(berr)
			defer txkit.Finish(&rErr, tx)
		}
		addingRollbackStep := func() {
			assert.Should(t).NoError(txkit.OnRollback(tx2, func() {}))
		}
		testcase.Race(makingSubTx, makingSubTx, addingRollbackStep, addingRollbackStep)
	})

	s.Test("during rollback, the original context is not yet cancelled", func(t *testcase.T) {
		ctx, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(txkit.OnRollback(ctx, func(context.Context) error { return ctx.Err() }))
		assert.Must(t).NoError(txkit.Rollback(ctx))
	})

	s.Test("committing the sub context should not cancel the parent context", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		tx2, err := txkit.Begin(tx1)
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(txkit.Commit(tx2))
		assert.Must(t).NoError(tx1.Err())
		assert.Must(t).NoError(txkit.Commit(tx1))
	})

	s.Test("suppose in a multi transaction setup, the context provided for a rollback step is not cancelled, even if committed context is", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		tx2, err := txkit.Begin(tx1)
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(txkit.OnRollback(tx2, func(ctx context.Context) error { return ctx.Err() }))
		assert.Must(t).NoError(txkit.Commit(tx2))
		assert.Must(t).NoError(tx1.Err())
		assert.Must(t).NoError(txkit.Commit(tx1))
	})

	s.Test("suppose in a multi transaction setup, and the top level transaction is rolled back, the context provided for a rollback step is not cancelled", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		tx2, err := txkit.Begin(tx1)
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(txkit.OnRollback(tx2, func(ctx context.Context) error { return ctx.Err() }))
		assert.Must(t).NoError(txkit.Commit(tx2), "commit successful TX2")
		assert.Must(t).NoError(txkit.Rollback(tx1), "error at top level, rollback TX1")
	})

	s.Test("suppose a rollback is done in a sub tx, the top tx's Finish don't misinform a rollback error", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		tx2, err := txkit.Begin(tx1)
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(txkit.Rollback(tx2))
		var expectedErr error = errorkit.Error(t.Random.Error().Error())
		actualErr := expectedErr
		txkit.Finish(&actualErr, tx1)
		assert.Equal(t, expectedErr, actualErr)
	})

	s.Test("rollbacking back on multiple tx level yield no rollback error on Finish", func(t *testcase.T) {
		assertNoFinishErrOnRollback := func(ctx context.Context) {
			var expectedErr error = errorkit.Error(t.Random.Error().Error())
			actualErr := expectedErr
			txkit.Finish(&actualErr, ctx)
			assert.Must(t).Equal(expectedErr, actualErr,
				"equality check intentionally to see no error wrapping is going on")
		}
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		tx2, err := txkit.Begin(tx1)
		assert.Must(t).NoError(err)
		assertNoFinishErrOnRollback(tx2)
		assertNoFinishErrOnRollback(tx1)
	})

	s.Test("on rollback error, original value can be unwrapped", func(t *testcase.T) {
		tx1, err := txkit.Begin(context.Background())
		assert.Must(t).NoError(err)
		expectedErr := errorkit.Error(t.Random.Error().Error())
		assert.Must(t).NoError(txkit.OnRollback(tx1, func(ctx context.Context) error { return expectedErr }))
		var actualErr error = expectedErr
		txkit.Finish(&actualErr, tx1)
		assert.Must(t).ErrorIs(expectedErr, actualErr)
	})

	s.Test("on context cancellation, the context of the rollback is still not cancelled", func(t *testcase.T) {
		ctx, cancel := context.WithCancel(context.Background())

		tx1, err := txkit.Begin(ctx)
		assert.Must(t).NoError(err)
		assert.Must(t).NoError(txkit.OnRollback(tx1, func(ctx context.Context) error {
			assert.NoError(t, ctx.Err())
			return nil
		}))

		cancel() // cancel the context
		_ = txkit.Rollback(tx1)
	})
}

func MyActionWhichMutateTheSystemState(ctx context.Context) error {
	return nil
}

func RollbackForMyActionWhichMutatedTheSystemState(ctx context.Context) error {
	return nil
}

func MyUseCase(ctx context.Context) (returnErr error) {
	ctx, err := txkit.Begin(ctx)
	if err != nil {
		return err
	}
	defer txkit.Finish(&returnErr, ctx)

	if err := MyActionWhichMutateTheSystemState(ctx); err != nil {
		return err
	}

	txkit.OnRollback(ctx, func(ctx context.Context) error {
		return RollbackForMyActionWhichMutatedTheSystemState(ctx)
	})

	return nil
}

func Example_pkgLevelTxFunctions() {
	ctx := context.Background()
	ctx, err := txkit.Begin(ctx)
	if err != nil {
		logger.Error(ctx, "error with my tx", logging.ErrField(err))
	}

	if err := MyUseCase(ctx); err != nil {
		txkit.Rollback(ctx)
		return
	}
	txkit.Commit(ctx)
}

func Test_smoke(tt *testing.T) {
	t := testcase.NewT(tt)

	ctx := context.Background()
	ns := make([]int, 0, 2)

	_ = func(ctx context.Context) (rerr error) {
		tx1, _ := txkit.Begin(ctx)
		defer txkit.Finish(&rerr, tx1)

		_ = func(ctx context.Context) (rerr error) {
			tx2, _ := txkit.Begin(ctx)
			defer txkit.Finish(&rerr, tx2)
			txkit.OnRollback(tx2, func() { ns = append(ns, 42) })
			txkit.OnRollback(tx2, func() { ns = append(ns, 24) })

			// err == nil ->> CommitTx because TxFinish
			return nil
		}(tx1)

		// err != nil ->> RollbackTx because TxFinish
		return fmt.Errorf("rollback this and anything that depends on our current tx")
	}(ctx)

	assert.Must(t).Equal([]int{24, 42}, ns)
}

type CPProxy struct {
	BeginTxFn    func(ctx context.Context) (context.Context, error)
	CommitTxFn   func(ctx context.Context) error
	RollbackTxFn func(ctx context.Context) error
}

func (proxy CPProxy) BeginTx(ctx context.Context) (context.Context, error) {
	return proxy.BeginTxFn(ctx)
}

func (proxy CPProxy) CommitTx(ctx context.Context) error {
	return proxy.CommitTxFn(ctx)
}

func (proxy CPProxy) RollbackTx(ctx context.Context) error {
	return proxy.RollbackTxFn(ctx)
}

func TestManager(t *testing.T) {
	s := testcase.NewSpec(t)

	type DB struct {
		N string
		V string
	}
	type TX struct {
		ID   string
		V    string
		DB   *DB
		Done bool
	}
	type Queryable struct {
		V string
		F any
	}

	makeManager := func(t *testcase.T, db *DB, resourceID string) txkit.Manager[DB, TX, Queryable] {
		return txkit.Manager[DB, TX, Queryable]{
			R:          db,
			ResourceID: resourceID,
			ResAdapter: func(db *DB) Queryable {
				return Queryable{V: db.V, F: db}
			},
			TxAdapter: func(tx *TX) Queryable {
				return Queryable{V: tx.V, F: tx}
			},
			Begin: func(ctx context.Context, db *DB) (*TX, error) {
				return &TX{ID: t.Random.UUID(), V: db.V, DB: db}, nil
			},
			Commit: func(ctx context.Context, tx *TX) error {
				if tx.Done {
					return txkit.ErrTxDone
				}
				tx.DB.V = tx.V
				tx.Done = true
				return nil
			},
			Rollback: func(ctx context.Context, tx *TX) error {
				if tx.Done {
					return txkit.ErrTxDone
				}
				tx.Done = true
				return nil
			},
		}
	}

	var (
		initialValue = let.Var(s, func(t *testcase.T) string { return t.Random.String() })
		db           = let.Var(s, func(t *testcase.T) *DB {
			return &DB{N: t.Random.Domain(), V: initialValue.Get(t)}
		})
		resourceID = let.Var(s, func(t *testcase.T) string { return "" })
		subject    = let.Var(s, func(t *testcase.T) txkit.Manager[DB, TX, Queryable] {
			return makeManager(t, db.Get(t), resourceID.Get(t))
		})
	)

	s.Test("comprotocontracts.OnePhaseCommitProtocol", func(t *testcase.T) {
		comprotocontract.OnePhaseCommitProtocol(subject.Get(t)).Spec(testcase.NewSpec(t))
	})

	s.Test("smoke", func(t *testcase.T) {
		subject, db := subject.Get(t), db.Get(t)
		ctx := context.Background()
		q := subject.Q(ctx)
		assert.Equal(t, q.V, db.V)
		assert.Equal[any](t, q.F, db)

		tx, ok := subject.LookupTx(ctx)
		var _ *TX = tx
		assert.False(t, ok)
		assert.Nil(t, tx)

		ctx, err := subject.BeginTx(ctx)
		assert.NoError(t, err)
		t.Cleanup(func() { _ = subject.RollbackTx(ctx) })

		tx, ok = subject.LookupTx(ctx)
		assert.True(t, ok)
		assert.NotNil(t, tx)
		assert.Equal(t, tx.V, db.V)

		q = subject.Q(ctx)
		assert.Equal[any](t, q.F, tx)
	})

	s.Test("cancel will not cannel the context of the Rollback call", func(t *testcase.T) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		var subject txkit.Manager[DB, TX, Queryable] = subject.Get(t) // pass by value copy
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

	s.Test("empty ResourceID shares the default scope for the same resource pointer and TX type", func(t *testcase.T) {
		owner := subject.Get(t)
		reconstructed := makeManager(t, db.Get(t), "")
		ctx, err := owner.BeginTx(context.Background())
		assert.Must(t).NoError(err)
		t.Cleanup(func() { _ = owner.RollbackTx(ctx) })

		tx, ok := owner.LookupTx(ctx)
		assert.Must(t).True(ok)
		got, ok := reconstructed.LookupTx(ctx)
		assert.True(t, ok)
		assert.True(t, got == tx)
		assert.True(t, reconstructed.Q(ctx).F == tx)

		tx.V = random.Unique(t.Random.String, initialValue.Get(t))
		assert.NoError(t, reconstructed.CommitTx(ctx))
		assert.Equal(t, db.Get(t).V, tx.V)
	})

	s.Describe("#InTx", func(s *testcase.Spec) {
		var (
			observer = let.Var[comproto.OnePhaseCommitProtocol](s, func(t *testcase.T) comproto.OnePhaseCommitProtocol {
				return subject.Get(t)
			})
			txCtx = let.Var(s, func(t *testcase.T) context.Context {
				manager := subject.Get(t)
				ctx, err := manager.BeginTx(context.Background())
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = manager.RollbackTx(ctx) })
				return ctx
			})
			ctx = let.Var(s, txCtx.Get)
		)
		act := func(t *testcase.T) bool {
			detector, ok := observer.Get(t).(comproto.InTx)
			assert.Must(t).True(ok, "Manager must implement the optional comproto.InTx capability")
			return detector.InTx(ctx.Get(t))
		}

		s.Test("a copied manager recognizes the shared scope without completing it", func(t *testcase.T) {
			assert.True(t, act(t))
			tx, ok := subject.Get(t).LookupTx(txCtx.Get(t))
			assert.Must(t).True(ok)
			tx.V = random.Unique(t.Random.String, initialValue.Get(t))
			assert.True(t, act(t))
			assert.Equal(t, db.Get(t).V, initialValue.Get(t))
			assert.NoError(t, subject.Get(t).CommitTx(txCtx.Get(t)))
			assert.Equal(t, db.Get(t).V, tx.V)
			assert.False(t, act(t))
		})

		for _, completion := range []string{"committed", "rolled back", "canceled"} {
			s.When("the context is "+completion, func(s *testcase.Spec) {
				ctx.Let(s, func(t *testcase.T) context.Context {
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

				s.Then("reports inactive without changing native lookup or query routing", func(t *testcase.T) {
					tx, ok := subject.Get(t).LookupTx(txCtx.Get(t))
					assert.Must(t).True(ok)
					assert.False(t, act(t))
					got, ok := subject.Get(t).LookupTx(ctx.Get(t))
					assert.True(t, ok)
					assert.True(t, got == tx)
					assert.True(t, subject.Get(t).Q(ctx.Get(t)).F == tx)
				})
			})
		}

		s.When("a wrapper is reconstructed around the same resource pointer", func(s *testcase.Spec) {
			observer.Let(s, func(t *testcase.T) comproto.OnePhaseCommitProtocol {
				return makeManager(t, db.Get(t), "")
			})

			s.Then("recognizes the active scope", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		s.When("a wrapper uses the same explicit ID and TX type with a different handle", func(s *testcase.Spec) {
			resourceID.Let(s, func(t *testcase.T) string { return t.Random.UUID() })
			observer.Let(s, func(t *testcase.T) comproto.OnePhaseCommitProtocol {
				return makeManager(t, &DB{V: t.Random.String()}, resourceID.Get(t))
			})

			s.Then("recognizes the shared resource scope", func(t *testcase.T) {
				assert.True(t, act(t))
			})
		})

		for _, identity := range []string{"different resource pointer", "different explicit ID", "different TX type"} {
			s.When("the observer has a "+identity, func(s *testcase.Spec) {
				observer.Let(s, func(t *testcase.T) comproto.OnePhaseCommitProtocol {
					switch identity {
					case "different resource pointer":
						return makeManager(t, &DB{V: t.Random.String()}, "")
					case "different explicit ID":
						return makeManager(t, db.Get(t), t.Random.UUID())
					default:
						type OtherTX TX
						base := subject.Get(t)
						return txkit.Manager[DB, OtherTX, Queryable]{
							R: base.R,
							Begin: func(ctx context.Context, db *DB) (*OtherTX, error) {
								tx, err := base.Begin(ctx, db)
								return (*OtherTX)(tx), err
							},
							Commit:   func(ctx context.Context, tx *OtherTX) error { return base.Commit(ctx, (*TX)(tx)) },
							Rollback: func(ctx context.Context, tx *OtherTX) error { return base.Rollback(ctx, (*TX)(tx)) },
						}
					}
				})

				s.Then("does not recognize another resource's transaction", func(t *testcase.T) {
					assert.False(t, act(t))
				})

				s.And("both resources have transactions in the context chain", func(s *testcase.Spec) {
					ctx.Let(s, func(t *testcase.T) context.Context {
						manager := observer.Get(t)
						ctx, err := manager.BeginTx(txCtx.Get(t))
						assert.Must(t).NoError(err)
						t.Cleanup(func() { _ = manager.RollbackTx(ctx) })
						return context.WithoutCancel(ctx)
					})

					s.Then("tracks completion independently for each resource", func(t *testcase.T) {
						assert.True(t, act(t))
						owner, ok := any(subject.Get(t)).(comproto.InTx)
						assert.Must(t).True(ok)
						assert.True(t, owner.InTx(ctx.Get(t)))
						assert.NoError(t, observer.Get(t).CommitTx(ctx.Get(t)))
						assert.False(t, act(t))
						assert.True(t, owner.InTx(ctx.Get(t)))
						assert.True(t, owner.InTx(txCtx.Get(t)))
					})
				})
			})
		}
	})

	s.When("ResourceID is configured", func(s *testcase.Spec) {
		resourceID.Let(s, func(t *testcase.T) string { return t.Random.UUID() })

		var (
			otherInitialValue = let.Var(s, func(t *testcase.T) string { return t.Random.String() })
			otherDB           = let.Var(s, func(t *testcase.T) *DB {
				return &DB{N: t.Random.Domain(), V: otherInitialValue.Get(t)}
			})
			otherResourceID = let.Var(s, func(t *testcase.T) string {
				return random.Unique(t.Random.UUID, resourceID.Get(t))
			})
			other = let.Var(s, func(t *testcase.T) txkit.Manager[DB, TX, Queryable] {
				return makeManager(t, otherDB.Get(t), otherResourceID.Get(t))
			})
			valueA = let.Var(s, func(t *testcase.T) string {
				return random.Unique(t.Random.String, initialValue.Get(t))
			})
			valueB = let.Var(s, func(t *testcase.T) string {
				return random.Unique(t.Random.String, otherInitialValue.Get(t), valueA.Get(t))
			})
			ctxA = let.Var(s, func(t *testcase.T) context.Context {
				manager := subject.Get(t)
				ctx, err := manager.BeginTx(context.Background())
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = manager.RollbackTx(ctx) })
				tx, ok := manager.LookupTx(ctx)
				assert.Must(t).True(ok)
				tx.V = valueA.Get(t)
				return ctx
			})
			txA = let.Var(s, func(t *testcase.T) *TX {
				tx, ok := subject.Get(t).LookupTx(ctxA.Get(t))
				assert.Must(t).True(ok)
				return tx
			})
			ctxB = let.Var(s, func(t *testcase.T) context.Context {
				manager := other.Get(t)
				ctx, err := manager.BeginTx(ctxA.Get(t))
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = manager.RollbackTx(ctx) })
				tx, ok := manager.LookupTx(ctx)
				assert.Must(t).True(ok)
				tx.V = valueB.Get(t)
				return ctx
			})
			txB = let.Var(s, func(t *testcase.T) *TX {
				tx, ok := other.Get(t).LookupTx(ctxB.Get(t))
				assert.Must(t).True(ok)
				return tx
			})
			ctx = let.Var(s, func(t *testcase.T) context.Context { return ctxB.Get(t) })
		)

		s.Describe("#LookupTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) (*TX, bool) {
				return other.Get(t).LookupTx(ctx.Get(t))
			}

			s.Test("A and B are independently available from the A -> B context", func(t *testcase.T) {
				got, ok := act(t)
				assert.Must(t).True(ok)
				assert.True(t, got != txA.Get(t))
				assert.True(t, got.DB == otherDB.Get(t))
				assert.Equal(t, got.V, valueB.Get(t))
				gotA, ok := subject.Get(t).LookupTx(ctx.Get(t))
				assert.True(t, ok)
				assert.True(t, gotA == txA.Get(t))
				assert.Equal(t, gotA.V, valueA.Get(t))
			})

			s.When("only A has a transaction", func(s *testcase.Spec) {
				ctx.Let(s, ctxA.Get)

				s.Then("B cannot look up A's transaction", func(t *testcase.T) {
					tx, ok := act(t)
					assert.False(t, ok)
					assert.Nil(t, tx)
				})
			})
		})

		s.Describe("#Q", func(s *testcase.Spec) {
			act := func(t *testcase.T) Queryable { return other.Get(t).Q(ctx.Get(t)) }

			s.Test("each resource queries its own transaction in the A -> B context", func(t *testcase.T) {
				q := act(t)
				assert.True(t, q.F == txB.Get(t))
				assert.Equal(t, q.V, valueB.Get(t))
				qA := subject.Get(t).Q(ctx.Get(t))
				assert.True(t, qA.F == txA.Get(t))
				assert.Equal(t, qA.V, valueA.Get(t))
			})

			s.When("only A has a transaction", func(s *testcase.Spec) {
				ctx.Let(s, ctxA.Get)

				s.Then("B queries its own DB instead of A's transaction", func(t *testcase.T) {
					q := act(t)
					assert.True(t, q.F == otherDB.Get(t))
					assert.Equal(t, q.V, otherInitialValue.Get(t))
				})
			})
		})

		s.Describe("#BeginTx", func(s *testcase.Spec) {
			var (
				manager = let.Var(s, other.Get)
				parent  = let.Var(s, ctxA.Get)
			)
			act := func(t *testcase.T) (context.Context, error) {
				return manager.Get(t).BeginTx(parent.Get(t))
			}

			s.Test("a different ID starts its own transaction without replacing A", func(t *testcase.T) {
				ctx, err := act(t)
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = other.Get(t).RollbackTx(ctx) })
				got, ok := other.Get(t).LookupTx(ctx)
				assert.Must(t).True(ok)
				assert.True(t, got != txA.Get(t))
				assert.True(t, got.DB == otherDB.Get(t))
				assert.Equal(t, got.V, otherInitialValue.Get(t))
				gotA, ok := subject.Get(t).LookupTx(ctx)
				assert.True(t, ok)
				assert.True(t, gotA == txA.Get(t))
				assert.Equal(t, gotA.V, valueA.Get(t))
			})

			s.When("both IDs refer to the same DB", func(s *testcase.Spec) {
				otherDB.Let(s, db.Get)

				s.Then("the IDs still select separate transactions", func(t *testcase.T) {
					ctx, err := act(t)
					assert.Must(t).NoError(err)
					t.Cleanup(func() { _ = other.Get(t).RollbackTx(ctx) })
					got, ok := other.Get(t).LookupTx(ctx)
					assert.Must(t).True(ok)
					assert.True(t, got != txA.Get(t))
					assert.True(t, got.DB == db.Get(t))
					assert.Equal(t, got.V, initialValue.Get(t))
				})
			})

			s.When("A is entered again inside B", func(s *testcase.Spec) {
				manager.Let(s, subject.Get)
				parent.Let(s, ctxB.Get)

				s.Then("A -> B -> A reuses A through the context chain and leaves B independent", func(t *testcase.T) {
					ctx, err := act(t)
					assert.Must(t).NoError(err)
					t.Cleanup(func() { _ = subject.Get(t).RollbackTx(ctx) })
					gotA, ok := subject.Get(t).LookupTx(ctx)
					assert.True(t, ok)
					assert.True(t, gotA == txA.Get(t))
					gotB, ok := other.Get(t).LookupTx(ctx)
					assert.True(t, ok)
					assert.True(t, gotB == txB.Get(t))
					assert.True(t, subject.Get(t).Q(ctx).F == txA.Get(t))
					assert.True(t, other.Get(t).Q(ctx).F == txB.Get(t))

					assert.NoError(t, subject.Get(t).CommitTx(ctx))
					assert.False(t, txA.Get(t).Done)
					assert.False(t, txB.Get(t).Done)
					assert.Equal(t, db.Get(t).V, initialValue.Get(t))
					assert.Equal(t, otherDB.Get(t).V, otherInitialValue.Get(t))
					assert.NoError(t, other.Get(t).CommitTx(ctxB.Get(t)))
					assert.Equal(t, otherDB.Get(t).V, valueB.Get(t))
					assert.False(t, txA.Get(t).Done)
					assert.NoError(t, subject.Get(t).CommitTx(ctxA.Get(t)))
					assert.Equal(t, db.Get(t).V, valueA.Get(t))
				})
			})
		})

		s.Describe("#CommitTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) error { return other.Get(t).CommitTx(ctx.Get(t)) }

			s.Test("committing B persists only B and leaves A available for its own commit", func(t *testcase.T) {
				assert.NoError(t, act(t))
				assert.Equal(t, otherDB.Get(t).V, valueB.Get(t))
				assert.Equal(t, db.Get(t).V, initialValue.Get(t))
				assert.False(t, txA.Get(t).Done)
				assert.NoError(t, subject.Get(t).CommitTx(ctxA.Get(t)))
				assert.Equal(t, db.Get(t).V, valueA.Get(t))
			})

			s.When("only A has a transaction", func(s *testcase.Spec) {
				ctx.Let(s, ctxA.Get)

				s.Then("B returns ErrNoTx without settling A", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), txkit.ErrNoTx)
					assert.False(t, txA.Get(t).Done)
					assert.Equal(t, db.Get(t).V, initialValue.Get(t))
					assert.Equal(t, otherDB.Get(t).V, otherInitialValue.Get(t))
					assert.NoError(t, subject.Get(t).CommitTx(ctxA.Get(t)))
					assert.Equal(t, db.Get(t).V, valueA.Get(t))
				})
			})
		})

		s.Describe("#RollbackTx", func(s *testcase.Spec) {
			act := func(t *testcase.T) error { return other.Get(t).RollbackTx(ctx.Get(t)) }

			s.Test("rolling back B discards only B and leaves A available for its own commit", func(t *testcase.T) {
				transaction := txB.Get(t)
				assert.NoError(t, act(t))
				assert.True(t, transaction.Done)
				assert.Equal(t, otherDB.Get(t).V, otherInitialValue.Get(t))
				assert.Equal(t, db.Get(t).V, initialValue.Get(t))
				assert.False(t, txA.Get(t).Done)
				assert.NoError(t, subject.Get(t).CommitTx(ctxA.Get(t)))
				assert.Equal(t, db.Get(t).V, valueA.Get(t))
			})

			s.When("only A has a transaction", func(s *testcase.Spec) {
				ctx.Let(s, ctxA.Get)

				s.Then("B returns ErrNoTx without settling A", func(t *testcase.T) {
					assert.ErrorIs(t, act(t), txkit.ErrNoTx)
					assert.False(t, txA.Get(t).Done)
					assert.Equal(t, db.Get(t).V, initialValue.Get(t))
					assert.Equal(t, otherDB.Get(t).V, otherInitialValue.Get(t))
					assert.NoError(t, subject.Get(t).CommitTx(ctxA.Get(t)))
					assert.Equal(t, db.Get(t).V, valueA.Get(t))
				})
			})
		})

		s.When("the other manager uses the empty default ResourceID", func(s *testcase.Spec) {
			otherResourceID.LetValue(s, "")

			s.Then("the default scope does not share the named resource's transaction", func(t *testcase.T) {
				manager := other.Get(t)
				got, ok := manager.LookupTx(ctxA.Get(t))
				assert.False(t, ok)
				assert.Nil(t, got)
				assert.True(t, manager.Q(ctxA.Get(t)).F == otherDB.Get(t))
				assert.ErrorIs(t, manager.CommitTx(ctxA.Get(t)), txkit.ErrNoTx)
				assert.ErrorIs(t, manager.RollbackTx(ctxA.Get(t)), txkit.ErrNoTx)
				assert.True(t, txB.Get(t) != txA.Get(t))
				assert.NoError(t, manager.CommitTx(ctxB.Get(t)))
				assert.Equal(t, otherDB.Get(t).V, valueB.Get(t))
				assert.False(t, txA.Get(t).Done)
				assert.NoError(t, subject.Get(t).CommitTx(ctxA.Get(t)))
				assert.Equal(t, db.Get(t).V, valueA.Get(t))
			})
		})

		for _, mode := range []string{"copied", "reconstructed with a different DB"} {
			s.When("the manager is "+mode, func(s *testcase.Spec) {
				shared := let.Var(s, func(t *testcase.T) txkit.Manager[DB, TX, Queryable] {
					if mode == "copied" {
						return subject.Get(t)
					}
					return makeManager(t, otherDB.Get(t), resourceID.Get(t))
				})

				s.Test("matching IDs and TX types share lookup, queries, nested begin and commit", func(t *testcase.T) {
					manager := shared.Get(t)
					got, ok := manager.LookupTx(ctxA.Get(t))
					assert.True(t, ok)
					assert.True(t, got == txA.Get(t))
					assert.True(t, manager.Q(ctxA.Get(t)).F == txA.Get(t))

					ctx, err := manager.BeginTx(ctxA.Get(t))
					assert.Must(t).NoError(err)
					t.Cleanup(func() { _ = manager.RollbackTx(ctx) })
					got, ok = manager.LookupTx(ctx)
					assert.True(t, ok)
					assert.True(t, got == txA.Get(t))
					assert.NoError(t, manager.CommitTx(ctx))
					assert.False(t, txA.Get(t).Done)
					assert.Equal(t, db.Get(t).V, initialValue.Get(t))
					assert.NoError(t, manager.CommitTx(ctxA.Get(t)))
					assert.Equal(t, db.Get(t).V, valueA.Get(t))
					assert.Equal(t, otherDB.Get(t).V, otherInitialValue.Get(t))
				})

				s.Test("a matching ID and TX type can roll back the shared transaction", func(t *testcase.T) {
					transaction := txA.Get(t)
					assert.NoError(t, shared.Get(t).RollbackTx(ctxA.Get(t)))
					assert.True(t, transaction.Done)
					assert.Equal(t, db.Get(t).V, initialValue.Get(t))
					assert.Equal(t, otherDB.Get(t).V, otherInitialValue.Get(t))
				})
			})
		}

		s.Test("the same ResourceID with a different TX type remains isolated", func(t *testcase.T) {
			type OtherTX TX
			base := other.Get(t)
			manager := txkit.Manager[DB, OtherTX, Queryable]{
				R:          base.R,
				ResourceID: resourceID.Get(t),
				ResAdapter: base.ResAdapter,
				TxAdapter:  func(tx *OtherTX) Queryable { return base.TxAdapter((*TX)(tx)) },
				Begin: func(ctx context.Context, db *DB) (*OtherTX, error) {
					tx, err := base.Begin(ctx, db)
					return (*OtherTX)(tx), err
				},
				Commit:   func(ctx context.Context, tx *OtherTX) error { return base.Commit(ctx, (*TX)(tx)) },
				Rollback: func(ctx context.Context, tx *OtherTX) error { return base.Rollback(ctx, (*TX)(tx)) },
			}

			got, ok := manager.LookupTx(ctxA.Get(t))
			assert.False(t, ok)
			assert.Nil(t, got)
			assert.True(t, manager.Q(ctxA.Get(t)).F == otherDB.Get(t))
			assert.ErrorIs(t, manager.CommitTx(ctxA.Get(t)), txkit.ErrNoTx)
			assert.ErrorIs(t, manager.RollbackTx(ctxA.Get(t)), txkit.ErrNoTx)
			assert.False(t, txA.Get(t).Done)

			ctx, err := manager.BeginTx(ctxA.Get(t))
			assert.Must(t).NoError(err)
			t.Cleanup(func() { _ = manager.RollbackTx(ctx) })
			got, ok = manager.LookupTx(ctx)
			assert.Must(t).True(ok)
			assert.True(t, (*TX)(got) != txA.Get(t))
			got.V = valueB.Get(t)
			gotA, ok := subject.Get(t).LookupTx(ctx)
			assert.True(t, ok)
			assert.True(t, gotA == txA.Get(t))
			assert.True(t, manager.Q(ctx).F == (*TX)(got))
			assert.NoError(t, manager.CommitTx(ctx))
			assert.Equal(t, otherDB.Get(t).V, valueB.Get(t))
			assert.Equal(t, db.Get(t).V, initialValue.Get(t))
			assert.False(t, txA.Get(t).Done)
			assert.NoError(t, subject.Get(t).CommitTx(ctxA.Get(t)))
			assert.Equal(t, db.Get(t).V, valueA.Get(t))
		})
	})
}
