package ranges_test

import (
	"fmt"
	"iter"
	"testing"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/iterkit/iterkitcontract"
	"go.llib.dev/frameless/pkg/iterkit/ranges"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
)

func ExampleChar() {
	for char := range ranges.Char('A', 'Z') {
		// prints characters between A and Z
		// A, B, C, D... Z
		fmt.Println(string(char))
	}
}

func TestChar_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	vs := iterkit.Collect(ranges.Char('A', 'C'))
	it.Must.Equal([]rune{'A', 'B', 'C'}, vs)

	vs = iterkit.Collect(ranges.Char('a', 'c'))
	it.Must.Equal([]rune{'a', 'b', 'c'}, vs)

	vs = iterkit.Collect(ranges.Char('1', '9'))
	it.Must.Equal([]rune{'1', '2', '3', '4', '5', '6', '7', '8', '9'}, vs)
}

func TestChar(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		min = testcase.Let(s, func(t *testcase.T) rune {
			chars := []rune{'A', 'B', 'C'}
			return t.Random.SliceElement(chars).(rune)
		})
		max = testcase.Let(s, func(t *testcase.T) rune {
			chars := []rune{'E', 'F', 'G'}
			return t.Random.SliceElement(chars).(rune)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[rune] {
		return ranges.Char(min.Get(t), max.Get(t))
	})

	s.Then("it returns an iterator that contains the defined character range from min to max", func(t *testcase.T) {
		actual, err := iterkit.Collect(subject.Get(t))
		t.Must.NoError(err)

		var expected []rune
		for i := min.Get(t); i <= max.Get(t); i++ {
			expected = append(expected, i)
		}

		t.Must.NotEmpty(expected)
		t.Must.Equal(expected, actual)
	})

	s.Test("smoke", func(t *testcase.T) {
		min.Set(t, 'A')
		max.Set(t, 'D')

		vs := iterkit.Collect(subject.Get(t))
		t.Must.NoError(err)
		t.Must.Equal([]rune{'A', 'B', 'C', 'D'}, vs)
	})
}

func TestChar_implementsIterator(t *testing.T) {
	iterkitcontract.Iterator[rune](func(tb testing.TB) iter.Seq[rune] {
		t := testcase.ToT(&tb)
		minChars := []rune{'A', 'B', 'C'}
		min := t.Random.SliceElement(minChars).(rune)
		maxChars := []rune{'E', 'F', 'G'}
		max := t.Random.SliceElement(maxChars).(rune)
		return ranges.Char(min, max)
	}).Test(t)
}
