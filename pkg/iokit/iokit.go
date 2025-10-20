package iokit

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"sync"
	"time"

	"go.llib.dev/frameless/pkg/compare"
	"go.llib.dev/frameless/pkg/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/mathkit"
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

type RuneReader interface {
	ReadRune() (r rune, size int, err error)
}

type RuneUnreader interface {
	UnreadRune() error
}

type RuneWriter interface {
	WriteRune(r rune) (n int, err error)
}

type runePeeker interface {
	RuneReader
	RuneUnreader
}

func PeekRune(in runePeeker) (rune, int, error) {
	char, size, err := in.ReadRune()
	if err != nil {
		return char, size, err
	}
	if err := in.UnreadRune(); err != nil {
		return char, size, err
	}
	return char, size, err
}

func MoveRune(in RuneReader, out RuneWriter) (rune, int, error) {
	char, size, err := in.ReadRune()
	if err != nil {
		return char, size, err
	}
	if _, err := out.WriteRune(char); err != nil {
		return 0, 0, err
	}
	return char, size, nil
}

type ByteReader interface {
	ReadByte() (byte, error)
}

type ByteUnreader interface {
	UnreadByte() error
}

type ByteWriter interface {
	WriteByte(c byte) error
}

type bytePeeker interface {
	ByteReader
	ByteUnreader
}

func PeekByte(in bytePeeker) (byte, error) {
	b, err := in.ReadByte()
	if err != nil {
		return 0, err
	}
	if err := in.UnreadByte(); err != nil {
		return b, err
	}
	return b, nil
}

func MoveByte(in ByteReader, out ByteWriter) (byte, error) {
	b, err := in.ReadByte()
	if err != nil {
		return b, err
	}
	if err := out.WriteByte(b); err != nil {
		return 0, err
	}
	return b, nil
}

// LockstepReaders creates n synchronized io.Reader instances that read from the same
// underlying source r in lockstep. All readers receive identical data and progress
// through the stream in perfect synchronization.
//
// The bufferWindow parameter controls the maximum amount of data that can be buffered
// internally. When the fastest reader gets ahead of the slowest by bufferWindow bytes,
// the fast readers will block until the slowest catches up, ensuring all readers
// remain within the same buffer window.
//
// This is useful for scenarios where multiple consumers need to process the same
// data stream concurrently while maintaining strict synchronization, such as
// replicated processing or parallel analysis of identical input.
//
// The returned readers are safe for concurrent use across different goroutines.
// If any reader encounters an error (including io.EOF), all readers will see
// the same error at the same logical position in the stream.
//
// Parameters:
//   - r: the source io.Reader to read from
//   - n: number of synchronized readers to create (must be > 0)
//   - bufferWindow: maximum buffer size before blocking fast readers
//
// Returns a slice of n synchronized io.Reader instances.
func LockstepReaders(r io.Reader, n int, bufferWindow ByteSize) []io.ReadCloser {
	if r == nil {
		panic("iokit.LockstepReaders input reader is nil")
	}
	if n <= 0 {
		panic("iokit.LockstepReaders reader multiplexing count must be greater than zero")
	}
	if bufferWindow <= 0 {
		panic("iokit.LockstepReaders buffer window size must be greater than zero")
	}
	src := newLockstepSource(r, bufferWindow)
	var out []io.ReadCloser
	for range n {
		out = append(out, src.NewOutput())
	}
	return out
}

func newLockstepSource(r io.Reader, bufferWindow ByteSize) *lockstepSource {
	var rwm sync.RWMutex
	return &lockstepSource{
		reader: r,
		wcap:   bufferWindow,
		buffer: make([]byte, bufferWindow),
		mutex:  &rwm,
		phaser: sync.NewCond(&rwm),
	}
}

type lockstepSource struct {
	reader io.Reader
	err    error

	mutex  *sync.RWMutex
	phaser *sync.Cond

	wlen   int
	wcap   ByteSize
	buffer []byte
	index  mathkit.BigInt[int]

	outs []*lockstepOutput
}

// window is an unsafe access to the current buffer,
// the caller must hold the lock for the mutex
func (src *lockstepSource) window() []byte {
	return src.buffer[:src.wlen]
}

func (src *lockstepSource) NewOutput() *lockstepOutput {
	lo := &lockstepOutput{src: src}
	src.outs = append(src.outs, lo)
	return lo
}

func (src *lockstepSource) advanceble() int {
	if src.err != nil {
		return 0
	}
	var (
		// once is requires here because we can't rely on zero value comparison.
		once                       sync.Once
		indexOfTheMostBehindOutput mathkit.BigInt[int]
	)
	for _, out := range src.outs {
		once.Do(func() { indexOfTheMostBehindOutput = out.index })
		if compare.IsLess(out.index.Compare(indexOfTheMostBehindOutput)) {
			indexOfTheMostBehindOutput = out.index
		}
	}
	ToAdvance := indexOfTheMostBehindOutput.Sub(src.index)
	toAdvance, ok := ToAdvance.ToInt()
	if !ok {
		toAdvance, ok = iterkit.First(ToAdvance.Iter())
		if !ok {
			return 0
		}
	}
	if src.wcap < toAdvance {
		toAdvance = src.wcap
	}
	return toAdvance
}

func (src *lockstepSource) fillBuffer() {
	if src.wcap <= src.wlen {
		return // buffer is full
	}
	if src.err != nil {
		return // no more read is possible
	}
	// fill the remaining buffer window

	n, err := io.ReadFull(src.reader, src.buffer[src.wlen:])
	if 0 < n {
		src.wlen += n
	}
	if err != nil {
		if err == io.ErrUnexpectedEOF {
			err = io.EOF
		}
		src.err = err
	}
}

func (src *lockstepSource) advance() bool {
	src.fillBuffer()
	toAdvance := src.advanceble()

	if src.wlen < toAdvance {
		toAdvance = src.wlen
	}
	switch {
	case toAdvance <= src.wlen:
		src.shiftLeft(toAdvance)
	case src.wlen < toAdvance:
		src.shiftLeft(src.wlen)
	}
	src.fillBuffer()
	return true
}

func (src *lockstepSource) WaitForWindowUpdate() {
	src.mutex.Lock()
	defer src.mutex.Unlock()

	if src.err != nil {
		return
	}

	if !src.advance() {
		// no chance to advance for us,
		// we have to wait until the one that was the most behind will be able to read.
		src.phaser.Wait()
		return
	}
	src.phaser.Broadcast()
}

// shiftLeft shifts the elements of s left by k positions.
// Vacated slots at the end are set to zero.
func (src *lockstepSource) shiftLeft(n int) {
	if len(src.buffer) < n || src.wcap < n {
		panic("not possible to shift more than the buffer window size")
	}
	if src.wlen < n {
		panic("not possible to shift more than the buffer window length")
	}
	// Move the tail of the slice forward by k positions.
	// [1,2,3,4,5] << 3
	// [4,5,0,0,0]
	// len == wlen - n  == 5 - 3 == 2
	copy(src.buffer, src.buffer[n:])
	for i := len(src.buffer) - n; i < len(src.buffer); i++ {
		src.buffer[i] = 0
	}
	src.wlen -= n
	src.index = src.index.Add(src.index.Of(n))
}

type lockstepOutput struct {
	src *lockstepSource

	index mathkit.BigInt[int]
}

func (out *lockstepOutput) Read(p []byte) (int, error) {
	var pIndex int
	for {

		n, err := out.read(p[pIndex:])

		// fast path
		if 0 < n {
			pIndex += n
		}
		if err != nil {
			return pIndex, err
		}

		if len(p) <= pIndex {
			return pIndex, nil
		}

		// slow path
		out.src.WaitForWindowUpdate()
		time.Sleep(time.Millisecond)
	}
}

func (out *lockstepOutput) read(p []byte) (_ int, rErr error) {
	out.src.mutex.RLock()
	defer out.src.mutex.RUnlock()
	defer func() { rErr = errorkitlite.Merge(out.src.err, rErr) }()

	start, ok := out.index.Sub(out.src.index).ToInt()
	if !ok {
		// potentially this will never be false,
		// since the max expectec diff between the reader index and the source index
		// is a max windows size, and a max windows size already fit into memory
		panic("implementation issue")
	}

	window := out.src.window()

	if len(window) < start || 0 < start {
		return 0, rErr
	}

	var current = window[start:]

	switch {
	// NO POSSIBLE READ // slow path
	case len(current) == 0:
		return 0, rErr

	// FULL READ IS POSSIBLE // fast path
	case len(p) <= len(current):

		n := len(p)
		copy(p, current[:n])
		out.index = out.index.Add(out.index.Of(n))
		return n, rErr

	// PARTIAL READ IS POSSIBLE // fast path
	case len(current) < len(p):

		n := len(current)
		copy(p, current)
		out.index = out.index.Add(out.index.Of(n))

		return n, rErr

	default:

		return 0, rErr
	}
}

func (out *lockstepOutput) Close() error {
	return nil
}
