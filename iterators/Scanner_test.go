package iterators_test

import (
	"bufio"
	"io"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/stretchr/testify/require"
)

func TestScanner_SingleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	i := iterators.NewScanner(NewReadCloser(strings.NewReader("Hello, World!")))

	var s string

	require.True(t, i.Next())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.False(t, i.Next())
}

func TestScanner_ClosableIOGiven_OnCloseItIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators.NewScanner(NewReadCloser(strings.NewReader(`Hy`)))

	require.Nil(t, i.Close())
	require.Error(t, i.Close(), "already closed")
}

func TestScanner_MultipleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	i := iterators.NewScanner(NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!")))

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

func TestScanner_NilReaderGiven_ErrorReturned(t *testing.T) {
	t.Parallel()

	i := iterators.NewScanner(NewReadCloser(new(BrokenReader)))

	var s string

	require.False(t, i.Next())
	require.Error(t, io.ErrUnexpectedEOF, i.Decode(&s))
}

func ExampleScanner_Split() *iterators.Scanner {
	reader := strings.NewReader("a\nb\nc\nd")
	i := iterators.NewScanner(reader)
	i.Split(bufio.ScanLines)
	return i
}

func TestScanner_Split(t *testing.T) {
	i := ExampleScanner_Split()

	lines := []string{}
	iterators.CollectAll(i, &lines)
	require.Equal(t, 4, len(lines))
	require.Equal(t, `a`, lines[0])
	require.Equal(t, `b`, lines[1])
	require.Equal(t, `c`, lines[2])
	require.Equal(t, `d`, lines[3])
}
