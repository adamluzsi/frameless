package multichannel_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/iterators/iterateover"

	"github.com/adamluzsi/frameless/queryusecases"
	"github.com/adamluzsi/frameless/storages"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/presenters"
	"github.com/adamluzsi/frameless/requests"

	"github.com/adamluzsi/frameless/examples/multichannel"
)

func TestUseCasesAddNote_NoNotesInTheStore_NoteCreatedAndNoteReturned(t *testing.T) {
	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)

	p := presenters.NewMock()

	sample := &multichannel.Note{
		Title:   randomdata.SillyName(),
		Content: randomdata.SillyName(),
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, "Title", sample.Title)
	ctx = context.WithValue(ctx, "Content", sample.Content)
	r := requests.NewMock(ctx, iterators.NewEmpty())

	require.Nil(t, usecases.AddNote(p, r))

	i := storage.Find(queryusecases.AllFor{Type: multichannel.Note{}})

	foundNotes := []*multichannel.Note{}
	require.Nil(t, iterateover.AndCollectAll(i, &foundNotes))

	require.Equal(t, 1, len(foundNotes))
	savedNote := foundNotes[0]

	fmt.Println(len(p.ReceivedMessages))
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
	ctx = context.WithValue(ctx, "Title", randomdata.SillyName())
	ctx = context.WithValue(ctx, "Content", randomdata.SillyName())
	r := requests.NewMock(ctx, iterators.NewEmpty())

	require.Error(t, expectedError, usecases.AddNote(p, r))

}
