package iterateover_test

import (
	"io"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterateover"
	"github.com/stretchr/testify/require"
)

var _ func(io.Reader) frameless.Iterator = iterateover.LineByLine

func TestLineByLine_SingleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	i := iterateover.LineByLine(NewReadCloser(strings.NewReader("Hello, World!")))

	var s string

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.False(t, i.Next())
}

func TestLineByLine_ClosableIOGiven_OnCloseItIsClosed(t *testing.T) {
	t.Parallel()

	i := iterateover.LineByLine(NewReadCloser(strings.NewReader(`Hy`)))

	require.Nil(t, i.Close())
	require.Error(t, i.Close(), "already closed")
}

func TestLineByLine_MultipleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	i := iterateover.LineByLine(NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!")))

	var s string

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "How are you?", s)

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Thanks I'm fine!", s)

	require.False(t, i.Next())
}

type brokenReader struct{}

func (b *brokenReader) Read(p []byte) (n int, err error) { return 0, io.ErrUnexpectedEOF }

func TestLineByLine_NilReaderGiven_ErrorReturned(t *testing.T) {
	t.Parallel()

	i := iterateover.LineByLine(NewReadCloser(new(brokenReader)))

	var s string

	require.False(t, i.Next())
	require.Error(t, io.ErrUnexpectedEOF, i.Decode(&s))
}
