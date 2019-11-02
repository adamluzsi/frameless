package iterators_test

import (
	"errors"
	"testing"

	"github.com/adamluzsi/frameless"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators"
)

var _ frameless.Iterator = iterators.NewMock(iterators.NewEmpty())

func TestMock_Err(t *testing.T) {
	t.Parallel()

	originalError := errors.New("Boom! original")
	expectedError := errors.New("Boom! stub")

	m := iterators.NewMock(iterators.NewError(originalError))

	// default is the wrapped iterator
	require.Error(t, originalError, m.Err())

	m.StubErr = func() error { return expectedError }
	require.Error(t, expectedError, m.Err())

	m.ResetErr()
	require.Error(t, originalError, m.Err())

}

func TestMock_Close(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom! stub")

	m := iterators.NewMock(iterators.NewEmpty())

	// default is the wrapped iterator
	require.Nil(t, m.Close())

	m.StubClose = func() error { return expectedError }
	require.Error(t, expectedError, m.Close())

	m.ResetClose()
	require.Nil(t, m.Close())
}

func TestMock_Next(t *testing.T) {
	t.Parallel()

	m := iterators.NewMock(iterators.NewEmpty())

	require.False(t, m.Next())

	m.StubNext = func() bool { return true }
	require.True(t, m.Next())

	m.ResetNext()
	require.False(t, m.Next())
}

func TestMock_Decode(t *testing.T) {
	t.Parallel()

	var value int
	expectedError := errors.New("Boom! stub")

	m := iterators.NewMock(iterators.NewSlice([]int{42, 43, 44}))

	require.True(t, m.Next())
	require.Nil(t, m.Decode(&value))
	require.Equal(t, 42, value)

	require.True(t, m.Next())
	require.Nil(t, m.Decode(&value))
	require.Equal(t, 43, value)

	m.StubDecode = func(i interface{}) error {
		*i.(*int) = 4242
		return expectedError
	}

	require.Error(t, expectedError, m.Decode(&value))
	require.Equal(t, 4242, value)

	m.ResetDecode()
	require.True(t, m.Next())
	require.Nil(t, m.Decode(&value))
	require.Equal(t, 44, value)

}
