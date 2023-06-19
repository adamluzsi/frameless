package spechelper

import (
	"context"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/testcase"
)

func MakeContext(tb testing.TB) context.Context {
	return context.Background()
}

// CRD is the minimum requirements to write easily behavioral specification for a resource.
type CRD[Entity, ID any] interface {
	crud.Creator[Entity]
	crud.ByIDFinder[Entity, ID]
	crud.ByIDDeleter[ID]
}

type CRUD[Entity, ID any] interface {
	CRD[Entity, ID]
	crud.Updater[Entity]
}

const ErrIDRequired errorkit.Error = `
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

type eventSubscriber[Entity, ID any] struct {
	TB         testing.TB
	ReturnErr  error
	ContextErr error
	Filter     func(event interface{}) bool

	events []interface{}
	errors []error
	mutex  sync.Mutex
}

func (s *eventSubscriber[Entity, ID]) Events() []interface{} {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.events
}

func filterEventSubscriberEvents[Entity, ID, T any](s *eventSubscriber[Entity, ID]) []T {
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

func (s *eventSubscriber[Entity, ID]) verifyContext(ctx context.Context) {
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
