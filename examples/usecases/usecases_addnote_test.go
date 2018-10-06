package usecases_test

import (
	"context"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/iterators/iterateover"
	"github.com/adamluzsi/frameless/queries/find"
	"github.com/adamluzsi/frameless/storages/memorystorage"
	"github.com/adamluzsi/frameless/storages/mockstorage"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/encoders"
	"github.com/adamluzsi/frameless/requests"

	"github.com/adamluzsi/frameless/examples"
)

var (
	AddNoteTitle   = randomdata.SillyName()
	AddNoteContent = randomdata.SillyName()
)

func TestUseCasesAddNote_NoNotesInTheStore_NoteCreatedAndNoteReturned(t *testing.T) {
	t.Parallel()

	storage := memorystorage.NewMemory()
	usecases := ExampleUseCases(storage)

	p := encoders.NewMock()

	sample := &examples.Note{
		Title:   AddNoteTitle,
		Content: AddNoteContent,
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "Title", sample.Title)
	ctx = context.WithValue(ctx, "Content", sample.Content)
	r := requests.New(ctx, iterators.NewEmpty())

	require.Nil(t, usecases.AddNote(r, p))

	i := storage.Exec(find.All{Type: examples.Note{}})

	foundNotes := []*examples.Note{}
	require.Nil(t, iterateover.AndCollectAll(i, &foundNotes))

	require.Equal(t, 1, len(foundNotes))
	savedNote := foundNotes[0]

	require.True(t, len(p.Received) > 0)
	require.Equal(t, savedNote, p.Entity())

}

func TestUseCasesAddNote_ErrHappenInStorage_ErrReturned(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom!")
	storage := mockstorage.NewMock()
	storage.ReturnError = expectedError

	usecases := ExampleUseCases(storage)

	p := encoders.NewMock()

	ctx := context.Background()
	ctx = context.WithValue(ctx, "Title", AddNoteTitle)
	ctx = context.WithValue(ctx, "Content", AddNoteContent)
	r := requests.New(ctx, iterators.NewEmpty())

	require.Error(t, expectedError, usecases.AddNote(r, p))
}

func TestUseCasesAddNote_MissingTitle_ErrReturned(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	usecases := ExampleUseCases(nil)

	require.Equal(t, errors.New("missing Title"), usecases.AddNote(requests.New(ctx, iterators.NewEmpty()), nil))

	ctx = context.WithValue(ctx, "Title", AddNoteTitle)
	require.Equal(t, errors.New("missing Content"), usecases.AddNote(requests.New(ctx, iterators.NewEmpty()), nil))

}
