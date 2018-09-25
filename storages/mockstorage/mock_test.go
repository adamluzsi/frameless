package storages_test

import (
	"errors"
	"testing"

	"github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/storages"
	"github.com/stretchr/testify/require"
)

var _ frameless.Storage = storages.NewMemory()

func TestMock(t *testing.T) {

	entity := "I'm an entity, just kidding"

	expected := randomdata.SillyName()
	var actually string

	FindStub := func(frameless.Query) frameless.Iterator {
		return iterators.NewSingleElement(expected)
	}

	ExecStub := func(frameless.Query) error {
		return errors.New("stub")
	}

	mock := storages.NewMock()
	mock.FindStub = FindStub
	mock.ExecStub = ExecStub

	// Happy stubbed case
	require.Nil(t, mock.Create(entity))
	require.Equal(t, []frameless.Entity{entity}, mock.Created)
	require.Equal(t, "stub", mock.Exec(nil).Error())
	require.Nil(t, iterators.DecodeNext(mock.Find(nil), &actually))
	require.Equal(t, expected, actually)

	// Err case
	mock.ReturnError = errors.New("BOOM!")
	require.Equal(t, mock.ReturnError, mock.Create(entity))
	require.Equal(t, mock.ReturnError, mock.Exec(nil))
	require.Equal(t, mock.ReturnError, mock.Find(nil).Err())

	require.True(t, mock.IsOpen)
	require.Nil(t, mock.Close())
	require.False(t, mock.IsOpen)
}
