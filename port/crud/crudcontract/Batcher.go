package crudcontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/internal/spechelper"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func Batcher[ENT, ID any, Batch crud.Batch[ENT]](subject crud.Batcher[ENT, Batch], opts ...Option[ENT, ID]) contract.Contract {
	c := option.ToConfig(opts)
	s := testcase.NewSpec(nil)

	crt, CreatorOK := subject.(crud.Creator[ENT])
	byIDD, byIDDeleterOK := subject.(crud.ByIDDeleter[ID])
	byIDF, ByIDFinderOK := subject.(crud.ByIDFinder[ENT, ID])
	allF, AllFinderOK := subject.(crud.AllFinder[ENT])

	var (
		ctxVar = testcase.Let(s, func(t *testcase.T) context.Context {
			return c.MakeContext(t)
		})
		values = testcase.Let(s, func(t *testcase.T) []ENT {
			return random.Slice(t.Random.IntBetween(3, 7), func() ENT {
				return c.MakeEntity(t)
			})
		})
	)
	act := func(t *testcase.T) error {
		var do = func() error {
			ctx := ctxVar.Get(t)
			batch := subject.Batch(ctx)
			for _, v := range values.Get(t) {
				if err := batch.Add(v); err != nil {
					return err
				}
			}
			return batch.Close()
		}
		var err = do()
		if err == nil {
			t.Defer(spechelper.TryCleanup, t, c.MakeContext(t), subject)
		}
		return err
	}

	s.Before(func(t *testcase.T) {
		spechelper.TryCleanup(t, c.MakeContext(t), subject)
		// since we don't know what data we puked into the system by ID
		// we just try to clean everything up.
		t.Defer(spechelper.TryCleanup, t, c.MakeContext(t), subject)
	})

	s.Then(`it should succeed with the batch operation`, func(t *testcase.T) {
		assert.NoError(t, act(t))
	})

	s.Test(`Batch#Close is idempotent and repeatable`, func(t *testcase.T) {
		batch := subject.Batch(ctxVar.Get(t))

		for _, v := range values.Get(t) {
			assert.NoError(t, batch.Add(v))
		}

		t.Random.Repeat(3, 7, func() {
			assert.NoError(t, batch.Close())
		})
	})

	if AllFinderOK {
		s.Then(`added entities can be retrieved with #FindAll`, func(t *testcase.T) {
			assert.NoError(t, act(t))

			t.Eventually(func(t *testcase.T) {
				gotVS, err := iterkit.CollectE(allF.FindAll(c.MakeContext(t)))
				assert.NoError(t, err)

				for _, exp := range values.Get(t) {
					assert.OneOf(t, gotVS, func(t testing.TB, got ENT) {
						if id, ok := c.IDA.Lookup(exp); ok {
							if zerokit.IsZero(id) {
								// nuke v's ID field to zero to make comparison easier
								var zeroID ID
								c.IDA.Set(&got, zeroID)
							} else {
								assert.Equal(t, c.IDA.Get(exp), id)
							}
						}
						assert.Equal(t, exp, got)
					})
				}
			})
		})
	}

	s.When(`ctx arg has an error`, func(s *testcase.Spec) {
		ctxVar.Let(s, func(t *testcase.T) context.Context {
			ctx, cancel := context.WithCancel(c.MakeContext(t))
			cancel()
			return ctx
		})

		s.Then(`it expected to return with Context's error`, func(t *testcase.T) {
			assert.Must(t).ErrorIs(ctxVar.Get(t).Err(), act(t))
		})
	})

	s.Context(`ctx arg has an error eventually`, func(s *testcase.Spec) {
		values.Let(s, func(t *testcase.T) []ENT {
			// non zero slice list
			return random.Slice(t.Random.IntBetween(3, 7), func() ENT {
				return c.MakeEntity(t)
			})
		})

		s.Test("during Batch#Add", func(t *testcase.T) {
			ctx, cancel := context.WithCancel(c.MakeContext(t))

			last, ok := slicekit.Last(values.Get(t))
			assert.True(t, ok)

			batch := subject.Batch(ctx)
			defer batch.Close()

			for i := 0; i < len(values.Get(t))-1; i++ {
				assert.NoError(t, batch.Add(values.Get(t)[i]))
			}

			cancel()

			assert.Error(t, batch.Add(last))
			assert.Error(t, batch.Close())
		})

		s.Test("during Batch#Close", func(t *testcase.T) {
			ctx, cancel := context.WithCancel(c.MakeContext(t))

			var batch = subject.Batch(ctx)
			defer batch.Close()

			for _, v := range values.Get(t) {
				assert.NoError(t, batch.Add(v))
			}

			cancel()

			assert.Error(t, batch.Close())
		})
	})

	if (c.SupportIDReuse || c.SupportRecreate) && byIDDeleterOK {
		s.When(`entity ID is provided ahead of time`, func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				for i, value := range values.Get(t) {
					if _, hasID := lookupNonZeroID(c, value); hasID {
						continue
					}

					if !byIDDeleterOK {
						t.Skipf("unable to finish test as MakeEntity doesn't supply ID, and %T doesn't implement crud.ByIDDeleter", subject)
					}

					if !CreatorOK {
						t.Skipf("unable to finish test as MakeEntity doesn't supply ID, and %T doesn't implement crud.Creator", subject)
					}

					assert.NoError(t, crt.Create(c.MakeContext(t), &value))
					values.Get(t)[i] = value

					id := c.IDA.Get(value)
					if ByIDFinderOK {
						c.Helper().IsPresent(t, byIDF, c.MakeContext(t), id)
					}

					assert.Must(t).NoError(byIDD.DeleteByID(c.MakeContext(t), id))

					if ByIDFinderOK {
						c.Helper().IsAbsent(t, byIDF, c.MakeContext(t), id)
					}
				}
			})

			s.Then(`it will accept it`, func(t *testcase.T) {
				assert.Must(t).NoError(act(t))
			})

			if ByIDFinderOK {
				s.Then(`the persisted objects can be found with #FindByID`, func(t *testcase.T) {
					assert.Must(t).NoError(act(t))

					for _, value := range values.Get(t) {
						c.Helper().IsPresent(t, byIDF, c.MakeContext(t), c.IDA.Get(value))
					}
				})
			}

			if AllFinderOK {
				s.Then(`the persisted objects are retrieved with #FindAll`, func(t *testcase.T) {
					assert.Must(t).NoError(act(t))

					t.Eventually(func(t *testcase.T) {
						for _, exp := range values.Get(t) {
							id := c.IDA.Get(exp)
							// if zerokit.IsZero(id) {
							// 	continue
							// }

							assert.AnyOf(t, func(a *assert.A) {
								for got, err := range allF.FindAll(c.MakeContext(t)) {
									assert.NoError(t, err)

									a.Case(func(t testing.TB) {
										assert.Equal(t, id, c.IDA.Get(got))
									})

									if a.OK() {
										break
									}
								}
							})
						}
					})
				})
			}
		})
	}

	if c.OnePhaseCommit != nil {
		s.Context("OnePhaseCommitProtocol", func(s *testcase.Spec) {
			s.Test(`BeginTx -> Batch#Add -> Batch#Close -> CommitTx = value is persisted`, func(t *testcase.T) {
				tx, err := c.OnePhaseCommit.BeginTx(c.MakeContext(t))
				assert.Must(t).NoError(err)

				batch := subject.Batch(tx)
				exp := c.MakeEntity(t)
				assert.NoError(t, batch.Add(exp))
				assert.NoError(t, batch.Close())
				assert.NoError(t, c.OnePhaseCommit.CommitTx(tx))

				if AllFinderOK {
					var zeroID ID
					assert.NoError(t, c.IDA.Set(&exp, zeroID))
					t.Eventually(func(t *testcase.T) {
						for got, err := range allF.FindAll(c.MakeContext(t)) {
							assert.NoError(t, err)
							assert.NoError(t, c.IDA.Set(&got, zeroID))
							assert.NoError(t, c.IDA.Set(&got, zeroID))
							assert.Equal(t, exp, got)
						}
					})
				}
			})

			s.Test(`BeginTx -> Batch#Add -> Batch#Close -> RollbackTx = value is not persisted`, func(t *testcase.T) {
				tx, err := c.OnePhaseCommit.BeginTx(c.MakeContext(t))
				assert.Must(t).NoError(err)

				batch := subject.Batch(tx)
				exp := c.MakeEntity(t)
				assert.NoError(t, batch.Add(exp))
				assert.NoError(t, batch.Close())
				assert.NoError(t, c.OnePhaseCommit.RollbackTx(tx))

				if AllFinderOK {
					var zeroID ID
					assert.NoError(t, c.IDA.Set(&exp, zeroID))
					t.Eventually(func(t *testcase.T) {
						for got, err := range allF.FindAll(c.MakeContext(t)) {
							assert.NoError(t, err)
							assert.NoError(t, c.IDA.Set(&got, zeroID))
							assert.NoError(t, c.IDA.Set(&got, zeroID))
							assert.NotEqual(t, exp, got)
						}
					})
				}
			})

			s.Test(`finished transaction casuses issue for batch adding`, func(t *testcase.T) {
				tx, err := c.OnePhaseCommit.BeginTx(c.MakeContext(t))
				assert.Must(t).NoError(err)

				t.Log("given the transaction is finished prior to the batch call")
				assert.NoError(t, random.Pick(t.Random, c.OnePhaseCommit.CommitTx, c.OnePhaseCommit.RollbackTx)(tx))

				batch := subject.Batch(tx)
				defer batch.Close() // to ensure that batch is closed

				assert.AnyOf(t, func(a *assert.A) {
					a.Case(func(t testing.TB) { assert.Error(t, batch.Add(c.MakeEntity(t))) })
					a.Case(func(t testing.TB) { assert.Error(t, batch.Close()) })
				})
			})
		})
	}

	return s.AsSuite("Batcher")
}
