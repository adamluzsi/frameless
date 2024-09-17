package comprotocontracts

import (
	"context"

	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func OnePhaseCommitProtocol(subject comproto.OnePhaseCommitProtocol, opts ...Option) contract.Contract {
	c := option.Use[Config](opts)
	s := testcase.NewSpec(nil)

	s.Context("supplies OnePhaseCommitProtocol", func(s *testcase.Spec) {
		s.HasSideEffect()

		s.Test(`BeginTx + CommitTx, no error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.CommitTx(tx))
		})

		s.Test(`CommitTx cancels the context`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.CommitTx(tx))
			t.Must.ErrorIs(tx.Err(), context.Canceled)
		})

		s.Test(`BeginTx + multiple CommitTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.CommitTx(tx))
			t.Must.NotNil(subject.CommitTx(tx))
		})

		s.Test(`BeginTx + RollbackTx, no error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.RollbackTx(tx))
		})

		s.Test(`RollbackTx cancels the context`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.RollbackTx(tx))
			t.Must.ErrorIs(tx.Err(), context.Canceled)
		})

		s.Test(`BeginTx + multiple RollbackTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.RollbackTx(tx))
			t.Must.NotNil(subject.RollbackTx(tx))
		})

		s.Test(`BeginTx + RollbackTx + CommitTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.RollbackTx(tx))
			t.Must.NotNil(subject.CommitTx(tx))
		})

		s.Test(`BeginTx + CommitTx + RollbackTx, yields error`, func(t *testcase.T) {
			tx, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)
			t.Must.Nil(subject.CommitTx(tx))
			t.Must.NotNil(subject.RollbackTx(tx))
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

			var globalContext = c.MakeContext()

			tx1, err := subject.BeginTx(globalContext)
			t.Must.Nil(err)
			t.Log(`given tx1 is began`)

			tx2InTx1, err := subject.BeginTx(tx1)
			t.Must.Nil(err)
			t.Log(`and tx2 is began using tx1 as a base`)

			t.Must.Nil(subject.CommitTx(tx2InTx1), `"inner" comproto should be considered done`)
			t.Must.NotNil(subject.CommitTx(tx2InTx1), `"inner" comproto should be already done`)

			t.Must.Nil(subject.CommitTx(tx1), `"outer" comproto should be considered done`)
			t.Must.NotNil(subject.CommitTx(tx1), `"outer" comproto should be already done`)
		})

		s.Test("CommitTx and context cancellation behaviour with nested context", func(t *testcase.T) {
			tx1, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)

			tx2, err := subject.BeginTx(tx1)
			t.Must.Nil(err)

			t.Must.NoError(subject.CommitTx(tx2)) // commit innert tx
			assert.ErrorIs(t, context.Canceled, tx2.Err())
			assert.NoError(t, tx1.Err())

			t.Must.Nil(subject.CommitTx(tx1))
			assert.ErrorIs(t, context.Canceled, tx1.Err())
		})

		s.Test("RollbackTx and context cancellation behaviour with nested context", func(t *testcase.T) {
			tx1, err := subject.BeginTx(c.MakeContext())
			t.Must.Nil(err)

			tx2, err := subject.BeginTx(tx1)
			t.Must.Nil(err)

			t.Must.NoError(subject.RollbackTx(tx2)) // commit innert tx
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
			c, cfn := context.WithCancel(c.MakeContext())
			cancel.Set(t, cfn)
			return c
		}).EagerLoading(s)

		s.Test("BeginTx returns the error", func(t *testcase.T) {
			cancel.Get(t)()
			_, err := subject.BeginTx(ctx.Get(t))
			t.Must.ErrorIs(ctx.Get(t).Err(), err)
		})

		s.Test("CommitTx returns error on context.Context.Error", func(t *testcase.T) {
			tx, err := subject.BeginTx(ctx.Get(t))
			t.Must.NoError(err)
			cancel.Get(t)()
			t.Must.ErrorIs(ctx.Get(t).Err(), subject.CommitTx(tx))
		})

		s.Test("RollbackTx returns error on context.Context.Error", func(t *testcase.T) {
			tx, err := subject.BeginTx(ctx.Get(t))
			t.Must.NoError(err)
			cancel.Get(t)()
			t.Must.ErrorIs(ctx.Get(t).Err(), subject.CommitTx(tx))
		})
	})

	return s.AsSuite("OnePhaseCommitProtocol")
}
