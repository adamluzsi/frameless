package contracts

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/adamluzsi/testcase/fixtures"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/reflects"
)

type T = frameless.T

var benchmarkEntityVolumeCount int

func init() {
	benchmarkEntityVolumeCount = 128

	bsc, ok := os.LookupEnv(`BENCHMARK_ENTITY_VOLUME_COUNT`)
	if !ok {
		return
	}

	i, err := strconv.Atoi(bsc)
	if err != nil {
		fmt.Println(fmt.Sprintf(`WARNING - BENCHMARK_ENTITY_VOLUME_COUNT env var value not convertable to int, will be ignored`))
		return
	}

	benchmarkEntityVolumeCount = i
}

const ErrIDRequired frameless.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

// CRD is the minimum requirements to write easily behavioral specification for a resource.
type CRD interface {
	frameless.Creator
	frameless.Finder
	frameless.Deleter
}

func createEntities(f FixtureFactory, T interface{}) []interface{} {
	var es []interface{}
	for i := 0; i < benchmarkEntityVolumeCount; i++ {
		es = append(es, CreatePTR(f, T))
	}
	return es
}

func saveEntities(tb testing.TB, s CRD, f FixtureFactory, es ...interface{}) []interface{} {
	var ids []interface{}
	for _, e := range es {
		require.Nil(tb, s.Create(f.Context(), e))
		CreateEntity(tb, s, f.Context(), e)
		ids = append(ids, HasID(tb, e))
	}
	return ids
}

func cleanup(tb testing.TB, t frameless.Deleter, f FixtureFactory, T interface{}) {
	require.Nil(tb, t.DeleteAll(f.Context()))
}

func contains(tb testing.TB, slice interface{}, contains interface{}, msgAndArgs ...interface{}) {
	containsRefVal := reflect.ValueOf(contains)
	if containsRefVal.Kind() == reflect.Ptr {
		contains = containsRefVal.Elem().Interface()
	}
	require.Contains(tb, slice, contains, msgAndArgs...)
}

func newT(T interface{}) interface{} {
	return reflect.New(reflect.TypeOf(T)).Interface()
}

func newTFunc(T interface{}) func() interface{} {
	return func() interface{} { return newT(T) }
}

func requireNotContainsList(tb testing.TB, list interface{}, listOfNotContainedElements interface{}, msgAndArgs ...interface{}) {
	v := reflect.ValueOf(listOfNotContainedElements)

	for i := 0; i < v.Len(); i++ {
		require.NotContains(tb, list, v.Index(i).Interface(), msgAndArgs...)
	}
}

func requireContainsList(tb testing.TB, list interface{}, listOfContainedElements interface{}, msgAndArgs ...interface{}) {
	v := reflect.ValueOf(listOfContainedElements)

	for i := 0; i < v.Len(); i++ {
		require.Contains(tb, list, v.Index(i).Interface(), msgAndArgs...)
	}
}

func toT(ent interface{}) frameless.T {
	return reflects.BaseValueOf(ent).Interface()
}

func base(e interface{}) interface{} {
	return reflects.BaseValueOf(e).Interface()
}

func toBaseValues(in []interface{}) []interface{} {
	var baseEntities []interface{}
	for _, e := range in {
		baseEntities = append(baseEntities, base(e))
	}
	return baseEntities
}

func newEventSubscriber(tb testing.TB, name string, filter func(interface{}) bool) *eventSubscriber {
	return &eventSubscriber{TB: tb, Name: name, Filter: filter}
}

type eventSubscriber struct {
	TB         testing.TB
	Name       string
	ReturnErr  error
	ContextErr error
	Filter     func(event interface{}) bool

	events []interface{}
	errors []error
	mutex  sync.Mutex
}

func (s *eventSubscriber) filter(event interface{}) bool {
	if s.Filter == nil {
		return true
	}
	return s.Filter(event)
}

func (s *eventSubscriber) Handle(ctx context.Context, event interface{}) error {
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

func (s *eventSubscriber) Error(ctx context.Context, err error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.verifyContext(ctx)
	s.errors = append(s.errors, err)
	return s.ReturnErr
}

func (s *eventSubscriber) EventsLen() int {
	return len(s.Events())
}

func (s *eventSubscriber) Events() []interface{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.events
}

func (s *eventSubscriber) verifyContext(ctx context.Context) {
	if s.ContextErr == nil {
		return
	}
	require.NotNil(s.TB, ctx)
	require.Equal(s.TB, s.ContextErr, ctx.Err())
}

const (
	contextKey      = `Context`
	subscriberKey   = `subscriberGet`
	subscriptionKey = `subscription`
)

var ctx = testcase.Var{
	Name: contextKey,
	Init: func(t *testcase.T) interface{} {
		return context.Background()
	},
}

func ctxGet(t *testcase.T) context.Context {
	return ctx.Get(t).(context.Context)
}

var (
	subscribedEvent = testcase.Var{
		Name: `subscribed event`,
		Init: func(t *testcase.T) interface{} {
			return `unknown`
		},
	}
	subscriberFilter = testcase.Var{
		Name: `subscriber event filter`,
		Init: func(t *testcase.T) interface{} {
			return func(interface{}) bool { return true }
		},
	}
	subscriber = testcase.Var{
		Name: subscriberKey,
		Init: func(t *testcase.T) interface{} {
			s := newEventSubscriber(
				t,
				subscribedEvent.Get(t).(string),
				subscriberFilter.Get(t).(func(interface{}) bool),
			)
			return s
		},
	}
	subscription = testcase.Var{
		Name: subscriptionKey,
	}
)

func subscriberLet(s *testcase.Spec, name string, filter func(event interface{}) bool) {
	subscriber.Let(s, func(t *testcase.T) interface{} {
		return newEventSubscriber(t, name, filter)
	})
}

func subscriberGet(t *testcase.T) *eventSubscriber {
	return subscriber.Get(t).(*eventSubscriber)
}

func getSubscriber(t *testcase.T, key string) *eventSubscriber {
	return testcase.Var{Name: key, Init: subscriber.Init}.Get(t).(*eventSubscriber)
}

func subscriptionGet(t *testcase.T) frameless.Subscriber {
	return subscription.Get(t).(frameless.Subscriber)
}

func genEntities(ff FixtureFactory, T T) []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, CreatePTR(ff, T))
	}
	return es
}

func spec(tb testing.TB, c Interface, blk func(s *testcase.Spec)) {
	s := testcase.NewSpec(tb)
	defer s.Finish()
	var name = reflect.TypeOf(c).Name()
	if stringer, ok := c.(fmt.Stringer); ok {
		name = stringer.String()
	}
	s.Context(name, blk, testcase.Group(name))
}

func CreatePTR(ff FixtureFactory, T T) interface{} {
	ptr := reflect.New(reflect.TypeOf(T))
	ptr.Elem().Set(reflect.ValueOf(ff.Create(T)))
	return ptr.Interface()
}

var factory = testcase.Var{Name: "FixtureFactory"}

func factoryLet(s *testcase.Spec, fff func(testing.TB) FixtureFactory) {
	factory.Let(s, func(t *testcase.T) interface{} {
		return fff(t)
	})
}

func factoryGet(t *testcase.T) FixtureFactory {
	return factory.Get(t).(FixtureFactory)
}
