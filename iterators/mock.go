package iterators

import (
	"github.com/adamluzsi/frameless"
)

func NewMock(i frameless.Iterator) *Mock {
	return &Mock{
		iterator: i,

		DecodeStub: i.Decode,
		CloseStub:  i.Close,
		NextStub:   i.Next,
		ErrStub:    i.Err,
	}
}

type Mock struct {
	iterator frameless.Iterator

	DecodeStub func(interface{}) error
	CloseStub  func() error
	NextStub   func() bool
	ErrStub    func() error
}

// wrapper

func (m *Mock) Close() error {
	return m.CloseStub()
}

func (m *Mock) Next() bool {
	return m.NextStub()
}

func (m *Mock) Err() error {
	return m.ErrStub()
}

func (m *Mock) Decode(i interface{}) error {
	return m.DecodeStub(i)
}

// Reseting stubs

func (m *Mock) ResetClose() {
	m.CloseStub = m.iterator.Close
}

func (m *Mock) ResetNext() {
	m.NextStub = m.iterator.Next
}

func (m *Mock) ResetErr() {
	m.ErrStub = m.iterator.Err
}

func (m *Mock) ResetDecode() {
	m.DecodeStub = m.iterator.Decode
}
