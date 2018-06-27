package multichannel

import (
	"errors"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators/iterateover"
	"github.com/adamluzsi/frameless/queryusecases"
)

type UseCases struct {
	Storage frameless.Storage
}

func (uc *UseCases) ListNotes(p frameless.Presenter, r frameless.Request) error {
	notes := []*Note{}

	i := uc.Storage.Find(queryusecases.AllFor{Type: Note{}})

	if err := iterateover.AndCollectAll(i, &notes); err != nil {
		return err
	}

	return p.Render(notes)
}

func (uc *UseCases) AddNote(p frameless.Presenter, r frameless.Request) error {
	title, ok := r.Context().Value("Title").(string)
	if !ok {
		return errors.New("missing Title")
	}

	content, ok := r.Context().Value("Content").(string)
	if !ok {
		return errors.New("missing Content")
	}

	newNote := &Note{
		Title:   title,
		Content: content,
	}

	if err := uc.Storage.Create(newNote); err != nil {
		return err
	}

	return p.Render(newNote)
}
