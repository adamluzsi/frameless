package iokit

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/mathkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/testcase/clock"
)

const ErrSeekNegativePosition errorkitlite.Error = "iokit: negative position"

const ErrClosed errorkitlite.Error = "iokit: read/write on closed io"

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
		return 0, ErrClosed
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

const ErrReadLimitReached errorkitlite.Error = "request-entity-too-large"

func ReadAllWithLimit(body io.Reader, readLimit ByteSize) (_ []byte, returnErr error) {
	if body == nil { // TODO:TEST_ME
		return []byte{}, nil
	}
	if closer, ok := body.(io.ReadCloser); ok { // TODO:TEST_ME
		defer errorkitlite.Finish(&returnErr, closer.Close)
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

// NewLockstepReaders creates virtual clones of an io.Reader.
// Each clone can be read independently by different goroutines.
//
// This helps when several utilities need to use the io.Reader API,
// but each would normally consume the input, leaving it unavailable for others.
//
// LockstepReaders generates n synchronized io.Reader instances that share
// the same underlying source reader. All readers receive identical data and move
// through the stream in perfect sync with each other, using a shared reading buffer window.
//
// The bufferWindow parameter sets the maximum amount of data that can be
// buffered into memory. If a faster reader gets ahead of the slowest by more
// than bufferWindow bytes, it will pause until the slower reader catches up,
// keeping all readers within the same buffering window range.
//
// This is useful for scenarios where multiple consumers must process the
// same stream concurrently while also having memory constraints,
// such as replicated processing or parallel analysis of identical input.
//
// The returned readers must be used concurrently across different goroutines.
// If any reader encounters an error (including io.EOF), all readers will see
// the same error at the same logical position in the stream.
//
// Parameters:
//
//	r – the source io.Reader to read from
//	n – number of synchronized readers to create (must be greater than 0)
//	bufferWindow – maximum buffer size before blocking fast readers
//
// Returns a slice containing n synchronized io.ReadCloser instances.
func LockstepReaders(r io.Reader, n int, bufferWindow ByteSize) []io.ReadCloser {
	if r == nil {
		panic("iokit.LockstepReaders input reader is nil")
	}
	if n < 0 {
		panic("iokit.LockstepReaders reader multiplexing count must be non-negative")
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
	// var rwm sync.RWMutex
	return &lockstepSource{
		reader: r,
		wcap:   bufferWindow,
		buffer: make([]byte, bufferWindow),
		// mutex:  &rwm,
		// phaser: sync.NewCond(&rwm),
	}
}

type lockstepSource struct {
	reader io.Reader
	err    error

	mutex  sync.RWMutex
	phaser synckit.Phaser

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

func (src *lockstepSource) tail() mathkit.BigInt[int] {
	return src.index.Add(src.index.Of(src.wlen))
}

func (src *lockstepSource) NewOutput() *lockstepOutput {
	lo := &lockstepOutput{id: len(src.outs), src: src}
	src.outs = append(src.outs, lo)
	return lo
}

func (src *lockstepSource) advancebleIndexDistance() int {
	if src.err != nil {
		return 0
	}
	var (
		// once is requires here because we can't rely on zero value comparison.
		once                       sync.Once
		indexOfTheMostBehindOutput mathkit.BigInt[int]
	)
	for _, out := range src.outs {
		if out.isClosed() {
			// ignore readers who are already done
			continue
		}
		once.Do(func() { indexOfTheMostBehindOutput = out.index })
		if out.index.Compare(indexOfTheMostBehindOutput) < 0 {
			indexOfTheMostBehindOutput = out.index
		}
	}
	var distance = src.indexToInt(indexOfTheMostBehindOutput.Sub(src.index))
	return distance
}

func (*lockstepSource) indexToInt(bi mathkit.BigInt[int]) int {
	n, ok := bi.ToInt()
	if !ok {
		n, ok = iterkit.First(bi.Iter())
		if !ok {
			return 0
		}
	}
	return n
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

func (src *lockstepSource) readersLen() int {
	var n int
	for range src.readers() {
		n++
	}
	return n
}

func (src *lockstepSource) readers() iter.Seq[*lockstepOutput] {
	return func(yield func(*lockstepOutput) bool) {
		for _, r := range src.outs {
			if r.isClosed() {
				continue
			}
			if !yield(r) {
				return
			}
		}
	}
}

func (src *lockstepSource) advance() bool {
	var initialTail = src.tail()
	src.fillBuffer()
	var toAdvance = min(src.advancebleIndexDistance(), src.wlen)
	if toAdvance == 0 {
		return initialTail.Compare(src.tail()) < 0
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

func (out *lockstepOutput) waitForWindowUpdate() {
	out.src.mutex.Lock()
	defer out.src.mutex.Unlock()

	if out.src.err != nil {
		out.src.phaser.Broadcast()
		return
	}

	if out.src.advance() {
		out.src.phaser.Broadcast()
		return
	}

	if areEveryoneElseWaiting := out.src.phaser.Len() == out.src.readersLen()-1; areEveryoneElseWaiting {
		// wake up others, at least someone else should be able to advance
		// else it would be a deadlock
		out.src.phaser.Broadcast()
	}

	// no chance to advance for us,
	// we have to wait until the one that was the most behind will be able to read.
	out.src.phaser.Wait(&out.src.mutex)
}

// shiftLeft shifts the elements of s left by k positions.
// Vacated slots at the end are set to zero.
func (src *lockstepSource) shiftLeft(n int) {
	if n == 0 {
		return
	}
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
	id    int
	src   *lockstepSource
	index mathkit.BigInt[int]
	done  int32
}

func (out *lockstepOutput) Close() error {
	if atomic.CompareAndSwapInt32(&out.done, 0, 1) {
		out.src.mutex.RLock()
		defer out.src.mutex.RUnlock()
		out.src.phaser.Broadcast()
	}
	return nil
}

func (out *lockstepOutput) isClosed() bool {
	return atomic.LoadInt32(&out.done) == 1
}

func (out *lockstepOutput) Read(p []byte) (int, error) {
	var pIndex int
	for {
		if out.isClosed() {
			return 0, ErrClosed
		}
		// fast path
		n, err := out.read(p[pIndex:])
		if 0 < n {
			pIndex += n
		}
		if len(p) <= pIndex {
			return pIndex, err
		}
		if err != nil {
			return pIndex, err
		}
		// slow path
		out.waitForWindowUpdate()
	}
}

func (out *lockstepOutput) readErr() error {
	if out.isSourceFinished() {
		return out.src.err
	}
	return nil
}

func (out *lockstepOutput) read(p []byte) (_ int, rErr error) {
	out.src.mutex.RLock()
	defer out.src.mutex.RUnlock()
	n := out.pull(p)
	err := out.readErr()
	return n, err
}

func (out *lockstepOutput) pull(p []byte) int {
	start, ok := out.index.Sub(out.src.index).ToInt()
	if !ok {
		// potentially this will never be false,
		// since the max expectec diff between the reader index and the source index
		// is a max windows size, and a max windows size already fit into memory
		panic("implementation issue")
	}

	window := out.src.window()

	if len(window) < start { // TODO: is this even possible
		return 0
	}

	var current = window[start:]
	switch {
	// NO POSSIBLE READ // slow path
	case len(current) == 0:
		return 0

	// FULL READ IS POSSIBLE // fast path
	case len(p) <= len(current):
		n := len(p)
		copy(p, current[:n])
		out.advanceIndexBy(n)
		return n

	// PARTIAL READ IS POSSIBLE // fast path
	case len(current) < len(p):

		n := len(current)
		copy(p, current)
		out.advanceIndexBy(n)
		return n

	default:
		return 0
	}
}

func (out *lockstepOutput) advanceIndexBy(n int) {
	out.index = out.index.Add(out.index.Of(n))
	// since an output source index advanced, it became possible that maybe the buffering window can move forward
	out.src.phaser.Broadcast()
}

func (out *lockstepOutput) isSourceFinished() bool {
	if out.src.err == nil {
		return false
	}
	sourceLastPosition := out.src.index.Add(out.src.index.Of(out.src.wlen))
	return out.index.Compare(sourceLastPosition) == 0
}
