package usecases_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"

	randomdata "github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless/example"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/storages"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/presenters"
	"github.com/adamluzsi/frameless/requests"
)

func TestUseCasesListNotes_NoNotesInTheStore_EmptySetReturned(t *testing.T) {
	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)
	p := presenters.NewMock()
	r := requests.New(context.Background(), iterators.NewEmpty())

	require.Nil(t, usecases.ListNotes(p, r))
	require.Equal(t, []*example.Note{}, p.Message())
}

func TestUseCasesListNotes_NotesStoredInTheStorageAlready_AllNoteReturned(t *testing.T) {
	notes := CreateNotes()

	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)
	AddNotest(t, storage, notes)

	p := presenters.NewMock()
	r := requests.New(context.Background(), iterators.NewEmpty())

	require.Nil(t, usecases.ListNotes(p, r))
	p.MessageMatch(t, notes)

}

func TestUseCasesListNotes_StorageFails_ErrReturned(t *testing.T) {
	notes := CreateNotes()

	t.Parallel()

	storage := storages.NewMemory()
	usecases := ExampleUseCases(storage)
	AddNotest(t, storage, notes)

	p := presenters.NewMock()
	r := requests.New(context.Background(), iterators.NewEmpty())

	require.Nil(t, usecases.ListNotes(p, r))
	p.MessageMatch(t, notes)
}

func CreateNotes() []*example.Note {
	notes := []*example.Note{}
	for i := 0; i < 10; i++ {
		note := &example.Note{
			Title:   randomdata.SillyName(),
			Content: randomdata.SillyName(),
		}
		notes = append(notes, note)
	}
	return notes
}

func AddNotest(t testing.TB, toStorage frameless.Storage, notes []*example.Note) {
	for _, note := range notes {
		require.Nil(t, toStorage.Create(note))
	}
}
