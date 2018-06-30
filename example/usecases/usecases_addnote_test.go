package usecases_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/iterators/iterateover"

	"github.com/adamluzsi/frameless/queryusecases"
	"github.com/adamluzsi/frameless/storages"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/presenters"
	"github.com/adamluzsi/frameless/requests"

	"github.com/adamluzsi/frameless/example"
)

var (
	AddNoteTitle   = randomdata.SillyName()
	AddNoteContent = randomdata.SillyName()
)

func TestUseCasesAddNote_NoNotesInTheStore_NoteCreatedAndNoteReturned(t *testing.T) {
	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)

	p := presenters.NewMock()

	sample := &example.Note{
		Title:   AddNoteTitle,
		Content: AddNoteContent,
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "Title", sample.Title)
	ctx = context.WithValue(ctx, "Content", sample.Content)
	r := requests.NewMock(ctx, iterators.NewEmpty())

	require.Nil(t, usecases.AddNote(p, r))

	i := storage.Find(queryusecases.AllFor{Type: example.Note{}})

	foundNotes := []*example.Note{}
	require.Nil(t, iterateover.AndCollectAll(i, &foundNotes))

	require.Equal(t, 1, len(foundNotes))
	savedNote := foundNotes[0]

	require.True(t, len(p.ReceivedMessages) > 0)
	require.Equal(t, savedNote, p.Message())

}

func TestUseCasesAddNote_ErrHappenInStorage_ErrReturned(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom!")
	storage := storages.NewMock()
	storage.ReturnError = expectedError

	usecases := ExampleUseCases(storage)

	p := presenters.NewMock()

	ctx := context.Background()
	ctx = context.WithValue(ctx, "Title", AddNoteTitle)
	ctx = context.WithValue(ctx, "Content", AddNoteContent)
	r := requests.NewMock(ctx, iterators.NewEmpty())

	require.Error(t, expectedError, usecases.AddNote(p, r))
}
