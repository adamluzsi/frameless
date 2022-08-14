package iterators_test

import (
	"bufio"
	"io"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase/assert"
)

func TestScanner_SingleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("Hello, World!"))
	i := iterators.Scanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("Hello, World!", i.Value())
	assert.Must(t).False(i.Next())
}

func TestScanner_nilCloserGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("foo\nbar\nbaz"))
	i := iterators.Scanner[string](bufio.NewScanner(readCloser), nil)

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("foo", i.Value())
	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("bar", i.Value())
	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("baz", i.Value())
	assert.Must(t).False(i.Next())
	assert.Must(t).Nil(i.Close())
}

func TestScanner_ClosableIOGiven_OnCloseItIsClosed(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader(`Hy`))
	i := iterators.Scanner[string](bufio.NewScanner(readCloser), readCloser)
	assert.Must(t).Nil(i.Close())
	assert.Must(t).NotNil(i.Close(), "already closed")
}

func TestScanner_MultipleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!"))
	i := iterators.Scanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("Hello, World!", i.Value())

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("How are you?", i.Value())

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("Thanks I'm fine!", i.Value())

	assert.Must(t).False(i.Next())
}

func TestScanner_NilReaderGiven_ErrorReturned(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(new(BrokenReader))
	i := iterators.Scanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.Must(t).False(i.Next())
	assert.Must(t).NotNil(io.ErrUnexpectedEOF, i.Err())
}

func ExampleScanner_Split() *iterators.ScannerIter[string] {
	reader := strings.NewReader("a\nb\nc\nd")
	i := iterators.Scanner[string](bufio.NewScanner(reader), nil)
	i.Split(bufio.ScanLines)
	return i
}

func TestScanner_Split(t *testing.T) {
	i := ExampleScanner_Split()

	lines, err := iterators.Collect[string](i)
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(4, len(lines))
	assert.Must(t).Equal(`a`, lines[0])
	assert.Must(t).Equal(`b`, lines[1])
	assert.Must(t).Equal(`c`, lines[2])
	assert.Must(t).Equal(`d`, lines[3])
}
