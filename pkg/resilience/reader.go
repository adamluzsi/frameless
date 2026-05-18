package resilience

import (
	"context"
	"errors"
	"fmt"
	"io"

	"go.llib.dev/frameless/pkg/iokit"
	"go.llib.dev/frameless/pkg/mathkit"
)

// RetryReader will provide a seamless io.Reader | io.ReadCloser experience,
// while if an error occurs during reading, the underlying reader is restored
type RetryReader[RU RetryUnit] struct {
	Context     context.Context
	Open        func() (io.Reader, error)
	RetryPolicy RetryPolicy[RU]

	reader io.Reader
	offset mathkit.BigInt[int]
	closed bool
}

var _ io.Reader = (*RetryReader[FailureCount])(nil)

func (rr *RetryReader[RU]) Read(p []byte) (int, error) {
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

var _ io.Closer = (*RetryReader[FailureCount])(nil)

func (rr *RetryReader[RU]) restart() error {
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
	for range Retries(ctx, rr.RetryPolicy) {
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

func (rr *RetryReader[RU]) Close() error {
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

func (rr *RetryReader[RU]) tryClose(r io.Reader) error {
	if r == nil {
		return nil
	}
	if c, ok := r.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (rr *RetryReader[RU]) seekFunc(r io.Reader) func(n int) (error, bool) {
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
