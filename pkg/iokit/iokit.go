package iokit

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"

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

func ReadAllWithLimit(body io.Reader, readLimit ByteSize) (_ []byte, returnErr error) {
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
	Data     []byte
	ReadErr  error
	CloseErr error

	index  int
	lock   sync.RWMutex
	readAt time.Time
	closed bool
}

func (r *StubReader) Read(p []byte) (int, error) {
	r.lock.Lock()
	r.readAt = clock.Now()
	r.lock.Unlock()

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
	r.lock.Lock()
	r.closed = true
	r.lock.Unlock()
	return r.CloseErr
}

func (r *StubReader) LastReadAt() time.Time {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.readAt
}

func (r *StubReader) IsClosed() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.closed
}

func NewKeepAliveReader(r io.Reader, d time.Duration) *KeepAliveReader {
	kar := &KeepAliveReader{Source: r, IdleTimeout: d}
	kar.Init()
	return kar
}

// KeepAliveReader is a decorator designed to help prevent read timeouts.
// When working with external readers in a streaming fashion, such as an `http.Request#Body`,
// regular reads are crucial to avoid IO closures due to read timeouts.
// For example, If processing a single stream element takes longer than the read timeout on the source server,
// the server might close the input stream, causing issues in stream processing.
// NewKeepAliveReader addresses this by regularly reading a byte from the stream when there are no Read calls
// and buffering it for the next Read call.
type KeepAliveReader struct {
	Source io.Reader // | io.ReadCloser
	// IdleTimeout specifies how long the KeepAliveReader will wait without any read operations
	// before it reads from the Source to prevent an I/O timeout.
	IdleTimeout time.Duration

	buffer []byte
	eof    bool

	readError  error
	closeError error

	onInit  sync.Once
	onClose sync.Once
	lock    sync.Mutex
	done    chan struct{}
	beats   chan struct{}
}

func (r *KeepAliveReader) Init() {
	r.onInit.Do(func() {
		r.done = make(chan struct{})
		r.beats = make(chan struct{})
		go r.keepAlive()
	})
}

func (r *KeepAliveReader) Read(p []byte) (int, error) {
	r.Init()
	r.beat()

	r.lock.Lock()
	defer r.lock.Unlock()

	var n = len(p)

	if len(r.buffer) < n { // if buffer doesn't have enough, attempt to load more
		r.bufferAhead(n - len(r.buffer))
	}

	if len(r.buffer) == 0 && r.eof {
		return 0, io.EOF
	}

	if len(r.buffer) < n { // handling EOF case
		n = len(r.buffer)
	}

	copy(p, r.buffer[0:n])
	r.buffer = r.buffer[n:]

	return n, r.readError
}

func (r *KeepAliveReader) Close() error {
	r.onClose.Do(func() {
		r.Init()
		close(r.done)
		if c, ok := r.Source.(io.Closer); ok {
			r.closeError = c.Close()
		}
	})
	return r.closeError

}

func (r *KeepAliveReader) beat() {
	defer recover()
	select {
	case r.beats <- struct{}{}:
	default:
	}
}

func (r *KeepAliveReader) timeout() time.Duration {
	if r.IdleTimeout != 0 {
		return r.IdleTimeout
	}
	const defaultTimeout = 10 * time.Second
	return defaultTimeout
}

func (r *KeepAliveReader) keepAlive() {
	ticker := clock.NewTicker(r.timeout())
	for {
		select {
		case <-r.done:
			return

		case <-r.beats:
			ticker.Reset(r.timeout())

		case <-ticker.C:
			r.lock.Lock()
			r.bufferAhead(1)
			stop := r.eof || r.readError != nil
			r.lock.Unlock()
			if stop { // garbage collect this goroutine when the source is no longer readable
				return
			}
		}
	}
}

func (r *KeepAliveReader) bufferAhead(byteLength int) {
	if r.eof || r.readError != nil {
		return
	}
	d := make([]byte, byteLength) // read a byte from the input stream
	n, err := r.Source.Read(d)
	if 0 < n {
		r.buffer = append(r.buffer, d[0:n]...)
	}
	if errors.Is(err, io.EOF) {
		r.eof = true
	}
	if err != nil {
		r.readError = err
	}
}
