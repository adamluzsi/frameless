package spechelper

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/errutils"
	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"reflect"
	"sync"
	"testing"

	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

// CRD is the minimum requirements to write easily behavioral specification for a resource.
type CRD[Ent, ID any] interface {
	crud.Creator[Ent]
	crud.ByIDFinder[Ent, ID]
	crud.ByIDDeleter[ID]
}

const ErrIDRequired errutils.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

func TryCleanup(tb testing.TB, ctx context.Context, resource any) bool {
	tb.Helper()
	if purger, ok := resource.(crud.Purger); ok {
		assert.Must(tb).Nil(purger.Purge(ctx))
		return true
	}
	if deleter, ok := resource.(crud.AllDeleter); ok {
		assert.Must(tb).Nil(deleter.DeleteAll(ctx))
		return true
	}
	return false
}

func Cleanup(tb testing.TB, ctx context.Context, t crud.AllDeleter) {
	assert.Must(tb).Nil(t.DeleteAll(ctx))
}

func Contains[Ent any](tb testing.TB, slice []Ent, contains Ent, msgAndArgs ...interface{}) {
	assert.Must(tb).Contain(slice, contains, msgAndArgs...)
}

func NewT(T interface{}) interface{} {
	return reflect.New(reflect.TypeOf(T)).Interface()
}

func NewTFunc(T interface{}) func() interface{} {
	return func() interface{} { return NewT(T) }
}

func RequireNotContainsList(tb testing.TB, list interface{}, listOfNotContainedElements interface{}, msgAndArgs ...interface{}) {
	tb.Helper()

	v := reflect.ValueOf(listOfNotContainedElements)
	for i := 0; i < v.Len(); i++ {
		assert.Must(tb).NotContain(list, v.Index(i).Interface(), msgAndArgs...)
	}
}

func RequireContainsList(tb testing.TB, list interface{}, listOfContainedElements interface{}, msgAndArgs ...interface{}) {
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

func (s *eventSubscriber[Ent, ID]) HandleCreateEvent(ctx context.Context, event pubsub.CreateEvent[Ent]) error {
	return s.Handle(ctx, event)
}

func (s *eventSubscriber[Ent, ID]) HandleUpdateEvent(ctx context.Context, event pubsub.UpdateEvent[Ent]) error {
	return s.Handle(ctx, event)
}

func (s *eventSubscriber[Ent, ID]) HandleDeleteByIDEvent(ctx context.Context, event pubsub.DeleteByIDEvent[ID]) error {
	return s.Handle(ctx, event)
}

func (s *eventSubscriber[Ent, ID]) HandleDeleteAllEvent(ctx context.Context, event pubsub.DeleteAllEvent) error {
	if s.TB != nil {
		s.TB.Helper()
	}
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

func (s *eventSubscriber[Ent, ID]) CreateEvents() []pubsub.CreateEvent[Ent] {
	return filterEventSubscriberEvents[Ent, ID, pubsub.CreateEvent[Ent]](s)
}

func (s *eventSubscriber[Ent, ID]) UpdateEvents() []pubsub.UpdateEvent[Ent] {
	return filterEventSubscriberEvents[Ent, ID, pubsub.UpdateEvent[Ent]](s)
}

func (s *eventSubscriber[Ent, ID]) DeleteEvents() []any {
	var out []any
	for _, v := range filterEventSubscriberEvents[Ent, ID, pubsub.DeleteByIDEvent[ID]](s) {
		out = append(out, v)
	}
	for _, v := range filterEventSubscriberEvents[Ent, ID, pubsub.DeleteAllEvent](s) {
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

var ContextVar = testcase.Var[context.Context]{
	ID: "context.Context",
	Init: func(t *testcase.T) context.Context {
		return context.Background()
	},
}

var (
	SubscriberFilter = testcase.Var[func(interface{}) bool]{
		ID: `subscriber event filter`,
		Init: func(t *testcase.T) func(interface{}) bool {
			return func(interface{}) bool { return true }
		},
	}
)

func LetSubscription[Ent, ID any](s *testcase.Spec) testcase.Var[pubsub.Subscription] {
	return testcase.Let[pubsub.Subscription](s, nil)
}

func NewEventSubscriber[Ent, ID any](tb testing.TB, filter func(interface{}) bool) *eventSubscriber[Ent, ID] {
	return &eventSubscriber[Ent, ID]{TB: tb, Filter: filter}
}

func CreateSubscriptionFilter[Ent any](event interface{}) bool {
	if _, ok := event.(pubsub.CreateEvent[Ent]); ok {
		return true
	}
	return false
}

func UpdateSubscriptionFilter[Ent any](event interface{}) bool {
	if _, ok := event.(pubsub.UpdateEvent[Ent]); ok {
		return true
	}
	return false
}

func DeleteSubscriptionFilter[ID any](event interface{}) bool {
	if _, ok := event.(pubsub.DeleteAllEvent); ok {
		return true
	}
	if _, ok := event.(pubsub.DeleteByIDEvent[ID]); ok {
		return true
	}
	return false
}

func LetSubscriber[Ent, ID any](s *testcase.Spec, filter func(interface{}) bool) testcase.Var[*eventSubscriber[Ent, ID]] {
	return testcase.Let(s, func(t *testcase.T) *eventSubscriber[Ent, ID] {
		return NewEventSubscriber[Ent, ID](t, filter)
	})
}

func GenEntities[T any](t *testcase.T, MakeEnt func(testing.TB) T) []*T {
	var es []*T
	count := t.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		ent := MakeEnt(t)
		es = append(es, &ent)
	}
	return es
}

func ToPtr[T any](v T) *T { return &v }

func Base(e interface{}) interface{} {
	return reflects.BaseValueOf(e).Interface()
}

func TypeAssertTo[ToType any](in []any) (out []ToType) {
	for _, e := range in {
		out = append(out, e.(ToType))
	}
	return out
}
