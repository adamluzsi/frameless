package contracts

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/testcase"
	"reflect"
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
