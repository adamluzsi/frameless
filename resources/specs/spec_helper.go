package specs

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless/consterror"
	"github.com/adamluzsi/frameless/resources"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/reflects"
)

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

const msgNotMeasurable = `not measurable spec`

const ErrIDRequired consterror.Error = `
Can't find the ID in the current structure
if there is no ID in the subject structure
custom test needed that explicitly defines how ID is stored and retried from an entity
`

type minimumRequirements interface {
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

func saveEntities(tb testing.TB, s resources.Creator, f FixtureFactory, es ...interface{}) []interface{} {
	var ids []interface{}
	for _, e := range es {
		require.Nil(tb, s.Create(f.Context(), e))
		id, _ := resources.LookupID(e)
		ids = append(ids, id)
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
	return &eventSubscriber{
		tb:     tb,
		events: make([]interface{}, 0),
	}
}

type eventSubscriber struct {
	tb        testing.TB
	events    []interface{}
	errors    []error
	returnErr error

	mutex sync.Mutex
}

func (s *eventSubscriber) Handle(ctx context.Context, event interface{}) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.verifyContext(ctx)
	s.events = append(s.events, event)
	return s.returnErr
}

func (s *eventSubscriber) Error(ctx context.Context, err error) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.verifyContext(ctx)
	s.errors = append(s.errors, err)
	return s.returnErr
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
	require.NotNil(s.tb, ctx)
	select {
	case <-ctx.Done():
		s.tb.Fatal(`it was not expected to have a ctx finished`)
	default:
	}
	require.Nil(s.tb, ctx.Err())
}

const (
	contextKey      = `context`
	subscriberKey   = `subscriber`
	subscriptionKey = `subscription`
)

func getContext(t *testcase.T) context.Context {
	return t.I(contextKey).(context.Context)
}

func getSubscriber(t *testcase.T, key string) *eventSubscriber {
	return t.I(key).(*eventSubscriber)
}

func subscriber(t *testcase.T) *eventSubscriber {
	return getSubscriber(t, subscriberKey)
}
