package encoders_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/encoders"
)

var _ frameless.Encoder = &encoders.Mock{}

func TestMock_ErrorSetToReturnOnRenderCall_ErrorReturned(t *testing.T) {
	t.Parallel()

	mock := encoders.NewMock()

	err := errors.New("Boom!")
	mock.ReturnError = err

	require.Equal(t, err, mock.Encode(nil))
}

func TestMock_MessageGivenToPresenter_LastMessageAvailableByMessageMethod(t *testing.T) {
	t.Parallel()

	mock := encoders.NewMock()
	require.Nil(t, mock.Encode("OK"))
	require.Equal(t, "OK", mock.Entity())
}

func TestMock_ValueGiven_MatchCheckEquality(t *testing.T) {
	t.Parallel()

	mock := encoders.NewMock()
	require.Nil(t, mock.Encode("OK"))

	t.Run("when asserted value is equal", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.MessageMatch(tb, "OK")
		}()

		wg.Wait()

		require.False(t, tb.Failed())
	})

	t.Run("when asserted value is different", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.MessageMatch(tb, "KO")
		}()

		wg.Wait()

		require.True(t, tb.Failed())
	})

}

func TestMock_SliceMessageExpected_MatchMakeGivenTestToFailOrNotDependingByEquality(t *testing.T) {
	t.Parallel()

	msg := []int{1, 2, 3, 4}
	mock := encoders.NewMock()
	require.Nil(t, mock.Encode(msg))

	t.Run("when asserted value is equal", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.MessageMatch(tb, []int{4, 2, 1, 3})
		}()

		wg.Wait()

		require.False(t, tb.Failed())
	})

	t.Run("when asserted value is different by length", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.MessageMatch(tb, []int{4, 2, 1, 3, 42})
		}()

		wg.Wait()

		require.True(t, tb.Failed())
	})

	t.Run("when asserted value is different by content", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.MessageMatch(tb, []int{4, 2, 1, 42})
		}()

		wg.Wait()

		require.True(t, tb.Failed())
	})

}

func TestMock_StreamLikeUsageExpected_MatchMakeGivenTestToFailOrNotDependingByEquality(t *testing.T) {
	t.Parallel()

	msgs := []int{1, 2, 3, 4}
	mock := encoders.NewMock()
	for _, msg := range msgs {
		require.Nil(t, mock.Encode(msg))
	}

	t.Run("when asserted value is equal", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.StreamContains(tb, []int{4, 2, 1, 3})
		}()

		wg.Wait()

		require.False(t, tb.Failed())
	})

	t.Run("when asserted value is different by length", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.StreamContains(tb, []int{4, 2, 1, 3, 42})
		}()

		wg.Wait()

		require.True(t, tb.Failed())
	})

	t.Run("when asserted value is different by content", func(t *testing.T) {
		tb := &testing.T{}

		wg := &sync.WaitGroup{}
		wg.Add(1)

		go func() {
			defer wg.Done()
			mock.StreamContains(tb, []int{4, 2, 1, 42})
		}()

		wg.Wait()

		require.True(t, tb.Failed())
	})

}
