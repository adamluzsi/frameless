package crudcontracts_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/frameless/spechelper/testent"
	"testing"
)

func TestSuiteFor(t *testing.T) {
	crudcontracts.SuiteFor[
		testent.Foo, testent.FooID,
		*memory.Repository[testent.Foo, testent.FooID],
	](func(tb testing.TB) crudcontracts.SuiteSubject[
		testent.Foo, testent.FooID,
		*memory.Repository[testent.Foo, testent.FooID],
	] {
		m := memory.NewMemory()
		r := memory.NewRepository[testent.Foo, testent.FooID](m)
		return crudcontracts.SuiteSubject[
			testent.Foo, testent.FooID,
			*memory.Repository[testent.Foo, testent.FooID],
		]{
			Resource:              r,
			CommitManager:         m,
			MakeContext:           context.Background,
			MakeEntity:            testent.MakeFooFunc(tb),
			CreateSupportIDReuse:  true,
			CreateSupportRecreate: true,
		}
	}).Test(t)
}
