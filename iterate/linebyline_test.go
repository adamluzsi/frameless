package iterate_test

import (
	"io"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/dataproviders"

	"github.com/adamluzsi/frameless/iterate"
	"github.com/stretchr/testify/require"
)

var _ dataproviders.IteratorBuilder = iterate.LineByLine

func TestLineByLine_SingleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	i := iterate.LineByLine(strings.NewReader("Hello, World!"))

	var s string

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.False(t, i.More())
}

func TestLineByLine_MultipleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	i := iterate.LineByLine(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!"))

	var s string

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Hello, World!", s)

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "How are you?", s)

	require.True(t, i.More())
	require.Nil(t, i.Decode(&s))
	require.Equal(t, "Thanks I'm fine!", s)

	require.False(t, i.More())
}

type brokenReader struct{}

func (b *brokenReader) Read(p []byte) (n int, err error) { return 0, io.ErrUnexpectedEOF }

func TestLineByLine_NilReaderGiven_ErrorReturned(t *testing.T) {
	t.Parallel()

	i := iterate.LineByLine(new(brokenReader))

	var s string

	require.False(t, i.More())
	require.Error(t, io.ErrUnexpectedEOF, i.Decode(&s))
}
