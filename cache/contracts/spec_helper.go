package contracts

import (
	"context"
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/testcase"
)

var ctx = testcase.Var{
	Name: `context.Context`,
	Init: func(t *testcase.T) interface{} {
		return context.Background()
	},
}

func ctxGet(t *testcase.T) context.Context {
	return ctx.Get(t).(context.Context)
}

func newT(T frameless.T) interface{} {
	return reflect.New(reflect.TypeOf(T)).Interface()
}

var factory = testcase.Var{Name: "fixture factory"}

func factoryLet(s *testcase.Spec, ff func(testing.TB) frameless.FixtureFactory) {
	factory.Let(s, func(t *testcase.T) interface{} {
		return ff(t)
	})
}

func factoryGet(t *testcase.T) frameless.FixtureFactory {
	return factory.Get(t).(frameless.FixtureFactory)
}
