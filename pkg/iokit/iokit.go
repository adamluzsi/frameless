package iokit

import (
	"bytes"
	"go.llib.dev/frameless/pkg/units"
	"go.llib.dev/frameless/pkg/zerokit"
	"io"
	"io/fs"
	"sync"

	"go.llib.dev/frameless/pkg/errorkit"
)

const ErrSeekNegativePosition errorkit.Error = "iokit: negative position"

func NewBuffer[T []byte | string](data T) *Buffer {
	return &Buffer{buffer: *bytes.NewBuffer([]byte(data)), position: 0}
}

// Buffer is an in-memory io.ReadWriteSeeker implementation
type Buffer struct {
	buffer   bytes.Buffer
	position int
	closed   bool
}

func (b *Buffer) Read(p []byte) (n int, err error) {
	if b.closed {
		return 0, fs.ErrClosed
	}
	r := bytes.NewReader(b.buffer.Bytes())
	if _, err := r.Seek(int64(b.position), io.SeekStart); err != nil {
		return 0, err
	}
	n, err = r.Read(p)
	b.position += n
	return n, err
}

// Write writes to the buffer of this Buffer instance
func (b *Buffer) Write(p []byte) (n int, err error) {
	if b.closed {
		return 0, fs.ErrClosed
	}
	// If the offset is past the end of the buffer, grow the buffer with null bytes.
	if extra := b.position - b.buffer.Len(); extra > 0 {
		if _, err := b.buffer.Write(make([]byte, extra)); err != nil {
			return n, err
		}
	}

	// If the offset isn't at the end of the buffer, write as much as we can.
	if b.position < b.buffer.Len() {
		n = copy(b.buffer.Bytes()[b.position:], p)
		p = p[n:]
	}

	// If there are remaining bytes, append them to the buffer.
	if len(p) > 0 {
		var bn int
		bn, err = b.buffer.Write(p)
		n += bn
	}

	b.position += n
	return n, err
}

// Seek seeks in the buffer of this Buffer instance
func (b *Buffer) Seek(offset int64, whence int) (int64, error) {
	if b.closed {
		return 0, fs.ErrClosed
	}
	nextPos, offs := 0, int(offset)
	switch whence {
	case io.SeekStart:
		nextPos = offs
	case io.SeekCurrent:
		nextPos = b.position + offs
	case io.SeekEnd:
		nextPos = b.buffer.Len() + offs
	}
	if nextPos < 0 {
		return 0, ErrSeekNegativePosition
	}
	b.position = nextPos
	return int64(nextPos), nil
}

func (b *Buffer) Close() error {
	if b.closed {
		return fs.ErrClosed
	}
	b.closed = true
	return nil
}

func (b *Buffer) String() string {
	return b.buffer.String()
}

func (b *Buffer) Bytes() []byte {
	return b.buffer.Bytes()
}

///////////////////////////////////////////////////////// sync /////////////////////////////////////////////////////////

type SyncWriter struct {
	// Writer is the protected io.Writer
	Writer io.Writer
	// Locker is an optional sync.Locker value if you need to share locking between different multiple SyncWriter.
	//
	// Default: sync.Mutex
	Locker sync.Locker
}

func (w *SyncWriter) Write(p []byte) (n int, err error) {
	locker := getLocker(&w.Locker)
	locker.Lock()
	defer locker.Unlock()
	return w.Writer.Write(p)
}

type SyncReader struct {
	// Reader is the protected io.Reader
	Reader io.Reader
	// Locker is an optional sync.Locker value if you need to share locking between different multiple SyncReader.
	//
	// Default: sync.Mutex
	Locker sync.Locker
}

func (r *SyncReader) Read(p []byte) (n int, err error) {
	locker := getLocker(&r.Locker)
	locker.Lock()
	defer locker.Unlock()
	return r.Reader.Read(p)
}

type SyncReadWriter struct {
	// ReadWriter is the protected io.ReadWriter
	ReadWriter io.ReadWriter
	// Locker is an optional sync.Locker value if you need to share locking between different multiple SyncReader.
	//
	// Default: sync.Mutex
	Locker sync.Locker
}

func (rw *SyncReadWriter) Write(p []byte) (n int, err error) {
	locker := getLocker(&rw.Locker)
	locker.Lock()
	defer locker.Unlock()
	return rw.ReadWriter.Write(p)
}

func (rw *SyncReadWriter) Read(p []byte) (n int, err error) {
	locker := getLocker(&rw.Locker)
	locker.Lock()
	defer locker.Unlock()
	return rw.ReadWriter.Read(p)
}

func getLocker(ptr *sync.Locker) sync.Locker {
	return zerokit.Init(ptr, func() sync.Locker {
		return &sync.Mutex{}
	})
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

const ErrReadLimitReached errorkit.Error = "request-entity-too-large"

func ReadAllWithLimit(body io.Reader, readLimit units.ByteSize) (_ []byte, returnErr error) {
	if body == nil { // TODO:TEST_ME
		return []byte{}, nil
	}
	if closer, ok := body.(io.ReadCloser); ok { // TODO:TEST_ME
		defer errorkit.Finish(&returnErr, closer.Close)
	}
	data, err := io.ReadAll(io.LimitReader(body, int64(readLimit)))
	if err != nil {
		return nil, err
	}
	if n, err := body.Read(make([]byte, 1)); err == nil && 0 < n {
		return nil, ErrReadLimitReached
	}
	return data, nil
}

type StubReader struct {
	Data []byte

	ReadErr  error
	CloseErr error

	IsClosed bool

	index int
}

func (r *StubReader) Read(p []byte) (int, error) {
	if r.ReadErr != nil {
		return 0, r.ReadErr
	}
	if len(r.Data) <= r.index {
		return 0, io.EOF
	}
	n := copy(p, r.Data[r.index:])
	r.index += n
	return n, nil
}

func (r *StubReader) Close() error {
	r.IsClosed = true
	return r.CloseErr
}
