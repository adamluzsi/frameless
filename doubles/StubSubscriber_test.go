package doubles_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/doubles"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/stretchr/testify/require"
)

var (
	_ frameless.CreatorSubscriber = doubles.StubSubscriber{}
	_ frameless.UpdaterSubscriber = doubles.StubSubscriber{}
	_ frameless.DeleterSubscriber = doubles.StubSubscriber{}
)

func TestSubscriber(t *testing.T) {
	type Entity struct{ V int }
	ctx := context.Background()
	t.Run(`.Error`, func(t *testing.T) {
		t.Run(`.ErrorFunc is absent`, func(t *testing.T) {
			stub := doubles.StubSubscriber{}
			require.Nil(t, stub.Error(ctx, errors.New(fixtures.Random.String())))
		})
		t.Run(`.ErrorFunc is provided`, func(t *testing.T) {
			expectedInError := errors.New(fixtures.Random.String())
			expectedOutError := errors.New(fixtures.Random.String())
			stub := doubles.StubSubscriber{ErrorFunc: func(ctx context.Context, err error) error {
				require.Equal(t, expectedInError, err)
				return expectedOutError
			}}
			require.Equal(t, expectedOutError, stub.Error(ctx, expectedInError))
		})
	})
	t.Run(`.Handle`, func(t *testing.T) {
		t.Run(`.HandleFunc is absent`, func(t *testing.T) {
			stub := doubles.StubSubscriber{}
			require.Nil(t, stub.Handle(ctx, Entity{}))
		})
		t.Run(`.HandleFunc is provided`, func(t *testing.T) {
			expectedEntity := Entity{V: fixtures.Random.Int()}
			expectedOutError := errors.New(fixtures.Random.String())
			stub := doubles.StubSubscriber{HandleFunc: func(ctx context.Context, ent interface{}) error {
				require.Equal(t, expectedEntity, ent)
				return expectedOutError
			}}
			require.Equal(t, expectedOutError, stub.Handle(ctx, expectedEntity))
		})
	})
}
