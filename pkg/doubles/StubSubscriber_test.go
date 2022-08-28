package doubles_test

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless/pkg/doubles"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"testing"

	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

var (
	_ pubsub.CreatorSubscriber[any] = doubles.StubSubscriber[any, any]{}
	_ pubsub.UpdaterSubscriber[any] = doubles.StubSubscriber[any, any]{}
	_ pubsub.DeleterSubscriber[any] = doubles.StubSubscriber[any, any]{}
)

func TestSubscriber(t *testing.T) {
	type Entity struct{ V int }
	ctx := context.Background()
	rnd := random.New(random.CryptoSeed{})
	t.Run(`.Error`, func(t *testing.T) {
		t.Run(`.ErrorFunc is absent`, func(t *testing.T) {
			stub := doubles.StubSubscriber[Entity, int]{}
			assert.Must(t).Nil(stub.HandleError(ctx, errors.New(rnd.String())))
		})
		t.Run(`.ErrorFunc is provided`, func(t *testing.T) {
			expectedInError := errors.New(rnd.String())
			expectedOutError := errors.New(rnd.String())
			stub := doubles.StubSubscriber[Entity, int]{ErrorFunc: func(ctx context.Context, err error) error {
				assert.Must(t).Equal(expectedInError, err)
				return expectedOutError
			}}
			assert.Must(t).Equal(expectedOutError, stub.HandleError(ctx, expectedInError))
		})
	})
	t.Run(`.Handle`, func(t *testing.T) {
		t.Run(`.HandleFunc is absent`, func(t *testing.T) {
			stub := doubles.StubSubscriber[Entity, int]{}
			assert.Must(t).Nil(stub.Handle(ctx, Entity{}))
		})
		t.Run(`.HandleFunc is provided`, func(t *testing.T) {
			expectedEntity := Entity{V: rnd.Int()}
			expectedOutError := errors.New(rnd.String())
			stub := doubles.StubSubscriber[Entity, int]{HandleFunc: func(ctx context.Context, ent interface{}) error {
				assert.Must(t).Equal(expectedEntity, ent)
				return expectedOutError
			}}
			assert.Must(t).Equal(expectedOutError, stub.Handle(ctx, expectedEntity))
		})
	})
}
