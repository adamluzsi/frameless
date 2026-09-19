package comprotocontract

import (
	"context"
	"time"

	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
)

func OnePhaseCommitProtocol(subject comproto.OnePhaseCommitProtocol, opts ...Option) contract.Contract {
	c := option.ToConfig[Config](opts)
	s := testcase.NewSpec(nil)

	s.Context("supplies OnePhaseCommitProtocol", func(s *testcase.Spec) {
		s.HasSideEffect()

		s.Test(`BeginTx + CommitTx, no error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.CommitTx(tx))
		})

		s.Test(`CommitTx cancels the context`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.CommitTx(tx))
			assert.Must(t).ErrorIs(tx.Err(), context.Canceled)
		})

		s.Test(`BeginTx + multiple CommitTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.CommitTx(tx))
			assert.Must(t).Error(subject.CommitTx(tx))
		})

		s.Test(`BeginTx + RollbackTx, no error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.RollbackTx(tx))
		})

		s.Test(`RollbackTx cancels the context`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.RollbackTx(tx))
			assert.Must(t).ErrorIs(tx.Err(), context.Canceled)
		})

		s.Test(`BeginTx + multiple RollbackTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.RollbackTx(tx))
			assert.Must(t).Error(subject.RollbackTx(tx))
		})

		s.Test(`BeginTx + RollbackTx + CommitTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.RollbackTx(tx))
			assert.Must(t).Error(subject.CommitTx(tx))
		})

		s.Test(`BeginTx + CommitTx + RollbackTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)
			assert.Must(t).NoError(subject.CommitTx(tx))
			assert.Must(t).Error(subject.RollbackTx(tx))
		})

		s.Test(`BeginTx should be callable multiple times to ensure an emulated multi level transaction`, func(t *testcase.T) {
			t.Log(
				`Even if the current driver or resource don't support multi level transactions`,
				`It should still accept multiple transaction begins for a given context.Context`,
				`The benefit of this is that low level components that needs to ensure transactional execution,`,
				`they should not have any knowledge about how transaction might be managed on a higher level`,
				`e.g.: domain use-case should not be aware if there is a tx used around the use-case interactor itself.`,
				``,
				`behavior of the rainy path with rollbacks is not part of the base specification`,
				`please provide further specification if your code depends on rollback in an nested transaction scenario`,
			)

			var globalContext = c.MakeContext(t)

			tx1, err := subject.BeginTx(globalContext)
			assert.Must(t).NoError(err)
			t.Log(`given tx1 is began`)

			tx2InTx1, err := subject.BeginTx(tx1)
			assert.Must(t).NoError(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			assert.Must(t).NoError(subject.CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			assert.Must(t).Error(subject.CommitTx(tx2InTx1), `"inner" comproto should be already done`)

			assert.Must(t).NoError(subject.CommitTx(tx1), `"outer" comproto should be considered done`)
			assert.Must(t).Error(subject.CommitTx(tx1), `"outer" comproto should be already done`)
		})

		s.Test("CommitTx and context cancellation behaviour with nested context", func(t *testcase.T) {
			tx1, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)

			tx2, err := subject.BeginTx(tx1)
			assert.Must(t).NoError(err)

			assert.Must(t).NoError(subject.CommitTx(tx2)) // commit innert tx
			assert.ErrorIs(t, context.Canceled, tx2.Err())
			assert.NoError(t, tx1.Err())

			assert.Must(t).NoError(subject.CommitTx(tx1))
			assert.ErrorIs(t, context.Canceled, tx1.Err())
		})

		s.Test("RollbackTx and context cancellation behaviour with nested context", func(t *testcase.T) {
			tx1, err := subject.BeginTx(c.MakeContext(t))
			assert.Must(t).NoError(err)

			tx2, err := subject.BeginTx(tx1)
			assert.Must(t).NoError(err)

			assert.Must(t).NoError(subject.RollbackTx(tx2)) // commit innert tx
			assert.ErrorIs(t, context.Canceled, tx2.Err())
			t.Log("note: We can't guarantee that rollback is not related to an error use-case")
			t.Log("      or that the commit protocol support true nested transactions")
			t.Log("      so we leave open for interpretation how the parent context cancellation should behave.")

			_ = subject.RollbackTx(tx1)
			assert.ErrorIs(t, context.Canceled, tx1.Err())
		})
	})

	s.When("context has an error", func(s *testcase.Spec) {
		cancel := testcase.Let[func()](s, nil)
		ctx := testcase.Let(s, func(t *testcase.T) context.Context {
			c, cfn := context.WithCancel(c.MakeContext(t))
			cancel.Set(t, cfn)
			return c
		}).EagerLoading(s)

		s.Test("BeginTx returns the error", func(t *testcase.T) {
			cancel.Get(t)()
			_, err := subject.BeginTx(ctx.Get(t))
			assert.Must(t).ErrorIs(ctx.Get(t).Err(), err)
		})

		s.Test("CommitTx returns error on context.Context.Error", func(t *testcase.T) {
			tx, err := subject.BeginTx(ctx.Get(t))
			assert.Must(t).NoError(err)
			cancel.Get(t)()
			assert.Must(t).ErrorIs(ctx.Get(t).Err(), subject.CommitTx(tx))
		})

		s.Test("RollbackTx returns error on context.Context.Error", func(t *testcase.T) {
			tx, err := subject.BeginTx(ctx.Get(t))
			assert.Must(t).NoError(err)
			cancel.Get(t)()
			assert.Must(t).ErrorIs(ctx.Get(t).Err(), subject.RollbackTx(tx))
		})
	})

	if detector, ok := subject.(comproto.InTx); ok {
		s.Describe("#InTx", func(s *testcase.Spec) {
			s.HasSideEffect()

			var begin = func(t *testcase.T, parent context.Context) context.Context {
				t.Helper()
				ctx, err := subject.BeginTx(parent)
				assert.Must(t).NoError(err)
				t.Cleanup(func() { _ = subject.RollbackTx(ctx) })
				return ctx
			}
			var (
				root = let.Var(s, func(t *testcase.T) context.Context {
					return begin(t, c.MakeContext(t))
				})
				inner = let.Var(s, func(t *testcase.T) context.Context {
					return begin(t, root.Get(t))
				})
				leaf = let.Var(s, func(t *testcase.T) context.Context {
					return begin(t, inner.Get(t))
				})
				ctx = let.Var(s, root.Get)
			)
			act := func(t *testcase.T) bool { return detector.InTx(ctx.Get(t)) }

			s.Test("BeginTx yields an active transaction scope", func(t *testcase.T) {
				assert.True(t, act(t))
			})

			s.When("the context has no transaction", func(s *testcase.Spec) {
				ctx.Let(s, func(t *testcase.T) context.Context { return context.Background() })

				s.Then("reports no active transaction", func(t *testcase.T) {
					assert.False(t, act(t))
				})
			})

			for _, derivation := range []struct {
				name string
				make func(*testcase.T, context.Context) context.Context
			}{
				{"WithValue", func(t *testcase.T, ctx context.Context) context.Context {
					type key struct{}
					return context.WithValue(ctx, key{}, t.Random.String())
				}},
				{"WithCancel", func(t *testcase.T, ctx context.Context) context.Context {
					ctx, cancel := context.WithCancel(ctx)
					t.Cleanup(cancel)
					return ctx
				}},
				{"WithDeadline", func(t *testcase.T, ctx context.Context) context.Context {
					ctx, cancel := context.WithDeadline(ctx, time.Now().Add(time.Hour))
					t.Cleanup(cancel)
					return ctx
				}},
				{"WithoutCancel", func(t *testcase.T, ctx context.Context) context.Context {
					return context.WithoutCancel(ctx)
				}},
			} {
				s.When("the live context is derived with "+derivation.name, func(s *testcase.Spec) {
					ctx.Let(s, func(t *testcase.T) context.Context {
						return derivation.make(t, root.Get(t))
					})

					s.Then("retains the active scope", func(t *testcase.T) {
						assert.True(t, act(t))
					})
				})
			}

			for _, expired := range []bool{false, true} {
				name := "canceled"
				if expired {
					name = "past its deadline"
				}
				s.When("a derived context is "+name, func(s *testcase.Spec) {
					ctx.Let(s, func(t *testcase.T) context.Context {
						if expired {
							ctx, cancel := context.WithDeadline(root.Get(t), time.Now().Add(-time.Second))
							t.Cleanup(cancel)
							return ctx
						}
						ctx, cancel := context.WithCancel(root.Get(t))
						cancel()
						return ctx
					})

					s.Then("reports false without completing the original scope", func(t *testcase.T) {
						assert.False(t, act(t))
						assert.True(t, detector.InTx(root.Get(t)))
					})

					s.And("the derived context's cancellation is detached", func(s *testcase.Spec) {
						ctx.Let(s, func(t *testcase.T) context.Context {
							return context.WithoutCancel(ctx.Super(t))
						})

						s.Then("still sees the uncompleted scope", func(t *testcase.T) {
							assert.True(t, act(t))
						})
					})
				})
			}

			s.When("the scope is nested", func(s *testcase.Spec) {
				ctx.Let(s, inner.Get)

				s.Then("both scopes are active", func(t *testcase.T) {
					assert.True(t, act(t))
					assert.True(t, detector.InTx(root.Get(t)))
				})
			})

			for _, completion := range []struct {
				name   string
				finish func(context.Context) error
			}{
				{"committed", subject.CommitTx},
				{"rolled back", subject.RollbackTx},
			} {
				for _, scope := range []struct {
					name     string
					current  testcase.Var[context.Context]
					finished testcase.Var[context.Context]
				}{
					{"current root", root, root},
					{"current inner", inner, inner},
					{"root ancestor", leaf, root},
					{"intermediate ancestor", leaf, inner},
				} {
					s.When("the "+scope.name+" scope is "+completion.name, func(s *testcase.Spec) {
						ctx.Let(s, scope.current.Get)
						s.Before(func(t *testcase.T) {
							ctx.Get(t) // Derive the context before completing its scope or ancestor.
							assert.Must(t).NoError(completion.finish(scope.finished.Get(t)))
						})

						assertCompleted := func(t *testcase.T) {
							assert.False(t, act(t))
							assert.False(t, detector.InTx(context.WithoutCancel(scope.finished.Get(t))))
							if completion.name == "committed" && scope.finished.ID == inner.ID {
								assert.True(t, detector.InTx(root.Get(t)), "inner commit leaves the parent active")
							}
						}
						s.Then("reports no active scope", assertCompleted)

						s.And("context cancellation is detached", func(s *testcase.Spec) {
							ctx.Let(s, func(t *testcase.T) context.Context {
								return context.WithoutCancel(ctx.Super(t))
							})

							s.Then("does not revive the completed scope or ancestor", assertCompleted)
						})
					})
				}
			}
		})
	}

	return s.AsSuite("OnePhaseCommitProtocol")
}
