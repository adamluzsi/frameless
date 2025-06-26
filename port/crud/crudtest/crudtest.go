package crudtest

import (
	"context"
	"fmt"
	"iter"
	"reflect"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/resilience"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/pp"
)

type Helper[ENT, ID any] struct {
	Waiter resilience.Waiter
	IDA    extid.Accessor[ENT, ID]
}

type Resource[ENT, ID any] interface{}

func (a Helper[ENT, ID]) eventually() assert.Retry {
	return assert.Retry{Strategy: a.Waiter}
}

var Waiter = resilience.Waiter{
	WaitDuration: time.Millisecond,
	Timeout:      5 * time.Second,
}

var Eventually = assert.Retry{Strategy: &Waiter}

type Config[ENT, ID any] struct {
	IDA extid.Accessor[ENT, ID]
}

func (c Config[ENT, ID]) Configure(t *Config[ENT, ID]) {
	*t = reflectkit.MergeStruct(*t, c)
}

type Option[ENT, ID any] option.Option[Config[ENT, ID]]

func makeAsserter[ENT, ID any](opts []Option[ENT, ID]) Helper[ENT, ID] {
	c := option.ToConfig(opts)
	return Helper[ENT, ID]{
		Waiter: Waiter,
		IDA:    c.IDA,
	}
}

func HasID[ENT, ID any](tb testing.TB, ent *ENT, opts ...Option[ENT, ID]) (id ID) {
	tb.Helper()
	return makeAsserter(opts).HasID(tb, ent)
}

func (a Helper[ENT, ID]) HasID(tb testing.TB, ptr *ENT) ID {
	tb.Helper()
	assert.NotNil(tb, ptr)
	id, idOK := a.IDA.Lookup(*ptr)
	if !idOK && reflect.ValueOf(id).CanInt() {
		testcase.OnFail(tb, func() {
			tb.Logf("%s has a int based ID with zero as value,", reflectkit.TypeOf[ENT]().String())
			tb.Log("which is accepted as many system use index style ID sequences where the first valid ID is the value of zero.")
			tb.Log("In case your system doesn't meant to accept zero int as valid ID, this might be a root cause for this test failure.")
		})
		return id
	}
	assert.True(tb, idOK, assert.MessageF("expected to find external ID in %s", pp.Format(ptr)))
	assert.NotEmpty(tb, id)
	return id
}

func IsPresent[ENT, ID any](tb testing.TB, resource crud.ByIDFinder[ENT, ID], ctx context.Context, id ID, opts ...Option[ENT, ID]) *ENT {
	tb.Helper()
	return makeAsserter[ENT, ID](opts).IsPresent(tb, resource, ctx, id)
}

func (a Helper[ENT, ID]) IsPresent(tb testing.TB, resource crud.ByIDFinder[ENT, ID], ctx context.Context, id ID) *ENT {
	tb.Helper()
	var ent ENT
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be findable", new(ENT), id)
	a.eventually().Assert(tb, func(it assert.It) {
		e, found, err := resource.FindByID(ctx, id)
		it.Must.NoError(err)
		it.Must.True(found, assert.Message(errMessage))
		ent = e
	})
	return &ent
}

func IsAbsent[ENT, ID any](tb testing.TB, resource crud.ByIDFinder[ENT, ID], ctx context.Context, id ID, opts ...Option[ENT, ID]) {
	tb.Helper()
	a := makeAsserter[ENT, ID](opts)
	a.IsAbsent(tb, resource, ctx, id)
}

func (a Helper[ENT, ID]) IsAbsent(tb testing.TB, subject crud.ByIDFinder[ENT, ID], ctx context.Context, id ID) {
	tb.Helper()
	errMessage := fmt.Sprintf("it was expected that %T with id %#v will be absent", *new(ENT), id)
	a.eventually().Assert(tb, func(it assert.It) {
		_, found, err := subject.FindByID(ctx, id)
		it.Must.NoError(err)
		it.Must.False(found, assert.Message(errMessage))
	})
}

func HasEntity[ENT, ID any](tb testing.TB, subject crud.ByIDFinder[ENT, ID], ctx context.Context, ptr *ENT, opts ...Option[ENT, ID]) {
	tb.Helper()
	makeAsserter(opts).HasEntity(tb, subject, ctx, ptr)
}

func (a Helper[ENT, ID]) HasEntity(tb testing.TB, subject crud.ByIDFinder[ENT, ID], ctx context.Context, ptr *ENT) {
	tb.Helper()
	id := a.HasID(tb, ptr)
	a.eventually().Assert(tb, func(it assert.It) {
		// IsPresent yields the currently found value
		// that might be not yet the value we expect to see
		// so the .Assert block ensure multiple tries
		assert.Equal(it, ptr, a.IsPresent(it, subject, ctx, id))
	})
}

func Save[ENT, ID any](tb testing.TB, resource crud.Saver[ENT], ctx context.Context, ptr *ENT, opts ...Option[ENT, ID]) {
	tb.Helper()
	makeAsserter(opts).Save(tb, resource, ctx, ptr)
}

func (a Helper[ENT, ID]) Save(tb testing.TB, resource crud.Saver[ENT], ctx context.Context, ptr *ENT) {
	tb.Helper()
	assert.NoError(tb, resource.Save(ctx, ptr))
	a.cleanupENT(tb, resource, ctx, ptr)
}

func Create[ENT, ID any](tb testing.TB, resource crud.Creator[ENT], ctx context.Context, ptr *ENT, opts ...Option[ENT, ID]) {
	tb.Helper()
	makeAsserter(opts).Create(tb, resource, ctx, ptr)
}

func (a Helper[ENT, ID]) Create(tb testing.TB, resource crud.Creator[ENT], ctx context.Context, ptr *ENT) {
	tb.Helper()
	assert.NoError(tb, resource.Create(ctx, ptr))
	a.cleanupENT(tb, resource, ctx, ptr)
}

type updater[ENT, ID any] interface {
	crud.Updater[ENT]
	crud.ByIDFinder[ENT, ID]
	crud.ByIDDeleter[ID]
}

func Update[ENT, ID any](tb testing.TB, resource updater[ENT, ID], ctx context.Context, ptr *ENT, opts ...Option[ENT, ID]) {
	tb.Helper()
	makeAsserter(opts).Update(tb, resource, ctx, ptr)
}

func (a Helper[ENT, ID]) Update(tb testing.TB, resource updater[ENT, ID], ctx context.Context, ptr *ENT) {
	tb.Helper()
	assert.NotNil(tb, ptr)
	id := a.IDA.Get(*ptr)
	// ensures that by the time Update is called,
	// the entity is present in the resource.
	a.IsPresent(tb, resource, ctx, id) // wait for entity presence
	assert.NoError(tb, resource.Update(ctx, ptr))
	a.eventually().Assert(tb, func(it assert.It) {
		entity := a.IsPresent(it, resource, ctx, id)
		assert.Equal(it, ptr, entity)
	})
}

func Delete[ENT, ID any](tb testing.TB, resource crud.ByIDDeleter[ID], ctx context.Context, ptr *ENT, opts ...Option[ENT, ID]) {
	tb.Helper()
	makeAsserter(opts).Delete(tb, resource, ctx, ptr)
}

func (a Helper[ENT, ID]) Delete(tb testing.TB, resource crud.ByIDDeleter[ID], ctx context.Context, ptr *ENT) {
	tb.Helper()
	id := a.HasID(tb, ptr)
	if finder, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
		a.IsPresent(tb, finder, ctx, id)
	}
	assert.NoError(tb, resource.DeleteByID(ctx, id))
	if finder, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
		a.IsAbsent(tb, finder, ctx, id)
	}
}

type deleteAllDeleter[ENT, ID any] interface {
	crud.AllFinder[ENT]
	crud.AllDeleter
}

func DeleteAll[ENT, ID any](tb testing.TB, resource deleteAllDeleter[ENT, ID], ctx context.Context, opts ...Option[ENT, ID]) {
	tb.Helper()
	makeAsserter(opts).DeleteAll(tb, resource, ctx)
}

func (a Helper[ENT, ID]) DeleteAll(tb testing.TB, subject deleteAllDeleter[ENT, ID], ctx context.Context) {
	tb.Helper()
	assert.NoError(tb, subject.DeleteAll(ctx))
	// a.Waiter.Wait() // TODO: FIXME: race condition between tests might depend on this
	a.eventually().Assert(tb, func(t assert.It) {
		vs, err := iterkit.CollectE(subject.FindAll(ctx))
		assert.NoError(t, err)
		assert.Empty(t, vs, `no entity was expected to be found`)
	})
}

func CountIs[T any](tb testing.TB, itr iter.Seq[T], expected int) {
	tb.Helper()
	makeAsserter[T, any](nil).CountIs(tb, itr, expected)
}

func (a Helper[ENT, ID]) CountIs(tb testing.TB, iter iter.Seq[ENT], expected int) {
	tb.Helper()
	assert.Must(tb).Equal(expected, iterkit.Count(iter))
}

func (a Helper[ENT, ID]) cleanupENT(tb testing.TB, resource any, ctx context.Context, ptr *ENT) {
	tb.Helper()
	id := a.HasID(tb, ptr)
	tb.Cleanup(func() {
		if del, ok := resource.(crud.ByIDDeleter[ID]); ok {
			_ = del.DeleteByID(ctx, id)
			return
		}
		if del, ok := resource.(crud.AllDeleter); ok {
			_ = del.DeleteAll(ctx)
			return
		}
		tb.Logf("skipping cleanup as %T doesn't implement crud.ByIDDeleter", resource)
		tb.Logf("make sure to manually clean up %T#%v", *new(ENT), id)
	})
	if finder, ok := resource.(crud.ByIDFinder[ENT, ID]); ok {
		a.IsPresent(tb, finder, ctx, id)
	}
}
