package iterators_test

import (
	"bufio"
	"fmt"
	"github.com/adamluzsi/testcase/pp"
	"io"
	"strings"
	"testing"

	"go.llib.dev/frameless/ports/iterators"

	"github.com/adamluzsi/testcase/assert"
)

func ExampleScanner() {
	reader := strings.NewReader("a\nb\nc\nd")
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanLines)
	i := iterators.BufioScanner[string](sc, nil)
	for i.Next() {
		fmt.Println(i.Value())
	}
	fmt.Println(i.Err())
}

func ExampleScanner_Split() {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i := iterators.BufioScanner[string](scanner, nil)
	for i.Next() {
		fmt.Println(i.Value())
	}
	fmt.Println(i.Err())
}

func TestScanner_SingleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("Hello, World!"))
	i := iterators.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.Must(t).True(i.Next())
	assert.Must(t).Equal("Hello, World!", i.Value())
	assert.Must(t).False(i.Next())
}

func TestScanner_nilCloserGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("foo\nbar\nbaz"))
	i := iterators.BufioScanner[string](bufio.NewScanner(readCloser), nil)

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
	i := iterators.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)
	assert.Must(t).Nil(i.Close())
	assert.Must(t).NotNil(i.Close(), "already closed")
}

func TestScanner_MultipleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!"))
	i := iterators.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

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
	i := iterators.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.Must(t).False(i.Next())
	assert.Must(t).NotNil(io.ErrUnexpectedEOF, assert.Message(pp.Format(i.Err())))
}

func TestScanner_Split(t *testing.T) {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i := iterators.BufioScanner[string](scanner, nil)

	lines, err := iterators.Collect[string](i)
	assert.Must(t).Nil(err)
	assert.Must(t).Equal(4, len(lines))
	assert.Must(t).Equal(`a`, lines[0])
	assert.Must(t).Equal(`b`, lines[1])
	assert.Must(t).Equal(`c`, lines[2])
	assert.Must(t).Equal(`d`, lines[3])
}
