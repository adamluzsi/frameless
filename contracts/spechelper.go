package contracts

import (
	"context"
	"reflect"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"

	"github.com/adamluzsi/frameless/reflects"
)

// CRD is the minimum requirements to write easily behavioral specification for a resource.
type CRD[T any, ID any] interface {
	frameless.Creator[T]
	frameless.Finder[T, ID]
	frameless.Deleter[ID]
}

const ErrIDRequired frameless.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

func cleanup[T any, ID any](tb testing.TB, ctx context.Context, t frameless.Deleter[ID]) {
	assert.Must(tb).Nil(t.DeleteAll(ctx))
}

func contains[Ent any](tb testing.TB, slice []Ent, contains Ent, msgAndArgs ...interface{}) {
	assert.Must(tb).Contain(slice, contains, msgAndArgs...)
}

func newT(T interface{}) interface{} {
	return reflect.New(reflect.TypeOf(T)).Interface()
}

func newTFunc(T interface{}) func() interface{} {
	return func() interface{} { return newT(T) }
}

func requireNotContainsList(tb testing.TB, list interface{}, listOfNotContainedElements interface{}, msgAndArgs ...interface{}) {
	tb.Helper()

	v := reflect.ValueOf(listOfNotContainedElements)
	for i := 0; i < v.Len(); i++ {
		assert.Must(tb).NotContain(list, v.Index(i).Interface(), msgAndArgs...)
	}
}

func requireContainsList(tb testing.TB, list interface{}, listOfContainedElements interface{}, msgAndArgs ...interface{}) {
	v := reflect.ValueOf(listOfContainedElements)

	for i := 0; i < v.Len(); i++ {
		assert.Must(tb).Contain(list, v.Index(i).Interface(), msgAndArgs...)
	}
}

type eventSubscriber[Ent, ID any] struct {
	TB         testing.TB
	ReturnErr  error
	ContextErr error
	Filter     func(event interface{}) bool

	events []interface{}
	errors []error
	mutex  sync.Mutex
}

func (s *eventSubscriber[Ent, ID]) HandleCreateEvent(ctx context.Context, event frameless.CreateEvent[Ent]) error {
	return s.Handle(ctx, event)
}

func (s *eventSubscriber[Ent, ID]) HandleUpdateEvent(ctx context.Context, event frameless.UpdateEvent[Ent]) error {
	return s.Handle(ctx, event)
}

func (s *eventSubscriber[Ent, ID]) HandleDeleteByIDEvent(ctx context.Context, event frameless.DeleteByIDEvent[ID]) error {
	return s.Handle(ctx, event)
}

func (s *eventSubscriber[Ent, ID]) HandleDeleteAllEvent(ctx context.Context, event frameless.DeleteAllEvent) error {
	return s.Handle(ctx, event)
}

func (s *eventSubscriber[Ent, ID]) filter(event interface{}) bool {
	if s.Filter == nil {
		return true
	}
	return s.Filter(event)
}

func (s *eventSubscriber[Ent, ID]) Handle(ctx context.Context, event interface{}) error {
	s.TB.Helper()
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.TB.Logf("%T", event)
	s.verifyContext(ctx)
	if s.filter(event) {
		s.events = append(s.events, event)
	}
	return s.ReturnErr
}

func (s *eventSubscriber[Ent, ID]) HandleError(ctx context.Context, err error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.verifyContext(ctx)
	s.errors = append(s.errors, err)
	return s.ReturnErr
}

func (s *eventSubscriber[Ent, ID]) EventsLen() int {
	return len(s.Events())
}

func (s *eventSubscriber[Ent, ID]) Events() []interface{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.events
}

func filterEventSubscriberEvents[Ent, ID, T any](s *eventSubscriber[Ent, ID]) []T {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	var out []T
	for _, e := range s.events {
		v, ok := e.(T)
		if !ok {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (s *eventSubscriber[Ent, ID]) CreateEvents() []frameless.CreateEvent[Ent] {
	return filterEventSubscriberEvents[Ent, ID, frameless.CreateEvent[Ent]](s)
}

func (s *eventSubscriber[Ent, ID]) UpdateEvents() []frameless.UpdateEvent[Ent] {
	return filterEventSubscriberEvents[Ent, ID, frameless.UpdateEvent[Ent]](s)
}

func (s *eventSubscriber[Ent, ID]) DeleteEvents() []any {
	var out []any
	for _, v := range filterEventSubscriberEvents[Ent, ID, frameless.DeleteByIDEvent[ID]](s) {
		out = append(out, v)
	}
	for _, v := range filterEventSubscriberEvents[Ent, ID, frameless.DeleteAllEvent](s) {
		out = append(out, v)
	}
	return out
}

func (s *eventSubscriber[Ent, ID]) verifyContext(ctx context.Context) {
	if s.ContextErr == nil {
		return
	}
	assert.Must(s.TB).NotNil(ctx)
	assert.Must(s.TB).Equal(s.ContextErr, ctx.Err())
}

var ctxVar = testcase.Var[context.Context]{
	ID: "context.Context",
	Init: func(t *testcase.T) context.Context {
		return context.Background()
	},
}

var (
	subscriberFilter = testcase.Var[func(interface{}) bool]{
		ID: `subscriber event filter`,
		Init: func(t *testcase.T) func(interface{}) bool {
			return func(interface{}) bool { return true }
		},
	}
)

func letSubscription[Ent, ID any](s *testcase.Spec) testcase.Var[frameless.Subscription] {
	return testcase.Let[frameless.Subscription](s, nil)
}

func newEventSubscriber[Ent, ID any](tb testing.TB, filter func(interface{}) bool) *eventSubscriber[Ent, ID] {
	return &eventSubscriber[Ent, ID]{TB: tb, Filter: filter}
}

func createSubscriptionFilter[Ent any](event interface{}) bool {
	if _, ok := event.(frameless.CreateEvent[Ent]); ok {
		return true
	}
	return false
}

func updateSubscriptionFilter[Ent any](event interface{}) bool {
	if _, ok := event.(frameless.UpdateEvent[Ent]); ok {
		return true
	}
	return false
}

func deleteSubscriptionFilter[ID any](event interface{}) bool {
	if _, ok := event.(frameless.DeleteAllEvent); ok {
		return true
	}
	if _, ok := event.(frameless.DeleteByIDEvent[ID]); ok {
		return true
	}
	return false
}

func letSubscriber[Ent, ID any](s *testcase.Spec, filter func(interface{}) bool) testcase.Var[*eventSubscriber[Ent, ID]] {
	return testcase.Let(s, func(t *testcase.T) *eventSubscriber[Ent, ID] {
		return newEventSubscriber[Ent, ID](t, filter)
	})
}

func genEntities[T any](t *testcase.T, MakeEnt func(testing.TB) T) []*T {
	var es []*T
	count := t.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		ent := MakeEnt(t)
		es = append(es, &ent)
	}
	return es
}

var factory = testcase.Var[frameless.FixtureFactory]{ID: "FixtureFactory"}

func factoryLet(s *testcase.Spec, fff func(testing.TB) frameless.FixtureFactory) {
	factory.Let(s, func(t *testcase.T) frameless.FixtureFactory { return fff(t) })
}

func factoryGet(t *testcase.T) frameless.FixtureFactory {
	return factory.Get(t)
}

func toPtr[T any](v T) *T { return &v }

func base(e interface{}) interface{} {
	return reflects.BaseValueOf(e).Interface()
}

func typeAssertTo[ToType any](in []any) (out []ToType) {
	for _, e := range in {
		out = append(out, e.(ToType))
	}
	return out
}
