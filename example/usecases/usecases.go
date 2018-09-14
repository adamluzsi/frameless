package usecases

import (
	"errors"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/example"
	"github.com/adamluzsi/frameless/iterators/iterateover"
	"github.com/adamluzsi/frameless/queries"
)

func NewUseCases(storage frameless.Storage) *UseCases {
	return &UseCases{storage: storage}
}

type UseCases struct {
	storage frameless.Storage
}

func (uc *UseCases) ListNotes(r frameless.Request, p frameless.Presenter) error {
	notes := []*example.Note{}

	i := uc.storage.Find(queries.AllFor{Type: example.Note{}})

	if err := iterateover.AndCollectAll(i, &notes); err != nil {
		return err
	}

	return p.Render(notes)
}

func (uc *UseCases) AddNote(r frameless.Request, p frameless.Presenter) error {
	title, ok := r.Context().Value("Title").(string)
	if !ok || title == "" {
		return errors.New("missing Title")
	}

	content, ok := r.Context().Value("Content").(string)
	if !ok || content == "" {
		return errors.New("missing Content")
	}

	newNote := &example.Note{
		Title:   title,
		Content: content,
	}

	if err := uc.storage.Create(newNote); err != nil {
		return err
	}

	return p.Render(newNote)
}
