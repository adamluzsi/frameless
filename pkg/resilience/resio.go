package resilience

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/mathkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/option"
)

// Reader will provide a seamless io.Reader | io.ReadCloser experience,
// while if an error occurs during reading, the underlying reader is restored
type Reader struct {
	Context       context.Context
	Open          func() (io.Reader, error)
	RetryStrategy RetryStrategy

	reader io.Reader
	offset mathkit.BigInt[int]
	closed bool
}

var _ io.Reader = (*Reader)(nil)

func (rr *Reader) Read(p []byte) (int, error) {
	if rr.closed {
		return 0, iokit.ErrClosed
	}

	if rr.reader == nil {
		if err := rr.restart(); err != nil {
			return 0, err
		}
	}

read:
	n, err := rr.reader.Read(p)
	if err == nil {
		rr.offset = rr.offset.Add(rr.offset.Of(n))
		return n, nil
	}
	if errors.Is(err, io.EOF) {
		rr.offset = rr.offset.Add(rr.offset.Of(n))
		return n, err
	}
	if err := rr.restart(); err != nil {
		return n, err
	}
	goto read
}

var _ io.Closer = (*Reader)(nil)

// Source is the current io.Reader source which is being used by the RetryReader.
// Interacting with source should be avoided,
// the only purpose of it would be to access stat related data,
// such as if the source is a fs.File, then accessing fs.File.Stat
func (rr *Reader) Source() io.Reader {
	if rr.reader == nil {
		rr.restart()
	}
	return rr.reader
}

func (rr *Reader) restart() error {
	if rr.closed {
		return iokit.ErrClosed
	}
	_ = rr.Close()
	rr.closed = false
	var lastError error
	var attempt = func() (io.Reader, error, bool) {
		var r, err = rr.Open()
		if err != nil {
			return r, err, false
		}
		if r == nil {
			return r, fmt.Errorf("nil io.Reader from %T#Open", rr), false
		}
		if rr.offset.IsZero() {
			return r, nil, true
		}
		var seek = rr.seekFunc(r)
		for n := range rr.offset.Iter() {
			err, ok := seek(n)
			if err != nil {
				return r, err, ok
			}
		}
		return r, nil, true
	}
	var ctx = rr.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if rr.RetryStrategy == nil {
		return fmt.Errorf("missing RetryStrategy in %T", rr)
	}
	for range rr.RetryStrategy.Retry(ctx) {
		r, err, ok := attempt()
		if err == nil && ok {
			rr.reader = r
			return nil
		}
		if err != nil {
			lastError = err
			if errors.Is(err, io.EOF) && ok {
				rr.reader = r
				return err
			}
			_ = rr.tryClose(r)
			if !ok {
				continue
			}
		}
	}
	if lastError != nil {
		return lastError
	}
	if rr.reader == nil {
		return fmt.Errorf("%T failed restart io.Reader", rr)
	}
	return nil
}

func (rr *Reader) Close() error {
	if rr.closed {
		return iokit.ErrClosed
	}
	if rr.reader != nil {
		_ = rr.tryClose(rr.reader)
	}
	rr.reader = nil
	rr.closed = true
	return nil
}

func (rr *Reader) tryClose(r io.Reader) error {
	if r == nil {
		return nil
	}
	if c, ok := r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (rr *Reader) seekFunc(r io.Reader) func(n int) (error, bool) {
	return func(offset int) (error, bool) {
		if seeker, ok := any(r).(io.Seeker); ok {
			for delta := 0; delta < offset; {
				got, err := seeker.Seek(int64(offset), io.SeekCurrent)
				half := got / 2
				delta += int(half)
				delta += int(got - half)
				if err != nil {
					return err, delta == offset
				}
			}
			return nil, true
		} else {
			offset := int64(offset)
			delta, err := io.CopyN(io.Discard, r, offset)
			return err, delta == offset
		}
	}
}

// Transfer copies the Source stream into the Output stream.
// Both streams are closed before Transfer returns.
//
// Reading from the Source is made resilient: on a transient read failure the
// Source is re-opened and replayed from the current offset, and the stream is
// kept alive during slow writes to avoid idle read timeouts on remote sources.
func Transfer(ctx context.Context,
	source func() (io.ReadCloser, error),
	output func() (io.WriteCloser, error),
	opts ...TransferOption) (rErr error) {

	c := option.ToConfig(opts)

	if source == nil {
		return fmt.Errorf("missing Source dependency in Transfer")
	}
	if output == nil {
		return fmt.Errorf("missing Output dependency in Transfer")
	}

	dst, err := output()
	if err != nil {
		return fmt.Errorf("failed to open the output: %w", err)
	}
	defer errorkit.Finish(&rErr, dst.Close)

	// RetryReader re-opens the Source on transient read failures and replays
	// from the current offset, providing a seamless read stream even when the
	// underlying (remote) source flakes.
	rr := &Reader{
		Open: func() (io.Reader, error) {
			return source()
		},
		RetryStrategy: c.getRetryStrategy(),
		Context:       ctx,
	}

	// keep the (potentially remote) source alive across slow writes.
	// Closing the keep-alive reader cascades to the RetryReader and,
	// in turn, to the currently open Source stream.
	keptAlive := iokit.NewKeepAliveReader(rr, 5*time.Second)
	defer errorkit.Finish(&rErr, keptAlive.Close)

	// buffer 16mb worth of reads to improve throughput from the remote source.
	buffered := bufio.NewReaderSize(keptAlive, 16*iokit.Megabyte)

	w := &progressWriter{Writer: dst, report: c.reportProgress}
	r := &ctxReader{ctx: ctx, Reader: buffered}

	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("failed to transfer: %w", err)
	}

	return nil
}

// ctxReader aborts the copy as soon as the context is done.
type ctxReader struct {
	ctx context.Context
	io.Reader
}

func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

// progressWriter is an io.Writer proxy that reports the cumulative
// number of bytes written through the report callback.
type progressWriter struct {
	Writer io.Writer

	report  func(TransferProgress)
	written int64
}

var _ io.Writer = (*progressWriter)(nil)

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.written += int64(n)
	if w.report != nil {
		w.report(TransferProgress{Written: w.written})
	}
	return n, err
}

type TransferOption option.Option[TransferConfig]

type TransferConfig struct {
	RetryStrategy RetryStrategy
	// OnProgress is an optional observer invoked while the stream
	// is being copied. It can be used to report transfer progress.
	OnProgress func(TransferProgress)
}

// TransferProgress describes the state of an ongoing transfer.
type TransferProgress struct {
	Name      string
	TotalSize int64
	Written   int64
}

func (c TransferConfig) Configure(t *TransferConfig) {
	*t = reflectkit.MergeStruct(*t, c)
}

func (c TransferConfig) getRetryStrategy() RetryStrategy {
	if c.RetryStrategy != nil {
		return c.RetryStrategy
	}
	return Jitter{}
}

func (c TransferConfig) reportProgress(p TransferProgress) {
	if c.OnProgress != nil {
		c.OnProgress(p)
	}
}
