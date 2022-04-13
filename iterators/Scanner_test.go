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

	i := iterators.NewScanner[string](NewReadCloser(strings.NewReader("Hello, World!")))

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("Hello, World!", i.Value())
	assert.Must(t).False(i.Next())
}

func TestScanner_ClosableIOGiven_OnCloseItIsClosed(t *testing.T) {
	t.Parallel()

	i := iterators.NewScanner[string](NewReadCloser(strings.NewReader(`Hy`)))
	assert.Must(t).Nil(i.Close())
	assert.Must(t).NotNil(i.Close(), "already closed")
}

func TestScanner_MultipleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	i := iterators.NewScanner[string](NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!")))

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

	i := iterators.NewScanner[string](NewReadCloser(new(BrokenReader)))

	assert.Must(t).False(i.Next())
	assert.Must(t).NotNil(io.ErrUnexpectedEOF, i.Err())
}

func ExampleScanner_Split() *iterators.Scanner[string] {
	reader := strings.NewReader("a\nb\nc\nd")
	i := iterators.NewScanner[string](reader)
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
