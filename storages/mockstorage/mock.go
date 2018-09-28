package mockstorage

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
)

func NewMock() *Mock {
	return &Mock{
		IsOpen:   true,
		Created:  []frameless.Entity{},
		ExecStub: func(quc frameless.Query) frameless.Iterator { return iterators.NewEmpty() },
	}
}

type Mock struct {
	IsOpen bool

	ReturnError error

	Created  []frameless.Entity
	ExecStub func(frameless.Query) frameless.Iterator
}

func (mock *Mock) Close() error {
	mock.IsOpen = false
	return nil
}

func (mock *Mock) Store(e frameless.Entity) error {
	mock.Created = append(mock.Created, e)

	return mock.ReturnError
}

func (mock *Mock) Exec(quc frameless.Query) frameless.Iterator {
	if mock.ReturnError != nil {
		return iterators.NewError(mock.ReturnError)
	}

	return mock.ExecStub(quc)
}
