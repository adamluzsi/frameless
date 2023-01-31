package ranges_test

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/ports/iterators"
	iteratorcontracts "github.com/adamluzsi/frameless/ports/iterators/iteratorcontracts"
	"github.com/adamluzsi/frameless/ports/iterators/ranges"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func ExampleInt() {
	iter := ranges.Int(1, 9)
	defer iter.Close()

	for iter.Next() {
		// prints characters between 1 and 9
		// 1, 2, 3, 4, 5, 6, 7, 8, 9
		fmt.Println(iter.Value())
	}

	if err := iter.Err(); err != nil {
		panic(err.Error())
	}
}

func TestInt_smoke(t *testing.T) {
	it := assert.MakeIt(t)

	vs, err := iterators.Collect(ranges.Int(1, 9))
	it.Must.NoError(err)
	it.Must.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}, vs)

	vs, err = iterators.Collect(ranges.Int(4, 7))
	it.Must.NoError(err)
	it.Must.Equal([]int{4, 5, 6, 7}, vs)

	vs, err = iterators.Collect(ranges.Int(5, 1))
	it.Must.NoError(err)
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
	subject := testcase.Let(s, func(t *testcase.T) iterators.Iterator[int] {
		return ranges.Int(begin.Get(t), end.Get(t))
	})

	s.Then("it returns an iterator that contains the defined numeric range from min to max", func(t *testcase.T) {
		actual, err := iterators.Collect(subject.Get(t))
		t.Must.NoError(err)

		var expected []int
		for i := begin.Get(t); i <= end.Get(t); i++ {
			expected = append(expected, i)
		}

		t.Must.NotEmpty(expected)
		t.Must.Equal(expected, actual)
	})
}

func TestInt_implementsIterator(t *testing.T) {
	iteratorcontracts.Iterator[int]{
		MakeSubject: func(tb testing.TB) iterators.Iterator[int] {
			t := testcase.ToT(&tb)
			min := t.Random.IntB(3, 7)
			max := t.Random.IntB(8, 13)
			return ranges.Int(min, max)
		},
	}.Test(t)
}
