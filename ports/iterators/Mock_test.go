package iterators_test

import (
	"errors"
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"testing"

	"github.com/adamluzsi/testcase/assert"
)

var _ iterators2.Iterator[any] = iterators2.Stub[any](iterators2.Empty[any]())

func TestMock_Err(t *testing.T) {
	t.Parallel()

	originalError := errors.New("Boom! original")
	expectedError := errors.New("Boom! stub")

	m := iterators2.Stub[any](iterators2.Error[any](originalError))

	// default is the wrapped iterator
	assert.Must(t).NotNil(originalError, m.Err())

	m.StubErr = func() error { return expectedError }
	assert.Must(t).NotNil(expectedError, m.Err())

	m.ResetErr()
	assert.Must(t).NotNil(originalError, m.Err())

}

func TestMock_Close(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom! stub")

	m := iterators2.Stub[any](iterators2.Empty[any]())

	// default is the wrapped iterator
	assert.Must(t).Nil(m.Close())

	m.StubClose = func() error { return expectedError }
	assert.Must(t).NotNil(expectedError, m.Close())

	m.ResetClose()
	assert.Must(t).Nil(m.Close())
}

func TestMock_Next(t *testing.T) {
	t.Parallel()

	m := iterators2.Stub[any](iterators2.Empty[any]())

	assert.Must(t).False(m.Next())

	m.StubNext = func() bool { return true }
	assert.Must(t).True(m.Next())

	m.ResetNext()
	assert.Must(t).False(m.Next())
}

func TestMock_Decode(t *testing.T) {
	t.Parallel()

	m := iterators2.Stub[int](iterators2.Slice[int]([]int{42, 43, 44}))

	assert.Must(t).True(m.Next())
	assert.Must(t).Equal(42, m.Value())

	assert.Must(t).True(m.Next())
	assert.Must(t).Equal(43, m.Value())

	m.StubValue = func() int {
		return 4242
	}
	assert.Must(t).Equal(4242, m.Value())

	m.ResetValue()
	assert.Must(t).True(m.Next())
	assert.Must(t).Equal(44, m.Value())
}
