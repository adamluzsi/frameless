package crudcontract_test

import (
	"context"
	"iter"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/port/crud/crudcontract"

	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func TestQueryOne(t *testing.T) {
	mem := memory.NewMemory()
	repo := memory.NewRepository[testent.Foo, testent.FooID](mem)

	crudcontract.QueryOne[testent.Foo, testent.FooID](repo, "QueryOne",
		func(tb testing.TB) crudcontract.QueryOneSubject[testent.Foo] {
			tc := tb.(*testcase.T)
			var baz = tc.Random.String()
			v := testent.MakeFoo(tb)
			v.Baz = baz
			return crudcontract.QueryOneSubject[testent.Foo]{
				Query: func(ctx context.Context) (_ testent.Foo, found bool, _ error) {
					return repo.QueryOne(ctx, func(v testent.Foo) bool {
						return v.Baz == baz
					})
				},
				ExpectedEntity: v,
				ExcludedEntity: func() testent.Foo {
					v := testent.MakeFoo(tb)
					v.Baz = random.Unique(tc.Random.String, baz) // not baz (doesn't match the query)
					return v
				},
			}
		}).Test(t)
}

func TestQueryMany(t *testing.T) {
	mem := memory.NewMemory()
	repo := memory.NewRepository[testent.Foo, testent.FooID](mem)

	crudcontract.QueryMany[testent.Foo, testent.FooID](repo, "QueryMany",
		func(tb testing.TB) crudcontract.QueryManySubject[testent.Foo] {
			tc := tb.(*testcase.T)
			var foo = tc.Random.String()
			tb.Log("expected foo value:", foo)

			return crudcontract.QueryManySubject[testent.Foo]{
				Query: func(ctx context.Context) iter.Seq2[testent.Foo, error] {
					return iterkit.OnSeqEValue(repo.FindAll(ctx), func(itr iter.Seq[testent.Foo]) iter.Seq[testent.Foo] {
						return iterkit.Filter(itr, func(f testent.Foo) bool {
							return f.Foo == foo
						})
					})
				},
				IncludedEntity: func() testent.Foo {
					v := testent.MakeFoo(tb)
					v.Foo = foo
					return v
				},
				ExcludedEntity: func() testent.Foo {
					v := testent.MakeFoo(tb)
					v.Foo = random.Unique(tc.Random.String, foo)
					return v
				},
			}
		}).Test(t)
}
