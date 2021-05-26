package iterators_test

import (
	"errors"
	"io"
	"sync"

	"github.com/adamluzsi/frameless"
)

type Entity struct {
	Text string
}

type ReadCloser struct {
	IsClosed bool
	io       io.Reader
}

func NewReadCloser(r io.Reader) *ReadCloser {
	return &ReadCloser{io: r, IsClosed: false}
}

func (this *ReadCloser) Read(p []byte) (n int, err error) {
	return this.io.Read(p)
}

func (this *ReadCloser) Close() error {
	if this.IsClosed {
		return errors.New("already closed")
	}

	this.IsClosed = true
	return nil
}

type BrokenReader struct{}

func (b *BrokenReader) Read(p []byte) (n int, err error) { return 0, io.ErrUnexpectedEOF }

type x struct{ data string }

type StubIterator struct {
	frameless.Decoder
	once   sync.Once
	closed bool
}

func (s *StubIterator) Close() error {
	s.closed = true
	return nil
}

func (s *StubIterator) Next() bool {
	var has bool
	s.once.Do(func() { has = true })
	return has
}

func (s *StubIterator) Err() error {
	return nil
}
