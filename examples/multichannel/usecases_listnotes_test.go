package multichannel_test

import (
	"context"
	"testing"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/storages"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/presenters"
	"github.com/adamluzsi/frameless/requests"

	"github.com/adamluzsi/frameless/examples/multichannel"
)

func TestUseCasesListNotes_NoNotesInTheStore_EmptySetReturned(t *testing.T) {
	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)
	p := presenters.NewMock()
	r := requests.NewMock(context.Background(), iterators.NewEmpty())

	require.Nil(t, usecases.ListNotes(p, r))
	require.Equal(t, []*multichannel.Note{}, p.Message())
}

func TestUseCasesListNotes_NotesStoredInTheStorageAlready_AllNoteReturned(t *testing.T) {
	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)

	notes := []*multichannel.Note{}
	for i := 0; i < 10; i++ {
		note := &multichannel.Note{
			Title:   randomdata.SillyName(),
			Content: randomdata.SillyName(),
		}

		notes = append(notes, note)
		require.Nil(t, storage.Create(note))
	}

	p := presenters.NewMock()
	r := requests.NewMock(context.Background(), iterators.NewEmpty())

	require.Nil(t, usecases.ListNotes(p, r))
	p.MessageMatch(t, notes)

}

func TestUseCasesListNotes_StorageFails_ErrReturned(t *testing.T) {
	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)

	notes := []*multichannel.Note{}
	for i := 0; i < 10; i++ {
		note := &multichannel.Note{
			Title:   randomdata.SillyName(),
			Content: randomdata.SillyName(),
		}

		notes = append(notes, note)
		require.Nil(t, storage.Create(note))
	}

	p := presenters.NewMock()
	r := requests.NewMock(context.Background(), iterators.NewEmpty())

	require.Nil(t, usecases.ListNotes(p, r))
	p.MessageMatch(t, notes)

}
