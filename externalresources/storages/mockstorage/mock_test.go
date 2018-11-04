package mockstorage_test

import (
	"errors"
	"testing"

	"github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/externalresources/storages/mockstorage"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

var _ frameless.Resource = mockstorage.NewMock()

func TestMock(t *testing.T) {

	entity := "I'm an entity, just kidding"

	expected := randomdata.SillyName()
	var actually string

	ExecStub := func(frameless.Query) frameless.Iterator {
		return iterators.NewSingleElement(expected)
	}

	t.Run("exec stub", func(t *testing.T) {
		t.Parallel()

		mock := mockstorage.NewMock()
		mock.ExecStub = ExecStub

		require.Nil(t, mock.Store(entity))
		require.Equal(t, []frameless.Entity{entity}, mock.Created)

		fakeIterator := mock.Exec(nil)
		require.NotNil(t, fakeIterator)
		require.Nil(t, fakeIterator.Err())
		require.Nil(t, iterators.DecodeNext(mock.Exec(nil), &actually))
		require.Equal(t, expected, actually)
	})

	t.Run("err stub", func(t *testing.T) {
		t.Parallel()

		mock := mockstorage.NewMock()
		mock.ExecStub = ExecStub

		mock.ReturnError = errors.New("BOOM!")
		require.Equal(t, mock.ReturnError, mock.Store(entity))
		require.Equal(t, mock.ReturnError, mock.Exec(nil).Err())

	})

	t.Run("Close", func(t *testing.T) {
		t.Parallel()

		mock := mockstorage.NewMock()
		require.True(t, mock.IsOpen)
		require.Nil(t, mock.Close())
		require.False(t, mock.IsOpen)

	})
}
