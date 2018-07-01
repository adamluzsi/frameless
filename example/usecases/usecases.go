package usecases

import (
	"errors"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/example"
	"github.com/adamluzsi/frameless/iterators/iterateover"
	"github.com/adamluzsi/frameless/queryusecases"
)

func NewUseCases(storage frameless.Storage) *UseCases {
	return &UseCases{storage: storage}
}

type UseCases struct {
	storage frameless.Storage
}

func (uc *UseCases) ListNotes(p frameless.Presenter, r frameless.Request) error {
	notes := []*example.Note{}

	i := uc.storage.Find(queryusecases.AllFor{Type: example.Note{}})

	if err := iterateover.AndCollectAll(i, &notes); err != nil {
		return err
	}

	return p.Render(notes)
}

func (uc *UseCases) AddNote(p frameless.Presenter, r frameless.Request) error {
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
