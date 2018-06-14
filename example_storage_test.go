package frameless_test

import (
	"errors"

	"github.com/adamluzsi/frameless"
)

type MyBusinessEntity interface {
	frameless.Persistable

	Name() string
}

type MyControllers struct {
	storage frameless.Storage
}

type FindByName struct{ Name string }

func ExampleStorage_where(s frameless.Storage, p frameless.Presenter, r frameless.Request) error {

	name, ok := r.Context().Value("Name").(string)

	if !ok {
		return errors.New("Name is not given")
	}

	i := s.Where(FindByName{name})
	entities := []MyBusinessEntity{}

	for i.Next() {
		var e MyBusinessEntity
		i.Decode(&e)
		entities = append(entities, e)
	}

	if err := i.Err(); err != nil {
		return err
	}

	return p.Render(entities)

}
