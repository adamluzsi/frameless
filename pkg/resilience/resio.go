package resilience

import (
	"bufio"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"time"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/mathkit"
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
	var rs = GetRetryStrategy(rr.RetryStrategy)
	for range rs.Retry(ctx) {
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

type TransferManager struct {
	RetryStrategy RetryStrategy
	BufferSize    iokit.ByteSize // default 16 Megabyte
}

// Transfer copies the Source stream into the Output stream.
// Both streams are closed before Transfer returns.
//
// Reading from the Source is made resilient: on a transient read failure the
// Source is re-opened and replayed from the current offset, and the stream is
// kept alive during slow writes to avoid idle read timeouts on remote sources.
func (m TransferManager) Transfer(ctx context.Context,
	source func() (io.ReadCloser, error),
	output func() (io.WriteCloser, error),
) (rErr error) {

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

	// If the output already holds partially (or fully) transferred data, resume
	// from where it left off instead of re-transferring from the beginning.
	// We rely on the output exposing its current size via a Stat method
	// (as os.File does); the source is then replayed from that offset.
	resumeOffset, err := outputSize(dst)
	if err != nil {
		return fmt.Errorf("failed to inspect the output for resuming: %w", err)
	}

	// RetryReader re-opens the Source on transient read failures and replays
	// from the current offset, providing a seamless read stream even when the
	// underlying (remote) source flakes.
	rr := &Reader{
		Open: func() (io.Reader, error) {
			return source()
		},
		RetryStrategy: GetRetryStrategy(m.RetryStrategy),
		Context:       ctx,
	}

	// Seed the offset so the source is seeked forward past the bytes already
	// present in the output, making the transfer resumable.
	if 0 < resumeOffset {
		// Position the output's write cursor at its end so the resumed bytes are
		// appended rather than overwriting from the start. This makes resuming
		// correct even when the output was not opened in append mode.
		if seeker, ok := dst.(io.Seeker); ok {
			if _, err := seeker.Seek(0, io.SeekEnd); err != nil {
				return fmt.Errorf("failed to seek the output to resume: %w", err)
			}
			// if we can seek the local file, we should continue from where we left with the reader
			rr.offset = rr.offset.Of(int(resumeOffset))
		} else if ft, ok := dst.(fileTruncate); ok {
			if err := ft.Truncate(0); err != nil {
				return fmt.Errorf("failed to truncate destination file: %w", err)
			}
		}
	}

	// keep the (potentially remote) source alive across slow writes.
	// Closing the keep-alive reader cascades to the RetryReader and,
	// in turn, to the currently open Source stream.
	keptAlive := iokit.NewKeepAliveReader(rr, 5*time.Second)
	defer errorkit.Finish(&rErr, keptAlive.Close)

	// buffer 16mb worth of reads to improve throughput from the remote source.
	var buffered = bufio.NewReaderSize(keptAlive, cmp.Or(m.BufferSize, 16*iokit.Megabyte))

	var src io.Reader = buffered

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to transfer: %w", err)
	}

	return nil
}

type fileStat interface {
	Stat() (fs.FileInfo, error)
}

type fileTruncate interface {
	Truncate(size int64) error
}

// outputSize reports how many bytes the output stream already holds.
//
// When the output exposes a Stat method that returns an fs.FileInfo
// (as *os.File does), its current size is used to resume an interrupted
// transfer. Outputs without such a method are treated as empty (size 0).
func outputSize(output io.Writer) (int64, error) {
	stater, ok := output.(fileStat)
	if !ok {
		return 0, nil
	}
	info, err := stater.Stat()
	if err != nil {
		return 0, err
	}
	if info == nil {
		return 0, nil
	}
	if size := info.Size(); 0 < size {
		return size, nil
	}
	return 0, nil
}
