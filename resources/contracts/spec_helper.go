package contracts

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/testcase/fixtures"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/reflects"
)

type T = resources.T

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

const msgNotMeasurable = `not measurable Spec`

const ErrIDRequired frameless.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

type crud interface {
	resources.Creator
	resources.Finder
	resources.Updater
	resources.Deleter
}

// CRD is the minimum requirements to write easily behavioral specification for a resource.
type CRD interface {
	resources.Creator
	resources.Finder
	resources.Deleter
}

func createEntities(f FixtureFactory, T interface{}) []interface{} {
	var es []interface{}
	for i := 0; i < benchmarkEntityVolumeCount; i++ {
		es = append(es, f.Create(T))
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

func cleanup(tb testing.TB, t resources.Deleter, f FixtureFactory, T interface{}) {
	require.Nil(tb, t.DeleteAll(f.Context(), T))
}

func contains(tb testing.TB, slice interface{}, contains interface{}, msgAndArgs ...interface{}) {
	containsRefVal := reflect.ValueOf(contains)
	if containsRefVal.Kind() == reflect.Ptr {
		contains = containsRefVal.Elem().Interface()
	}
	require.Contains(tb, slice, contains, msgAndArgs...)
}

func newEntity(T interface{}) interface{} {
	return reflect.New(reflect.TypeOf(T)).Interface()
}

func newEntityFunc(T interface{}) func() interface{} {
	return func() interface{} { return newEntity(T) }
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

func toT(ent interface{}) resources.T {
	return reflects.BaseValueOf(ent).Interface()
}

func toBaseValue(e interface{}) interface{} {
	return reflects.BaseValueOf(e).Interface()
}

func toBaseValues(in []interface{}) []interface{} {
	var baseEntities []interface{}
	for _, e := range in {
		baseEntities = append(baseEntities, toBaseValue(e))
	}
	return baseEntities
}

func newEventSubscriber(tb testing.TB) *eventSubscriber {
	return &eventSubscriber{TB: tb}
}

type eventSubscriber struct {
	TB        testing.TB
	Name      string
	ReturnErr error

	events []interface{}
	errors []error
	mutex  sync.Mutex
}

func (s *eventSubscriber) Handle(ctx context.Context, event interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.TB.Logf(`%s %#v`, s.Name, event)
	s.verifyContext(ctx)
	s.events = append(s.events, event)
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
	require.NotNil(s.TB, ctx)
	select {
	case <-ctx.Done():
		s.TB.Fatal(`it was not expected to have a ctx finished`)
	default:
	}
	require.Nil(s.TB, ctx.Err())
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

func ctxLetWithFixtureFactory(s *testcase.Spec, ff FixtureFactory) testcase.Var {
	return ctx.Let(s, func(t *testcase.T) interface{} {
		return ff.Context()
	})
}

var (
	subscriber = testcase.Var{
		Name: subscriberKey,
		Init: func(t *testcase.T) interface{} {
			return newEventSubscriber(t)
		},
	}
	subscription = testcase.Var{
		Name: subscriptionKey,
	}
)

func subscriberGet(t *testcase.T) *eventSubscriber {
	return subscriber.Get(t).(*eventSubscriber)
}

func getSubscriber(t *testcase.T, key string) *eventSubscriber {
	return testcase.Var{Name: key, Init: subscriber.Init}.Get(t).(*eventSubscriber)
}

func subscriptionGet(t *testcase.T) resources.Subscriber {
	return subscription.Get(t).(resources.Subscriber)
}

func genEntities(ff FixtureFactory, T T) []interface{} {
	var es []interface{}
	count := fixtures.Random.IntBetween(3, 7)
	for i := 0; i < count; i++ {
		es = append(es, ff.Create(T))
	}
	return es
}
