package buffers

import (
	"bytes"
	"io"
	"io/fs"

	"github.com/adamluzsi/frameless"
)

const ErrSeekNegativePosition frameless.Error = "buffers: negative position"

func New[T []byte | string](data T) *Buffer {
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
