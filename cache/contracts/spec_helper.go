package contracts

import (
	"context"
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/testcase"
)

var ctx = testcase.Var[context.Context]{
	ID: `context.Context`,
	Init: func(t *testcase.T) context.Context {
		return context.Background()
	},
}

func ctxGet(t *testcase.T) context.Context {
	return ctx.Get(t)
}

func newT(T frameless.T) interface{} {
	return reflect.New(reflect.TypeOf(T)).Interface()
}

var factory = testcase.Var[frameless.FixtureFactory]{ID: "fixture factory"}

func factoryLet(s *testcase.Spec, ff func(testing.TB) frameless.FixtureFactory) {
	factory.Let(s, func(t *testcase.T) frameless.FixtureFactory {
		return ff(t)
	})
}

func factoryGet(t *testcase.T) frameless.FixtureFactory {
	return factory.Get(t).(frameless.FixtureFactory)
}
