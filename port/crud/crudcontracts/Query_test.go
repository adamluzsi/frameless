package crudcontracts_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

func TestQueryOne(t *testing.T) {
	mem := memory.NewMemory()
	repo := memory.NewRepository[testent.Foo, testent.FooID](mem)

	crudcontracts.QueryOne[testent.Foo, testent.FooID](repo,
		func(tb testing.TB) crudcontracts.QueryOneSubject[testent.Foo] {
			tc := tb.(*testcase.T)
			var baz = tc.Random.String()
			v := testent.MakeFoo(tb)
			v.Baz = baz
			return crudcontracts.QueryOneSubject[testent.Foo]{
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

	crudcontracts.QueryMany[testent.Foo, testent.FooID](repo,
		func(tb testing.TB) crudcontracts.QueryManySubject[testent.Foo] {
			tc := tb.(*testcase.T)
			var foo = tc.Random.String()
			tb.Log("expected foo value:", foo)

			return crudcontracts.QueryManySubject[testent.Foo]{
				Query: func(ctx context.Context) (iterators.Iterator[testent.Foo], error) {
					iter, err := repo.FindAll(ctx)
					if err != nil {
						return nil, err
					}
					return iterators.Filter(iter, func(f testent.Foo) bool {
						return f.Foo == foo
					}), nil
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
