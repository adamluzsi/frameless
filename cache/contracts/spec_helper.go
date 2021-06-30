package contracts

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/testcase"
	"reflect"
	"testing"
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

func factoryLet(s *testcase.Spec, ff func(testing.TB) contracts.FixtureFactory) {
	factory.Let(s, func(t *testcase.T) interface{} {
		return ff(t)
	})
}

func factoryGet(t *testcase.T) contracts.FixtureFactory {
	return factory.Get(t).(contracts.FixtureFactory)
}
