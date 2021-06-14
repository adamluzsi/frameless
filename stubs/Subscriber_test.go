package stubs_test

import (
	"context"
	"errors"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/stubs"
	"github.com/stretchr/testify/require"
	"testing"
)

var _ frameless.Subscriber = stubs.Subscriber{}

func TestSubscriber(t *testing.T) {
	type Entity struct{ V int }
	ctx := context.Background()
	t.Run(`.Error`, func(t *testing.T) {
		t.Run(`.ErrorFunc is absent`, func(t *testing.T) {
			stub := stubs.Subscriber{}
			require.Nil(t, stub.Error(ctx, errors.New(fixtures.Random.String())))
		})
		t.Run(`.ErrorFunc is provided`, func(t *testing.T) {
			expectedInError := errors.New(fixtures.Random.String())
			expectedOutError := errors.New(fixtures.Random.String())
			stub := stubs.Subscriber{ErrorFunc: func(ctx context.Context, err error) error {
				require.Equal(t, expectedInError, err)
				return expectedOutError
			}}
			require.Equal(t, expectedOutError, stub.Error(ctx, expectedInError))
		})
	})
	t.Run(`.Handle`, func(t *testing.T) {
		t.Run(`.HandleFunc is absent`, func(t *testing.T) {
			stub := stubs.Subscriber{}
			require.Nil(t, stub.Handle(ctx, Entity{}))
		})
		t.Run(`.HandleFunc is provided`, func(t *testing.T) {
			expectedEntity := Entity{V: fixtures.Random.Int()}
			expectedOutError := errors.New(fixtures.Random.String())
			stub := stubs.Subscriber{HandleFunc: func(ctx context.Context, ent interface{}) error {
				require.Equal(t, expectedEntity, ent)
				return expectedOutError
			}}
			require.Equal(t, expectedOutError, stub.Handle(ctx, expectedEntity))
		})
	})
}
