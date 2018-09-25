package storages

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
)

func NewMock() *Mock {
	return &Mock{
		IsOpen:   true,
		Created:  []frameless.Entity{},
		FindStub: func(quc frameless.Query) frameless.Iterator { return iterators.NewEmpty() },
		ExecStub: func(quc frameless.Query) error { return nil },
	}
}

type Mock struct {
	IsOpen bool

	ReturnError error

	Created  []frameless.Entity
	FindStub func(frameless.Query) frameless.Iterator
	ExecStub func(frameless.Query) error
}

func (mock *Mock) Close() error {
	mock.IsOpen = false
	return nil
}

func (mock *Mock) Create(e frameless.Entity) error {
	mock.Created = append(mock.Created, e)

	return mock.ReturnError
}

func (mock *Mock) Find(quc frameless.Query) frameless.Iterator {
	if mock.ReturnError != nil {
		return iterators.NewError(mock.ReturnError)
	}

	return mock.FindStub(quc)
}

func (mock *Mock) Exec(quc frameless.Query) error {
	if mock.ReturnError != nil {
		return mock.ReturnError
	}

	return mock.ExecStub(quc)
}
