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

func ExampleInt() {
	for n := range ranges.Int(1, 9) {
		// prints characters between 1 and 9
		// 1, 2, 3, 4, 5, 6, 7, 8, 9
		fmt.Println(n)
	}
}

func TestInt_smoke(t *testing.T) {
	it := assert.MakeIt(t)

	vs := iterkit.Collect(ranges.Int(1, 9))
	it.Must.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}, vs)

	vs = iterkit.Collect(ranges.Int(4, 7))
	it.Must.Equal([]int{4, 5, 6, 7}, vs)

	vs = iterkit.Collect(ranges.Int(5, 1))
	it.Must.Equal([]int{}, vs)
}

func TestInt(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		begin = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, 7)
		})
		end = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(8, 13)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[int] {
		return ranges.Int(begin.Get(t), end.Get(t))
	})

	s.Then("it returns an iterator that contains the defined numeric range from min to max", func(t *testcase.T) {
		actual := iterkit.Collect(subject.Get(t))

		var expected []int
		for i := begin.Get(t); i <= end.Get(t); i++ {
			expected = append(expected, i)
		}

		t.Must.NotEmpty(expected)
		t.Must.Equal(expected, actual)
	})
}

func TestInt_implementsIterator(t *testing.T) {
	iterkitcontract.IteratorWithRelease[int](func(tb testing.TB) iter.Seq[int] {
		t := testcase.ToT(&tb)
		min := t.Random.IntB(3, 7)
		max := t.Random.IntB(8, 13)
		return ranges.Int(min, max)
	}).Test(t)
}
