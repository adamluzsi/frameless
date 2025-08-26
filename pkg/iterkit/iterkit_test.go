package iterkit_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"math/rand"
	"net/http"
	"net/url"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/iterkit/iterkitcontract"
	"go.llib.dev/frameless/pkg/mapkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/tasker"
	"go.llib.dev/frameless/port/option"
	. "go.llib.dev/frameless/testing/testent"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
)

type Entity struct {
	Text string
}

type ReadCloser struct {
	IsClosed bool
	io       io.Reader
}

func NewReadCloser(r io.Reader) *ReadCloser {
	return &ReadCloser{io: r, IsClosed: false}
}

func (rc *ReadCloser) Read(p []byte) (n int, err error) {
	return rc.io.Read(p)
}

func (rc *ReadCloser) Close() error {
	if rc.IsClosed {
		return errors.New("already closed")
	}

	rc.IsClosed = true
	return nil
}

type BrokenReader struct{}

func (b *BrokenReader) Read(p []byte) (n int, err error) { return 0, io.ErrUnexpectedEOF }

func ExampleFromSliceE() {
	var vs = []int{1, 2, 3}
	var i = iterkit.FromSliceE(vs)

	for v, err := range i {
		_, _ = v, err
	}
}

func TestFromSliceE(t *testing.T) {
	s := testcase.NewSpec(t)

	var vs = let.Var[[]int](s, nil)
	act := let.Act(func(t *testcase.T) iterkit.SeqE[int] {
		return iterkit.FromSliceE(vs.Get(t))
	})

	s.When("the slice is nil", func(s *testcase.Spec) {
		vs.LetValue(s, nil)

		s.Then("it will act as an empty iterator", func(t *testcase.T) {
			got, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			assert.Empty(t, got)
		})
	})

	s.When("the slice is empty", func(s *testcase.Spec) {
		vs.Let(s, func(t *testcase.T) []int {
			return []int{}
		})

		s.Then("it will act as an empty iterator", func(t *testcase.T) {
			got, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			assert.Empty(t, got)
		})
	})

	s.When("the slice has values", func(s *testcase.Spec) {
		vs.Let(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(1, 42), func() int {
				return t.Random.Int()
			})
		})

		s.Then("it will act as an empty iterator", func(t *testcase.T) {
			got, _ := iterkit.CollectE(act(t))
			assert.Equal(t, vs.Get(t), got)
		})

		s.Then("no error yielded", func(t *testcase.T) {
			_, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
		})
	})

	s.Context("behaves like a iter.Seq2[T, error]",
		iterkitcontract.IterSeq2(func(t testing.TB) iter.Seq2[int, error] {
			return iterkit.FromSliceE([]int{1, 2, 3})
		}).Spec)

	s.Test("supports custom slice types", func(t *testcase.T) {
		type T struct{ N int }
		type TS []T

		exp := TS(slicekit.Map(random.Slice(3, t.Random.Int), func(n int) T {
			return T{N: n}
		}))

		i := iterkit.FromSliceE(exp)

		got, err := iterkit.CollectE(i)
		assert.NoError(t, err)
		assert.Equal(t, exp, got)
	})
}

func ExampleFromSlice() {
	var vs = []int{1, 2, 3}
	var i = iterkit.FromSlice(vs)

	for v := range i {
		_ = v
	}
}

func TestFromSlice1(t *testing.T) {
	s := testcase.NewSpec(t)

	var vs = let.Var[[]int](s, nil)
	act := let.Act(func(t *testcase.T) iter.Seq[int] {
		return iterkit.FromSlice(vs.Get(t))
	})

	s.When("the slice is nil", func(s *testcase.Spec) {
		vs.LetValue(s, nil)

		s.Then("it will act as an empty iterator", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(act(t)))
		})
	})

	s.When("the slice is empty", func(s *testcase.Spec) {
		vs.Let(s, func(t *testcase.T) []int {
			return []int{}
		})

		s.Then("it will act as an empty iterator", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(act(t)))
		})
	})

	s.When("the slice has values", func(s *testcase.Spec) {
		vs.Let(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(1, 42), func() int {
				return t.Random.Int()
			})
		})

		s.Then("it will act as an empty iterator", func(t *testcase.T) {
			got := iterkit.Collect(act(t))

			assert.Equal(t, vs.Get(t), got)
		})
	})

	s.Context("it behaves like a iter.Seq2[T, error]",
		iterkitcontract.IterSeq2(func(t testing.TB) iter.Seq2[int, error] {
			return iterkit.FromSliceE([]int{1, 2, 3})
		}).Spec)
}

func ExampleLast() {
	itr := iterkit.IntRange(0, 10)

	n, ok := iterkit.Last(itr)
	_ = ok // true
	_ = n  // 10
}

func TestLast1(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		var expected int = 42
		i := iterkit.FromSlice([]int{4, 2, expected})
		actually, found := iterkit.Last(i)
		assert.True(t, found)
		assert.Equal(t, expected, actually)
	})

	s.Test("empty", func(t *testcase.T) {
		_, found := iterkit.Last(iterkit.Empty[Entity]())
		assert.False(t, found)
	})
}

func ExampleLast2() {
	var itr iter.Seq2[int, string] = func(yield func(int, string) bool) {
		for n := range iterkit.IntRange(0, 10) {
			if !yield(n, strconv.Itoa(n)) {
				return
			}
		}
	}

	num, str, ok := iterkit.Last2(itr)
	_ = ok  // true
	_ = num // 10
	_ = str // "10"
}

func TestLast2(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		expN := t.Random.IntB(10, 100)
		expS := strconv.Itoa(expN)

		var itr iter.Seq2[int, string] = func(yield func(int, string) bool) {
			for n := range iterkit.IntRange(0, expN) {
				if !yield(n, strconv.Itoa(n)) {
					return
				}
			}
		}

		num, str, ok := iterkit.Last2(itr)
		assert.True(t, ok)
		assert.Equal(t, num, expN)
		assert.Equal(t, str, expS)
	})

	s.Test("empty", func(t *testcase.T) {
		_, _, found := iterkit.Last2(iterkit.Empty2[int, string]())
		assert.False(t, found)
	})
}

func ExampleLastE() {
	itr := iterkit.IntRangeE(0, 10)

	n, ok, err := iterkit.LastE(itr)
	_ = err // nil
	_ = ok  // true
	_ = n   // 10
}

func TestLastE(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty", func(t *testcase.T) {
		_, found, err := iterkit.LastE(iterkit.Empty2[Entity, error]())
		assert.NoError(t, err)
		assert.False(t, found)
	})

	s.Test("last element returned", func(t *testcase.T) {
		var expected int = t.Random.Int()
		slc := random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		slc = append(slc, expected)
		i := iterkit.FromSliceE(slc)
		actually, found, err := iterkit.LastE(i)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, expected, actually)
	})

	s.Test("empty", func(t *testcase.T) {
		v, found, err := iterkit.LastE(iterkit.Empty2[Entity, error]())
		assert.NoError(t, err)
		assert.False(t, found)
		assert.Empty(t, v)
	})

	s.Test("the first encountered error is yielded back", func(t *testcase.T) {
		var (
			expErr = t.Random.Error()
			expVal = t.Random.Int()
		)
		var src iter.Seq2[int, error] = func(yield func(int, error) bool) {
			length := t.Random.IntBetween(3, 42)
			errIndex := t.Random.IntN(length)
			for i := range length {
				if i == errIndex {
					if !yield(expVal, expErr) {
						return
					}
				}
				if !yield(t.Random.Int(), nil) {
					return
				}
			}
		}

		got, found, err := iterkit.LastE(src)
		assert.ErrorIs(t, err, expErr)
		assert.False(t, found)
		assert.Equal(t, got, expVal)
	})
}

func TestErrorf(t *testing.T) {
	i := iterkit.ErrorF[any]("%s", "hello world!")
	vs, err := iterkit.CollectE(i)
	assert.Empty(t, vs)
	assert.Error(t, err)
	assert.Equal(t, "hello world!", err.Error())
}

var _ iter.Seq[string] = iterkit.FromSlice([]string{"A", "B", "C"})

func TestNewSlice_SliceGiven_SliceIterableAndValuesReturnedWithDecode(t *testing.T) {
	i := iterkit.FromSlice([]int{42, 4, 2})
	next, stop := iter.Pull(i)
	defer stop()

	var nextValueIs = func(t *testing.T, exp int) {
		v, ok := next()
		assert.True(t, ok, "expected that the iterator still had a value")
		assert.Equal(t, exp, v)
	}

	nextValueIs(t, 42)
	nextValueIs(t, 4)
	nextValueIs(t, 2)

	_, ok := next()
	assert.False(t, ok)
}

func ExampleFilter() {
	var i iter.Seq[int]
	i = iterkit.FromSlice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	i = iterkit.Filter[int](i, func(n int) bool { return n > 2 })

	for v := range i {
		fmt.Println(v)
	}
}

func TestFilter(t *testing.T) {
	t.Run("given the iterator has set of elements", func(t *testing.T) {
		originalInput := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		iterator := func() iter.Seq[int] { return iterkit.FromSlice(originalInput) }

		t.Run("when filter allow everything", func(t *testing.T) {
			i := iterkit.Filter(iterator(), func(int) bool { return true })
			assert.Must(t).NotNil(i)

			numbers := iterkit.Collect[int](i)
			assert.Equal(t, originalInput, numbers)
		})

		t.Run("when filter disallow part of the value stream", func(t *testing.T) {
			i := iterkit.Filter(iterator(), func(n int) bool { return 5 < n })
			assert.Must(t).NotNil(i)

			numbers := iterkit.Collect[int](i)
			assert.Equal(t, []int{6, 7, 8, 9}, numbers)
		})
	})
}

func TestFilter2(t *testing.T) {
	t.Run("given the iterator has set of elements", func(t *testing.T) {
		originalInput := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		var iterator iter.Seq2[int, int] = func(yield func(int, int) bool) {
			for _, n := range originalInput {
				if !yield(n, n*2) {
					return
				}
			}
		}

		t.Run("all source iter elements passed to the filter and filter yields all accepted back", func(t *testing.T) {
			var got []int

			i := iterkit.Filter2(iterator, func(k int, v int) bool {
				assert.Contains(t, originalInput, k)
				got = append(got, k)
				return true
			})

			assert.NotNil(t, i)
			kvs := iterkit.Collect2KV(i)
			assert.ContainsExactly(t, originalInput, got)
			assert.Equal(t, len(kvs), len(originalInput))
			for i, kv := range kvs {
				assert.Equal(t, kv.K, originalInput[i])
				assert.Equal(t, kv.K*2, kv.V)
			}
		})

		t.Run("when filter disallow part of the value stream", func(t *testing.T) {
			var exp []int = slicekit.Filter(originalInput, func(v int) bool {
				return v%2 == 0
			})

			i := iterkit.Filter2(iterator, func(k int, v int) bool {
				return k%2 == 0
			})
			assert.NotNil(t, i)

			var got []int
			for k, _ := range i {
				got = append(got, k)
			}

			assert.ContainsExactly(t, exp, got)
		})
	})
}

func ExampleFilter_withSeqE() {
	numbers := iterkit.IntRangeE(1, 100)
	evens := iterkit.Filter(numbers, func(n int) bool {
		return n%2 == 0
	})

	_ = evens
}

func TestFilter_withSeqE(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		itrErr = let.VarOf[error](s, nil)
		itrVs  = let.Var(s, func(t *testcase.T) []int {
			slice := random.Slice(t.Random.IntBetween(3, 7), func() int {
				return t.Random.Int()
			})
			t.Eventually(func(t *testcase.T) {
				n := t.Random.Int()
				assert.True(t, n%2 == 0)
				slice = append(slice, n)
			})
			return slice
		})
		itr = let.Var(s, func(t *testcase.T) iterkit.SeqE[int] {
			return func(yield func(int, error) bool) {
				for _, v := range itrVs.Get(t) {
					if !yield(v, itrErr.Get(t)) {
						return
					}
				}
			}
		})

		filter = let.Var(s, func(t *testcase.T) func(n int) bool {
			return func(n int) bool {
				return n%2 == 0
			}
		})
	)
	act := let.Act(func(t *testcase.T) iterkit.SeqE[int] {
		return iterkit.Filter(itr.Get(t), filter.Get(t))
	})

	s.Then("filter is applied", func(t *testcase.T) {
		exp := slicekit.Filter(itrVs.Get(t), filter.Get(t))
		got, err := iterkit.CollectE(act(t))
		assert.NoError(t, err)
		assert.Equal(t, exp, got)
	})

	s.Then("early iteration break is respected", func(t *testcase.T) {
		for range act(t) {
			break
		}
	})

	s.When("iterator is nil", func(s *testcase.Spec) {
		itr.LetValue(s, nil)

		s.Then("nil iterator returned", func(t *testcase.T) {
			assert.Nil(t, act(t))
		})
	})

	s.When("iteration has an error", func(s *testcase.Spec) {
		itrErr.Let(s, let.Error(s).Get)

		s.Then("error is propagated back", func(t *testcase.T) {
			_, err := iterkit.CollectE(act(t))

			assert.ErrorIs(t, err, itrErr.Get(t))
		})
	})
}

func BenchmarkFilter(b *testing.B) {
	var logic = func(n int) bool {
		return n > 500
	}

	rnd := random.New(random.CryptoSeed{})

	var values []int
	for i := 0; i < 1024; i++ {
		values = append(values, rnd.IntN(1000))
	}

	makeIterator := func() iter.Seq[int] {
		return iterkit.Filter[int](iterkit.FromSlice[int](values), logic)
	}

	var iterators = make([]iter.Seq[int], b.N)

	for i := 0; i < b.N; i++ {
		iterators[i] = makeIterator()
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for _ = range iterators[i] {
			//
		}
	}
}

func ExampleReduce() {
	raw := iterkit.FromSlice([]int{1, 2, 42})

	_, _ = iterkit.Reduce(raw, nil, func(vs []int, v int) []int {
		return append(vs, v)
	})
}

func TestReduce(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Context("iter.Seq[T]", func(s *testcase.Spec) {
		var (
			src = let.Var(s, func(t *testcase.T) []string {
				return []string{
					t.Random.StringNC(1, random.CharsetAlpha()),
					t.Random.StringNC(2, random.CharsetAlpha()),
					t.Random.StringNC(3, random.CharsetAlpha()),
					t.Random.StringNC(4, random.CharsetAlpha()),
				}
			})
			iterator = let.Var(s, func(t *testcase.T) iter.Seq[string] {
				return iterkit.FromSlice(src.Get(t))
			})
			initial = let.Var(s, func(t *testcase.T) int {
				return t.Random.Int()
			})
			reducer = let.Var(s, func(t *testcase.T) func(int, string) int {
				return func(r int, v string) int {
					return r + len(v)
				}
			})
		)
		act := func(t *testcase.T) (int, error) {
			return iterkit.Reduce(iterator.Get(t), initial.Get(t), reducer.Get(t))
		}

		s.Then("it will execute the reducing", func(t *testcase.T) {
			r, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, 1+2+3+4+initial.Get(t), r)
		})
	})

	s.Context("iter.Seq2[T, error]", func(s *testcase.Spec) {
		var (
			src = let.Var(s, func(t *testcase.T) []string {
				return []string{
					t.Random.StringNC(1, random.CharsetAlpha()),
					t.Random.StringNC(2, random.CharsetAlpha()),
					t.Random.StringNC(3, random.CharsetAlpha()),
					t.Random.StringNC(4, random.CharsetAlpha()),
				}
			})
			iterator = let.Var(s, func(t *testcase.T) iter.Seq2[string, error] {
				return iterkit.AsSeqE(iterkit.FromSlice(src.Get(t)))
			})
			initial = let.Var(s, func(t *testcase.T) int {
				return t.Random.Int()
			})
			reducer = let.Var(s, func(t *testcase.T) func(int, string) int {
				return func(r int, v string) int {
					return r + len(v)
				}
			})
		)
		act := func(t *testcase.T) (int, error) {
			return iterkit.Reduce(iterator.Get(t), initial.Get(t), reducer.Get(t))
		}

		s.Then("it will execute the reducing", func(t *testcase.T) {
			r, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, 1+2+3+4+initial.Get(t), r)
		})

		s.When("iterator encounters an error", func(s *testcase.Spec) {
			expErr := let.Error(s)

			iterator.Let(s, func(t *testcase.T) iter.Seq2[string, error] {
				return func(yield func(string, error) bool) {
					fIndex := t.Random.IntN(len(src.Get(t)))

					for i, v := range src.Get(t) {
						var err error
						if i == fIndex {
							err = expErr.Get(t)
						}
						if !yield(v, err) {
							return
						}
					}
				}
			})

			s.Then("it will return back the error", func(t *testcase.T) {
				_, err := act(t)
				assert.ErrorIs(t, err, expErr.Get(t))
			})
		})
	})
}

func ExampleReduceE() {
	raw := iterkit.FromSliceE([]int{1, 2, 42})

	_, _ = iterkit.ReduceE(raw, nil, func(vs []int, v int) []int {
		return append(vs, v)
	})
}

func TestReduceE(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		src = let.Var(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iterator = let.Var(s, func(t *testcase.T) iter.Seq2[string, error] {
			return iterkit.AsSeqE(iterkit.FromSlice(src.Get(t)))
		})
		initial = let.Var(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = let.Var(s, func(t *testcase.T) func(int, string) int {
			return func(r int, v string) int {
				return r + len(v)
			}
		})
	)
	act := func(t *testcase.T) (int, error) {
		return iterkit.ReduceE(iterator.Get(t), initial.Get(t), reducer.Get(t))
	}

	s.Then("it will execute the reducing", func(t *testcase.T) {
		r, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, 1+2+3+4+initial.Get(t), r)
	})

	s.When("iterator encounters an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		iterator.Let(s, func(t *testcase.T) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				fIndex := t.Random.IntN(len(src.Get(t)))

				for i, v := range src.Get(t) {
					var err error
					if i == fIndex {
						err = expErr.Get(t)
					}
					if !yield(v, err) {
						return
					}
				}
			}
		})

		s.Then("it will return back the error", func(t *testcase.T) {
			_, err := act(t)
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})
}

func ExampleReduce1() {
	raw := iterkit.FromSlice([]int{1, 2, 42})

	_ = iterkit.Reduce1[[]int](raw, nil, func(vs []int, v int) []int {
		return append(vs, v)
	})
}

func TestReduce1(t *testing.T) {
	s := testcase.NewSpec(t)
	var (
		src = let.Var(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iterator = let.Var(s, func(t *testcase.T) iter.Seq[string] {
			return iterkit.FromSlice(src.Get(t))
		})
		initial = let.Var(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = let.Var(s, func(t *testcase.T) func(int, string) int {
			return func(r int, v string) int {
				return r + len(v)
			}
		})
	)
	act := func(t *testcase.T) int {
		return iterkit.Reduce1(iterator.Get(t), initial.Get(t), reducer.Get(t))
	}

	s.Then("it will execute the reducing", func(t *testcase.T) {
		r := act(t)
		t.Must.Equal(1+2+3+4+initial.Get(t), r)
	})
}

func ExampleReduceErr() {
	raw := iterkit.FromSlice([]string{"1", "2", "42"})

	_, _ = iterkit.ReduceErr[[]int](raw, nil, func(vs []int, raw string) ([]int, error) {

		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, err
		}
		return append(vs, v), nil

	})
}

func TestReduceErr(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		src = let.Var(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iter = let.Var(s, func(t *testcase.T) iter.Seq[string] {
			return iterkit.FromSlice(src.Get(t))
		})
		initial = let.Var(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = let.Var(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(r int, v string) (int, error) {
				return r + len(v), nil
			}
		})
	)
	act := func(t *testcase.T) (int, error) {
		return iterkit.ReduceErr(iter.Get(t), initial.Get(t), reducer.Get(t))
	}

	s.Then("it will execute the reducing", func(t *testcase.T) {
		r, err := act(t)
		t.Must.NoError(err)
		t.Must.Equal(1+2+3+4+initial.Get(t), r)
	})

	s.When("there is an error during reducing", func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		reducer.Let(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(i int, s string) (int, error) {
				return 0, expectedErr.Get(t)
			}
		})

		s.Then("the error is propagated back", func(t *testcase.T) {
			_, err := act(t)

			assert.ErrorIs(t, err, expectedErr.Get(t))
		})
	})
}

func TestReduceErr_wSeqE(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		src = let.Var(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iterator = let.Var(s, func(t *testcase.T) iter.Seq2[string, error] {
			return iterkit.AsSeqE(iterkit.FromSlice(src.Get(t)))
		})
		initial = let.Var(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = let.Var(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(r int, v string) (int, error) {
				return r + len(v), nil
			}
		})
	)
	act := func(t *testcase.T) (int, error) {
		return iterkit.ReduceErr(iterator.Get(t), initial.Get(t), reducer.Get(t))
	}

	s.Then("it will execute the reducing", func(t *testcase.T) {
		r, err := act(t)
		t.Must.NoError(err)
		t.Must.Equal(1+2+3+4+initial.Get(t), r)
	})

	s.When("there is an error during reducing", func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		reducer.Let(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(i int, s string) (int, error) {
				return 0, expectedErr.Get(t)
			}
		})

		s.Then("the error is propagated back", func(t *testcase.T) {
			_, err := act(t)

			assert.ErrorIs(t, err, expectedErr.Get(t))
		})
	})

	s.When("there is an error in the iterators", func(s *testcase.Spec) {
		expErr := let.Error(s)
		iterator.Let(s, func(t *testcase.T) iter.Seq2[string, error] {
			return iterkit.Error[string](expErr.Get(t))
		})

		s.Then("error returned back", func(t *testcase.T) {
			_, err := act(t)
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})
}

func TestReduceEErr_wSeqE(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		src = let.Var(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iterator = let.Var(s, func(t *testcase.T) iter.Seq2[string, error] {
			return iterkit.AsSeqE(iterkit.FromSlice(src.Get(t)))
		})
		initial = let.Var(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = let.Var(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(r int, v string) (int, error) {
				return r + len(v), nil
			}
		})
	)
	act := func(t *testcase.T) (int, error) {
		return iterkit.ReduceEErr(iterator.Get(t), initial.Get(t), reducer.Get(t))
	}

	s.Then("it will execute the reducing", func(t *testcase.T) {
		r, err := act(t)
		t.Must.NoError(err)
		t.Must.Equal(1+2+3+4+initial.Get(t), r)
	})

	s.When("there is an error during reducing", func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		reducer.Let(s, func(t *testcase.T) func(int, string) (int, error) {
			return func(i int, s string) (int, error) {
				return 0, expectedErr.Get(t)
			}
		})

		s.Then("the error is propagated back", func(t *testcase.T) {
			_, err := act(t)

			assert.ErrorIs(t, err, expectedErr.Get(t))
		})
	})

	s.When("there is an error in the iterators", func(s *testcase.Spec) {
		expErr := let.Error(s)
		iterator.Let(s, func(t *testcase.T) iter.Seq2[string, error] {
			return iterkit.Error[string](expErr.Get(t))
		})

		s.Then("error returned back", func(t *testcase.T) {
			_, err := act(t)
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})
}

func ExampleFromPages() {
	ctx := context.Background()

	fetchMoreFoo := func(offset int) ([]Foo, error) {
		const limit = 10
		query := url.Values{}
		query.Set("limit", strconv.Itoa(limit))
		query.Set("offset", strconv.Itoa(offset))
		resp, err := http.Get("https://api.mydomain.com/v1/foos?" + query.Encode())
		if err != nil {
			return nil, err
		}

		var values []FooDTO
		defer resp.Body.Close()
		dec := json.NewDecoder(resp.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&values); err != nil {
			return nil, err
		}

		vs, err := dtokit.Map[[]Foo](ctx, values)
		if err != nil {
			return nil, err
		}
		if len(vs) < limit {
			return vs, iterkit.NoMore{}
		}
		return vs, nil
	}

	foos := iterkit.FromPages(fetchMoreFoo)
	_ = foos // foos can be called like any iterator,
	// and under the hood, the fetchMoreFoo function will be used dynamically,
	// to retrieve more values when the previously called values are already used up.
}

func TestFromPages(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		more = let.Var[func(offset int) ([]Foo, error)](s, nil)
	)
	act := func(t *testcase.T) iter.Seq2[Foo, error] {
		return iterkit.FromPages(more.Get(t))
	}

	s.When("more function returns no more values", func(s *testcase.Spec) {
		more.Let(s, func(t *testcase.T) func(offset int) (values []Foo, _ error) {
			return func(offset int) (values []Foo, _ error) {
				return nil, nil
			}
		})

		s.Then("iteration finishes and we get the empty result", func(t *testcase.T) {
			vs, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			assert.Empty(t, vs)
		})
	})

	s.When("the more function return a last page", func(s *testcase.Spec) {
		value := let.VarOf(s, Foo{ID: "42", Foo: "foo", Bar: "bar", Baz: "baz"})

		more.Let(s, func(t *testcase.T) func(offset int) (values []Foo, _ error) {
			return func(offset int) (values []Foo, _ error) {
				return []Foo{value.Get(t)}, iterkit.NoMore{}
			}
		})

		s.Then("we can collect that single value and return back", func(t *testcase.T) {
			vs, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			assert.Equal(t, []Foo{value.Get(t)}, vs)
		})
	})

	s.When("the more function returns back many pages", func(s *testcase.Spec) {
		values := let.VarOf[[]Foo](s, nil)

		more.Let(s, func(t *testcase.T) func(offset int) ([]Foo, error) {
			var (
				totalPageNumber = t.Random.IntBetween(3, 5)
				cur             int
			)
			return func(offset int) ([]Foo, error) {
				assert.Equal(t, len(values.Get(t)), offset,
					"expect that the offset represents the already consumed value count")
				defer func() { cur++ }()
				var vs []Foo
				t.Random.Repeat(3, 7, func() {
					vs = append(vs, rnd.Make(Foo{}).(Foo))
				})
				testcase.Append[Foo](t, values, vs...)
				if cur == totalPageNumber {
					return vs, iterkit.NoMore{}
				}
				return vs, nil
			}
		})

		s.Then("all the values received back till more function had no more values to be retrieved", func(t *testcase.T) {
			vs, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			assert.Equal(t, vs, values.Get(t))
		})
	})

	s.When("more func encountered an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		more.Let(s, func(t *testcase.T) func(offset int) (values []Foo, _ error) {
			var done int32
			return func(offset int) (values []Foo, _ error) {
				if atomic.CompareAndSwapInt32(&done, 0, 1) {
					return []Foo{MakeFoo(t)}, nil
				}
				return nil, expErr.Get(t)
			}
		})

		s.Then("the error is bubbled up to the iterator consumer", func(t *testcase.T) {
			_, err := iterkit.CollectE(act(t))
			assert.ErrorIs(t, expErr.Get(t), err)
		})
	})
}

func ExampleHeadE() {
	inf42 := func(yield func(int, error) bool) {
		for /* infinite */ {
			if !yield(42, nil) {
				return
			}
		}
	}

	i := iterkit.HeadE(inf42, 3)
	vs, err := iterkit.CollectE(i)
	_ = err // nil
	_ = vs  // []{42, 42, 42}, nil
}

func TestHeadE(t *testing.T) {
	var values iter.Seq2[string, error] = func(yield func(string, error) bool) {
		if !yield("foo", errors.New("oof")) {
			return
		}
		if !yield("bar", errors.New("rab")) {
			return
		}
		if !yield("baz", errors.New("zab")) {
			return
		}
	}
	t.Run("less", func(t *testing.T) {
		i := iterkit.HeadE(values, 2)
		got := iterkit.Collect2Map(i)
		exp := map[string]error{"foo": errors.New("oof"), "bar": errors.New("rab")}
		assert.ContainsExactly(t, exp, got)
	})

	t.Run("more", func(t *testing.T) {
		i := iterkit.HeadE(values, 5)
		got := iterkit.Collect2Map(i)
		exp := map[string]error{"foo": errors.New("oof"), "bar": errors.New("rab"), "baz": errors.New("zab")}
		assert.Equal(t, exp, got)
	})

	t.Run("inf iterator", func(t *testing.T) {
		assert.Within(t, time.Second, func(ctx context.Context) {
			infStream := iter.Seq2[int, error](func(yield func(int, error) bool) {
				for {
					if ctx.Err() != nil {
						return
					}
					if !yield(42, nil) {
						return
					}
				}
			})
			i := iterkit.HeadE(infStream, 3)
			vs, err := iterkit.CollectE(i)
			assert.NoError(t, err)
			assert.Equal(t, []int{42, 42, 42}, vs)
		})
	})

	t.Run("inf iterator with errors", func(t *testing.T) {
		assert.Within(t, time.Second, func(ctx context.Context) {
			infStream := iter.Seq2[int, error](func(yield func(int, error) bool) {
				var index int
				for {
					index++
					if ctx.Err() != nil {
						return
					}
					var msg = fmt.Sprintf("%d", index*2)
					if !yield(index, errors.New(msg)) {
						return
					}
				}
			})
			i := iterkit.HeadE(infStream, 3)
			got := iterkit.Collect2Map(i)
			assert.Equal(t, got, map[int]error{
				1: errors.New("2"),
				2: errors.New("4"),
				3: errors.New("6"),
			})
		})
	})
}

func ExampleHead() {
	inf42 := func(yield func(int) bool) {
		for /* infinite */ {
			if !yield(42) {
				return
			}
		}
	}

	i := iterkit.Head[int](inf42, 3)

	vs := iterkit.Collect(i)
	_ = vs // []{42, 42, 42}, nil
}

func TestHead1(t *testing.T) {
	t.Run("less", func(t *testing.T) {
		i := iterkit.FromSlice([]int{1, 2, 3})
		i = iterkit.Head(i, 2)
		vs := iterkit.Collect(i)
		assert.Equal(t, []int{1, 2}, vs)
	})
	t.Run("more", func(t *testing.T) {
		i := iterkit.FromSlice([]int{1, 2, 3})
		i = iterkit.Head(i, 5)
		vs := iterkit.Collect(i)
		assert.Equal(t, []int{1, 2, 3}, vs)
	})
	t.Run("inf iterator", func(t *testing.T) {
		assert.Within(t, time.Second, func(ctx context.Context) {
			infStream := iter.Seq[int](func(yield func(int) bool) {
				for {
					if ctx.Err() != nil {
						return
					}
					if !yield(42) {
						return
					}
				}
			})
			i := iterkit.Head(infStream, 3)
			vs := iterkit.Collect(i)
			assert.Equal(t, []int{42, 42, 42}, vs)
		})
	})
}

func ExampleHead2() {
	inf42 := func(yield func(int, int) bool) {
		for /* infinite */ {
			if !yield(42, 24) {
				return
			}
		}
	}

	i := iterkit.Head2[int](inf42, 3)

	vs := iterkit.Collect2Map(i)
	_ = vs // map[int]int{42:24, 42:24, 42:24}, nil
}

func TestHead2(t *testing.T) {
	var values iter.Seq2[string, int] = func(yield func(string, int) bool) {
		if !yield("foo", 42) {
			return
		}
		if !yield("bar", 7) {
			return
		}
		if !yield("baz", 13) {
			return
		}
	}
	t.Run("less", func(t *testing.T) {
		i := iterkit.Head2(values, 2)
		vs := iterkit.Collect2Map(i)
		assert.ContainsExactly(t, map[string]int{"foo": 42, "bar": 7}, vs)
	})
	t.Run("more", func(t *testing.T) {
		i := iterkit.Head2(values, 5)
		vs := iterkit.Collect2Map(i)
		assert.Equal(t, map[string]int{"foo": 42, "bar": 7, "baz": 13}, vs)
	})
	t.Run("inf iterator", func(t *testing.T) {
		assert.Within(t, time.Second, func(ctx context.Context) {
			infStream := iter.Seq2[int, int](func(yield func(int, int) bool) {
				var index int
				for {
					index++
					if ctx.Err() != nil {
						return
					}
					if !yield(index, index*2) {
						return
					}
				}
			})
			i := iterkit.Head2(infStream, 3)
			vs := iterkit.Collect2Map(i)
			assert.Equal(t, map[int]int{1: 2, 2: 4, 3: 6}, vs)
		})
	})
}

func ExampleTakeE() {
	i := iterkit.FromSliceE([]int{1, 2, 3})
	next, stop := iter.Pull2(i)
	defer stop()
	vs, err := iterkit.TakeE(next, 2)
	_ = err // nil
	_ = vs  // []int{1, 2}
}

func TestTakeE(t *testing.T) {
	t.Run("NoElementsToTake", func(t *testing.T) {
		i := iterkit.Empty2[int, error]()
		next, stop := iter.Pull2(i)
		defer stop()
		vs, err := iterkit.TakeE(next, 5)
		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	t.Run("EnoughElementsToTake", func(t *testing.T) {
		i := iterkit.FromSliceE([]int{1, 2, 3, 4, 5})
		next, stop := iter.Pull2(i)
		defer stop()
		vs, err := iterkit.TakeE(next, 3)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, vs)

		rem, err := iterkit.TakeAllE(next)
		assert.NoError(t, err)
		assert.Equal(t, rem, []int{4, 5})
	})

	t.Run("MoreElementsToTakeThanAvailable", func(t *testing.T) {
		i := iterkit.FromSliceE([]int{1, 2, 3})
		next, stop := iter.Pull2(i)
		defer stop()
		vs, err := iterkit.TakeE(next, 5)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, vs)
		_, _, ok := next()
		assert.False(t, ok, "expected no next value")
	})

	t.Run("ZeroElementsToTake", func(t *testing.T) {
		i := iterkit.FromSliceE([]int{1, 2, 3})
		next, stop := iter.Pull2(i)
		defer stop()
		vs, err := iterkit.TakeE(next, 0)
		assert.NoError(t, err)
		assert.Empty(t, vs)

		rem, err := iterkit.TakeAllE(next)
		assert.NoError(t, err)
		assert.Equal(t, rem, []int{1, 2, 3})
	})

	t.Run("NegativeNumberOfElementsToTake", func(t *testing.T) {
		i := iterkit.FromSliceE([]int{1, 2, 3})
		next, stop := iter.Pull2(i)
		defer stop()
		vs, err := iterkit.TakeE(next, -5)
		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	t.Run("when error occurs in the middle of taking", func(t *testing.T) {
		var expErr = rnd.Error()
		var i iter.Seq2[int, error] = func(yield func(int, error) bool) {
			if !yield(1, nil) {
				return
			}
			if !yield(2, nil) {
				return
			}
			if !yield(0, expErr) {
				return
			}
			if !yield(3, nil) {
				return
			}
		}

		next, stop := iter.Pull2(i)
		defer stop()

		_, err := iterkit.TakeE(next, 10)
		assert.ErrorIs(t, err, expErr)
	})
}

func ExampleTake() {
	i := iterkit.FromSlice([]int{1, 2, 3})
	next, stop := iter.Pull(i)
	defer stop()
	vs := iterkit.Take(next, 2)
	_ = vs // []int{1, 2}
}

func TestTake(t *testing.T) {
	t.Run("NoElementsToTake", func(t *testing.T) {
		i := iterkit.Empty[int]()
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 5)
		assert.Empty(t, vs)
	})

	t.Run("EnoughElementsToTake", func(t *testing.T) {
		i := iterkit.FromSlice([]int{1, 2, 3, 4, 5})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 3)
		assert.Equal(t, []int{1, 2, 3}, vs)

		rem := iterkit.TakeAll(next)
		assert.Equal(t, rem, []int{4, 5})
	})

	t.Run("MoreElementsToTakeThanAvailable", func(t *testing.T) {
		i := iterkit.FromSlice([]int{1, 2, 3})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 5)
		assert.Equal(t, []int{1, 2, 3}, vs)
		_, ok := next()
		assert.False(t, ok, "expected no next value")
	})

	t.Run("ZeroElementsToTake", func(t *testing.T) {
		i := iterkit.FromSlice([]int{1, 2, 3})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 0)
		assert.Empty(t, vs)

		rem := iterkit.TakeAll(next)
		assert.Equal(t, rem, []int{1, 2, 3})
	})

	t.Run("NegativeNumberOfElementsToTake", func(t *testing.T) {
		i := iterkit.FromSlice([]int{1, 2, 3})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, -5)
		assert.Empty(t, vs)
	})
}

func ExampleTake2() {
	kvs := maps.All(map[string]int{
		"foo": 42,
		"bar": 7,
		"baz": 13,
	})

	next, stop := iter.Pull2(kvs)
	defer stop()

	type E struct {
		Key   string
		Value int
	}

	es := iterkit.Take2[E](next, 3, func(k string, v int) E {
		return E{Key: k, Value: v}
	})

	_ = len(es) // 3
}

func TestTake2(t *testing.T) {
	var toKV = func(n int, s string) iterkit.KV[int, string] {
		return iterkit.KV[int, string]{K: n, V: s}
	}

	var values = []iterkit.KV[int, string]{
		{K: 1, V: "foo"},
		{K: 2, V: "bar"},
		{K: 3, V: "baz"},
		{K: 4, V: "qux"},
		{K: 5, V: "quux"},
	}

	t.Run("NoElementsToTake", func(t *testing.T) {
		i := iterkit.Empty2[int, string]()
		next, stop := iter.Pull2(i)
		defer stop()
		vs := iterkit.Take2(next, 5, toKV)
		assert.Empty(t, vs)
	})

	t.Run("EnoughElementsToTake", func(t *testing.T) {
		i := iterkit.FromKV(values)
		next, stop := iter.Pull2(i)
		defer stop()
		vs := iterkit.Take2(next, 2, toKV)
		assert.Equal(t, vs, values[:2])

		rem := iterkit.TakeAll2(next, toKV)
		assert.Equal(t, rem, values[2:])
	})

	t.Run("MoreElementsToTakeThanAvailable", func(t *testing.T) {
		i := iterkit.FromKV(values)
		next, stop := iter.Pull2(i)
		defer stop()
		vs := iterkit.Take2(next, len(values)+rnd.IntBetween(1, 7), toKV)
		assert.Equal(t, vs, values)
		_, _, ok := next()
		assert.False(t, ok, "expected no next value")
	})

	t.Run("ZeroElementsToTake", func(t *testing.T) {
		i := iterkit.FromKV(values)
		next, stop := iter.Pull2(i)
		defer stop()
		vs := iterkit.Take2(next, 0, toKV)
		assert.Empty(t, vs)

		rem := iterkit.TakeAll2(next, toKV)
		assert.Equal(t, rem, values)
	})

	t.Run("NegativeNumberOfElementsToTake", func(t *testing.T) {
		i := iterkit.FromKV(values)
		next, stop := iter.Pull2(i)
		defer stop()
		vs := iterkit.Take2(next, -5, toKV)
		assert.Empty(t, vs)
	})
}

func ExampleTakeAll() {
	i := iterkit.FromSlice([]int{1, 2, 3, 4, 5})
	next, stop := iter.Pull(i)
	defer stop()
	vs := iterkit.TakeAll(next)
	_ = vs // []int{1, 2, 3, 4, 5}
}

func TestTakeAll(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.FromSlice(values.Get(t))
		})
		next = let.Var(s, func(t *testcase.T) func() (int, bool) {
			next, stop := iter.Pull(itr.Get(t))
			t.Defer(stop)
			return next
		})
	)
	act := func(t *testcase.T) []int {
		return iterkit.TakeAll(next.Get(t))
	}

	s.Then("values are collected", func(t *testcase.T) {
		vs := act(t)

		assert.Equal(t, values.Get(t), vs)
	})

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs := act(t)

			assert.Empty(t, vs)
		})
	})

	s.When("iterator panics", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq[int] {
			return func(yield func(int) bool) {
				panic("boom")
			}
		})

		s.Then("panic bubles up", func(t *testcase.T) {
			assert.Panic(t, func() {
				act(t)
			})
		})
	})

	s.Test("smoke", func(t *testcase.T) {
		i := iterkit.FromSlice([]int{1, 2, 3, 4, 5})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.TakeAll(next)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, vs)
	})
}

func ExampleTakeAll2() {
	var i iter.Seq2[int, int] = func(yield func(int, int) bool) {
		for i := range 3 {
			if !yield(i, i*2) {
				return
			}
		}
	}

	next, stop := iter.Pull2(i)
	defer stop()

	vs := iterkit.TakeAll2(next, func(n int, v int) int {
		return n + v
	})

	_ = vs
}

func TestTakeAll2(t *testing.T) {
	var i iter.Seq2[int, string] = func(yield func(int, string) bool) {
		for i := range 3 {
			if !yield(i, fmt.Sprintf("%d", i)) {
				return
			}
		}
	}

	next, stop := iter.Pull2(i)
	defer stop()

	type E struct {
		N int
		V string
	}
	vs := iterkit.TakeAll2(next, func(n int, v string) E {
		return E{N: n, V: v}
	})

	assert.Equal(t, vs, []E{
		{N: 0, V: "0"},
		{N: 1, V: "1"},
		{N: 2, V: "2"},
	})
}

func ExampleTakeAll2KV() {
	var i iter.Seq2[int, int] = func(yield func(int, int) bool) {
		for i := range 3 {
			if !yield(i, i*2) {
				return
			}
		}
	}

	next, stop := iter.Pull2(i)
	defer stop()

	kvs := iterkit.TakeAll2KV(next)
	_ = kvs
}

func TestTakeAll2KV(t *testing.T) {
	var i iter.Seq2[int, string] = func(yield func(int, string) bool) {
		for i := range 3 {
			if !yield(i, fmt.Sprintf("%d", i)) {
				return
			}
		}
	}

	next, stop := iter.Pull2(i)
	defer stop()

	vs := iterkit.TakeAll2KV(next)

	assert.Equal(t, vs, []iterkit.KV[int, string]{
		{K: 0, V: "0"},
		{K: 1, V: "1"},
		{K: 2, V: "2"},
	})
}

func TestTakeAllE(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		expVS := []int{1, 2, 3, 42}
		i := iterkit.FromSliceE(expVS)
		next, stop := iter.Pull2(i)
		defer stop()
		vs, err := iterkit.TakeAllE(next)
		assert.NoError(t, err)
		assert.Equal(t, vs, expVS)
	})

	t.Run("rainy", func(t *testing.T) {
		var expErr = rnd.Error()
		var i iter.Seq2[int, error] = func(yield func(int, error) bool) {
			if !yield(1, nil) {
				return
			}
			if !yield(2, nil) {
				return
			}
			if !yield(0, expErr) {
				return
			}
			if !yield(3, nil) {
				return
			}
		}

		next, stop := iter.Pull2(i)
		defer stop()

		vs, err := iterkit.TakeE(next, 10)
		assert.ErrorIs(t, err, expErr)
		assert.Equal(t, []int{1, 2}, vs)
	})
}

func TestLimit_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterkit.LimitE(iterkit.IntRangeE(2, 6), 3)
	vs, err := iterkit.CollectE(subject)
	assert.NoError(it, err)
	assert.Equal(it, []int{2, 3, 4}, vs)
}

func TestLimit(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		itr = let.Var[iterkit.SeqE[int]](s, func(t *testcase.T) iterkit.SeqE[int] {
			return iterkit.IntRangeE(1, iterLen)
		})
		n = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen-1)
		})
	)
	act := let.Var(s, func(t *testcase.T) iterkit.SeqE[int] {
		return iterkit.LimitE(itr.Get(t), n.Get(t))
	})

	s.Then("it will limit the returned results to the expected number", func(t *testcase.T) {
		vs, err := iterkit.CollectE(act.Get(t))
		t.Must.NoError(err)
		t.Must.Equal(n.Get(t), len(vs))
	})

	s.Then("it will limited amount of value", func(t *testcase.T) {
		vs, err := iterkit.CollectE(act.Get(t))
		t.Must.NoError(err)

		t.Log("n", n.Get(t))
		var exp []int
		for i := 0; i < n.Get(t); i++ {
			exp = append(exp, i+1)
		}

		t.Must.Equal(exp, vs)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iterkit.SeqE[int] {
			return iterkit.EmptyE[int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			got, err := iterkit.CollectE(act.Get(t))
			assert.NoError(t, err)
			assert.Empty(t, got)
		})
	})

	s.When("the source iterator has less values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			vs, err := iterkit.CollectE(act.Get(t))
			t.Must.NoError(err)
			t.Must.Equal(iterLen, len(vs))
		})
	})

	s.When("the source iterator has more values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen-1)

		s.Then("it will iterate only the limited number", func(t *testcase.T) {
			got, err := iterkit.CollectE(act.Get(t))
			t.Must.NoError(err)
			t.Must.NotEmpty(got)

			total := iterkit.Collect(iterkit.IntRange(1, iterLen))
			t.Must.NotEmpty(got)

			t.Logf("%v < %v", got, total)
			t.Must.True(len(got) < len(total), "got count is less than total")
		})
	})
}

func TestLimit1_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterkit.Limit(iterkit.IntRange(2, 6), 3)
	vs := iterkit.Collect(subject)
	assert.Equal(it, []int{2, 3, 4}, vs)
}

func TestLimit1(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		itr = let.Var[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.IntRange(1, iterLen)
		})
		n = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen-1)
		})
	)
	act := let.Var(s, func(t *testcase.T) iter.Seq[int] {
		return iterkit.Limit(itr.Get(t), n.Get(t))
	})

	s.Then("it will limit the returned results to the expected number", func(t *testcase.T) {
		vs := iterkit.Collect(act.Get(t))
		t.Must.Equal(n.Get(t), len(vs))
	})

	s.Then("it will limited amount of value", func(t *testcase.T) {
		vs := iterkit.Collect(act.Get(t))

		t.Log("n", n.Get(t))
		var exp []int
		for i := 0; i < n.Get(t); i++ {
			exp = append(exp, i+1)
		}

		t.Must.Equal(exp, vs)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(act.Get(t)))
		})
	})

	s.When("the source iterator has less values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			vs := iterkit.Collect(act.Get(t))
			t.Must.Equal(iterLen, len(vs))
		})
	})

	s.When("the source iterator has more values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen-1)

		s.Then("it will iterate only the limited number", func(t *testcase.T) {
			got := iterkit.Collect(act.Get(t))
			t.Must.NotEmpty(got)

			total := iterkit.Collect(iterkit.IntRange(1, iterLen))
			t.Must.NotEmpty(got)

			t.Logf("%v < %v", got, total)
			t.Must.True(len(got) < len(total), "got count is less than total")
		})
	})
}

func TestLimit2_smoke(tt *testing.T) {
	t := testcase.NewT(tt)

	m := random.Map(5, func() (int, string) {
		return t.Random.Int(), t.Random.String()
	}, random.UniqueValues)

	var i iter.Seq2[int, string] = maps.All(m)
	subject := iterkit.Limit2(i, 3)
	vs := iterkit.Collect2Map(subject)
	assert.Contains(t, m, vs)
	t.Must.Equal(3, len(vs))
}

func TestLimit2(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		values = let.Var(s, func(t *testcase.T) map[string]int {
			return random.Map(iterLen, func() (string, int) {
				return t.Random.HexN(64), t.Random.Int()
			}, random.UniqueValues)
		})
		itr = let.Var[iter.Seq2[string, int]](s, func(t *testcase.T) iter.Seq2[string, int] {
			return maps.All(values.Get(t))
		})
		n = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen-1)
		})
	)
	act := let.Var(s, func(t *testcase.T) iter.Seq2[string, int] {
		return iterkit.Limit2(itr.Get(t), n.Get(t))
	})

	s.Then("it will limit the returned results to the expected number", func(t *testcase.T) {
		vs := iterkit.Collect2Map(act.Get(t))
		t.Must.Equal(n.Get(t), len(vs))
		assert.Contains(t, values.Get(t), vs)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq2[string, int] {
			return iterkit.Empty2[string, int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect2Map(act.Get(t)))
		})
	})

	s.When("the source iterator has less values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			vs := iterkit.Collect2Map(act.Get(t))
			t.Must.ContainsExactly(values.Get(t), vs)
		})
	})

	s.When("the source iterator has more values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen-1)

		s.Then("it will iterate only the limited number", func(t *testcase.T) {
			got := iterkit.Collect2Map(act.Get(t))
			t.Must.NotEmpty(got)
			assert.Equal(t, len(got), n.Get(t))
			assert.Contains(t, values.Get(t), got)
		})
	})
}

func TestLimit1_implementsIterator(t *testing.T) {
	iterkitcontract.IterSeq[int](func(tb testing.TB) iter.Seq[int] {
		t := testcase.ToT(&tb)
		return iterkit.Limit(
			iterkit.IntRange(1, 99),
			t.Random.IntB(1, 12),
		)
	}).Test(t)
}

var _ iter.Seq[any] = iterkit.Of[any]("")

type ExampleStruct struct {
	Name string
}

var rnd = random.New(random.CryptoSeed{})

var RandomName = fmt.Sprintf("%d", rand.Int())

func TestOf(t *testing.T) {
	t.Run("StructGiven_StructReceivedWithDecode", func(t *testing.T) {
		var expected = ExampleStruct{Name: RandomName}
		i := iterkit.Of(expected)
		actually, found := iterkit.First(i)
		assert.True(t, found)
		assert.Equal(t, expected, actually)
	})
	t.Run("StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat", func(t *testing.T) {
		var expected = ExampleStruct{Name: RandomName}

		i := iterkit.Of(expected)
		next, stop := iter.Pull(i)
		defer stop()

		v, ok := next()
		assert.True(t, ok)
		assert.Equal(t, expected, v)

		checkAmount := rnd.IntBetween(1, 100)
		for n := 0; n < checkAmount; n++ {
			_, ok := next()
			assert.False(t, ok)
		}
	})
}

func TestOf2(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("StructGiven_StructReceivedWithDecode", func(t *testcase.T) {
		var (
			exp1 = ExampleStruct{Name: t.Random.Domain()}
			exp2 = t.Random.Int()
		)
		i := iterkit.Of2(exp1, exp2)
		got1, got2, found := iterkit.First2(i)
		assert.True(t, found)
		assert.Equal(t, exp1, got1)
		assert.Equal(t, exp2, got2)
	})
	s.Test("StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat", func(t *testcase.T) {
		var (
			exp1 = ExampleStruct{Name: t.Random.Domain()}
			exp2 = t.Random.Int()
		)
		i := iterkit.Of2(exp1, exp2)

		next, stop := iter.Pull2(i)
		defer stop()

		k, v, ok := next()
		assert.True(t, ok)
		assert.Equal(t, exp1, k)
		assert.Equal(t, exp2, v)

		checkAmount := rnd.IntBetween(1, 100)
		for n := 0; n < checkAmount; n++ {
			_, _, ok := next()
			assert.False(t, ok)
		}
	})
}

func TestOfE(t *testing.T) {
	t.Run("StructGiven_StructReceivedWithDecode", func(t *testing.T) {
		var expected = ExampleStruct{Name: RandomName}
		i := iterkit.OfE(expected)
		actually, found, err := iterkit.FirstE(i)
		assert.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, expected, actually)
	})
	t.Run("StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat", func(t *testing.T) {
		var expected = ExampleStruct{Name: RandomName}
		i := iterkit.OfE(expected)
		next, stop := iter.Pull2(i)
		defer stop()

		v, err, ok := next()
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, expected, v)

		checkAmount := rnd.IntBetween(1, 100)
		for n := 0; n < checkAmount; n++ {
			_, _, ok := next()
			assert.False(t, ok)
		}
	})
}

func ExampleOffsetE() {
	_ = iterkit.Offset(iterkit.IntRange(2, 6), 2)
}

func TestOffsetE(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.FromSlice(values.Get(t))
		})
		offset = let.Var(s, func(t *testcase.T) int {
			length := len(values.Get(t))
			assert.NotEqual(t, 0, length)
			return t.Random.IntBetween(1, length)
		})
	)
	act := let.Act(func(t *testcase.T) iter.Seq[int] {
		return iterkit.Offset(itr.Get(t), offset.Get(t))
	})

	s.Test("smoke", func(t *testcase.T) {
		it := assert.MakeIt(t)
		subject := iterkit.Offset(iterkit.IntRange(2, 6), 2)
		vs := iterkit.Collect(subject)
		assert.Equal(it, []int{4, 5, 6}, vs)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			i := act(t)

			assert.Empty(t, iterkit.Collect(i))
		})
	})

	s.When("offset number is bigger than the iterator element count", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) + t.Random.IntBetween(1, 7)
		})

		s.Then("the result iterator will be empty", func(t *testcase.T) {
			got := iterkit.Collect(act(t))

			assert.Empty(t, got)
		})
	})

	s.When("offset number is equal to the iterator element count", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t))
		})

		s.Then("the result iterator will be empty", func(t *testcase.T) {
			got := iterkit.Collect(act(t))

			assert.Empty(t, got)
		})
	})

	s.When("the offset number is smaller than the iterator element count", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return t.Random.IntBetween(0, len(values.Get(t))-1)
		})

		s.Then("it will offset the results by provided offset number and yield the remainder", func(t *testcase.T) {
			got := iterkit.Collect(act(t))

			var exp = values.Get(t)[offset.Get(t):]
			assert.Equal(t, exp, got)
		})
	})

	s.Context("immplement iter.Seq1", iterkitcontract.IterSeq(func(t testing.TB) iter.Seq[int] {
		const iterLen = 10
		src := iterkit.IntRange(1, iterLen)
		return iterkit.Offset(src, iterLen/2)
	}).Spec)
}

func ExampleOffset2() {
	_ = iterkit.Offset2(iterkit.IntRangeE(2, 6), 2)
}

func TestOffset2(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(int(iterLen), func() int {
				return t.Random.Int()
			})
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq2[int, int] {
			return func(yield func(int, int) bool) {
				for _, v := range values.Get(t) {
					if !yield(v, v*2) {
						return
					}
				}
			}
		})
		offset = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen)
		})
	)
	subject := let.Var(s, func(t *testcase.T) iter.Seq2[int, int] {
		return iterkit.Offset2(itr.Get(t), offset.Get(t))
	})

	s.Test("smoke", func(t *testcase.T) {
		subject := iterkit.Offset2(func(yield func(int, int) bool) {
			for v := range iterkit.IntRange(2, 6) {
				if !yield(v, v*2) {
					return
				}
			}
		}, 2)
		vs := iterkit.Collect2KV(subject)
		assert.Equal(t, vs, []iterkit.KV[int, int]{
			{K: 4, V: 8},
			{K: 5, V: 10},
			{K: 6, V: 12},
		})
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq2[int, int] {
			return iterkit.Empty2[int, int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			i := subject.Get(t)

			assert.Empty(t, iterkit.Collect2Map(i))
		})
	})

	s.When("offset is zero", func(s *testcase.Spec) {
		offset.LetValue(s, 0)

		s.Then("all the values will be received from te source iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.ContainsExactly(t, values.Get(t), mapkit.Keys(got))
		})
	})

	s.When("the source iterator has less values than the defined offset number", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) + 1
		})

		s.Then("no value received back from the offsetted iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.Empty(t, got)
		})
	})

	s.When("the source iterator has as many values as the offset number", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t))
		})

		s.Then("no value received back from the offsetted iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.Empty(t, got)
		})
	})

	s.When("the source iterator has more values than the defined offset number", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) - t.Random.IntBetween(1, len(values.Get(t))-1)
		})

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.Equal(t, len(values.Get(t))-offset.Get(t), len(got))
			var exp1 = values.Get(t)[offset.Get(t):]
			var exp2 = slicekit.Map(exp1, func(n int) int { return n * 2 })
			assert.ContainsExactly(t, exp1, mapkit.Keys(got))
			assert.ContainsExactly(t, exp2, mapkit.Values(got))
		})
	})

	s.Context("immplement iter.Seq2", iterkitcontract.IterSeq2[int, error](func(t testing.TB) iter.Seq2[int, error] {
		const iterLen = 10
		src := iterkit.IntRangeE(1, iterLen)
		return iterkit.Offset2(src, iterLen/2)
	}).Spec)
}

func ExampleOffset() {
	_ = iterkit.OffsetE(iterkit.IntRangeE(2, 6), 2)
}

func TestOffset(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(int(iterLen), func() int {
				return t.Random.Int()
			})
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				for _, v := range values.Get(t) {
					if !yield(v, fmt.Errorf("%d", v)) {
						return
					}
				}
			}
		})
		offset = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen)
		})
	)
	subject := let.Var(s, func(t *testcase.T) iter.Seq2[int, error] {
		return iterkit.OffsetE(itr.Get(t), offset.Get(t))
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq2[int, error] {
			return iterkit.Empty2[int, error]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			i := subject.Get(t)

			assert.Empty(t, iterkit.Collect2Map(i))
		})
	})

	s.When("offset is zero", func(s *testcase.Spec) {
		offset.LetValue(s, 0)

		s.Then("all the values will be received from te source iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.ContainsExactly(t, values.Get(t), mapkit.Keys(got))
		})
	})

	s.When("the source iterator has less values than the defined offset number", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) + 1
		})

		s.Then("no value received back from the offsetted iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.Empty(t, got)
		})
	})

	s.When("the source iterator has as many values as the offset number", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t))
		})

		s.Then("no value received back from the offsetted iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.Empty(t, got)
		})
	})

	s.When("the source iterator has more values than the defined offset number", func(s *testcase.Spec) {
		offset.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) - t.Random.IntBetween(1, len(values.Get(t))-1)
		})

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got := iterkit.Collect2Map(subject.Get(t))

			assert.Equal(t, len(values.Get(t))-offset.Get(t), len(got))
			var exp1 = values.Get(t)[offset.Get(t):]
			var exp2 = slicekit.Map(exp1, func(n int) error { return fmt.Errorf("%d", n) })
			assert.ContainsExactly(t, exp1, mapkit.Keys(got))
			assert.ContainsExactly(t, exp2, mapkit.Values(got))
		})
	})

	s.Context("immplement iter.Seq2", iterkitcontract.IterSeq2[int, error](func(t testing.TB) iter.Seq2[int, error] {
		const iterLen = 10
		src := iterkit.IntRangeE(1, iterLen)
		return iterkit.OffsetE(src, iterLen/2)
	}).Spec)
}

func ExampleEmptyE() {
	_ = iterkit.EmptyE[any]()
}

func TestEmptyE(t *testing.T) {
	vs, err := iterkit.CollectE(iterkit.EmptyE[any]())
	assert.NoError(t, err)
	assert.Empty(t, vs)
}

func ExampleEmpty() {
	_ = iterkit.Empty[any]()
}

func TestEmpty1(t *testing.T) {
	assert.Empty(t, iterkit.Collect(iterkit.Empty[any]()))
}

func TestEmpty2(t *testing.T) {
	var n int
	for range iterkit.Empty2[int, int]() {
		n++
	}
	assert.Equal(t, 0, n)
}

func ExampleCollect() {
	var itr iter.Seq2[string, int]

	kvs := iterkit.Collect2KV(itr)
	_ = kvs
}

func TestCollect(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		iterator = testcase.Var[iter.Seq[int]]{ID: `iterator`}
	)
	act := func(t *testcase.T) []int {
		return iterkit.Collect(iterator.Get(t))
	}

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs := act(t)
			assert.Empty(t, vs)
		})
	})

	s.When(`iterator is nil`, func(s *testcase.Spec) {
		iterator.LetValue(s, nil)

		s.Then(`no values returned`, func(t *testcase.T) {
			vs := act(t)
			assert.Empty(t, vs)
		})
	})

	s.When(`iterator has elements`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.FromSlice([]int{1, 2, 3})
		})

		s.Then(`it will collect the values`, func(t *testcase.T) {
			vs := act(t)
			t.Must.Equal([]int{1, 2, 3}, vs)
		})
	})
}

func ExampleCollect2KV() {
	var itr iter.Seq2[string, int]

	ints := iterkit.Collect2KV(itr)
	_ = ints
}

func TestCollect2KV(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		iterator = let.Var[iter.Seq2[string, int]](s, nil)
	)
	act := func(t *testcase.T) []iterkit.KV[string, int] {
		return iterkit.Collect2KV(iterator.Get(t))
	}

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq2[string, int] {
			return iterkit.Empty2[string, int]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs := act(t)
			assert.Empty(t, vs)
		})
	})

	s.When(`iterator is nil`, func(s *testcase.Spec) {
		iterator.LetValue(s, nil)

		s.Then(`no values returned`, func(t *testcase.T) {
			vs := act(t)
			assert.Empty(t, vs)
		})
	})

	s.When(`iterator has elements`, func(s *testcase.Spec) {
		values := let.Var(s, func(t *testcase.T) []iterkit.KV[string, int] {
			return random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, int] {
				return iterkit.KV[string, int]{
					K: t.Random.String(),
					V: t.Random.Int(),
				}
			})
		})
		iterator.Let(s, func(t *testcase.T) iter.Seq2[string, int] {
			return func(yield func(string, int) bool) {
				for _, kv := range values.Get(t) {
					if !yield(kv.K, kv.V) {
						return
					}
				}
			}
		})

		s.Then(`it will collect the values`, func(t *testcase.T) {
			assert.Equal(t, values.Get(t), act(t))
		})
	})
}

func ExampleCollect2() {
	var itr iter.Seq2[string, int]

	type T struct {
		S string
		I int
	}

	ints := iterkit.Collect2(itr, func(s string, i int) T {
		return T{S: s, I: i}
	})
	_ = ints
}

func TestCollect2(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	type T struct {
		S string
		I int
	}

	var (
		iterator = let.Var[iter.Seq2[string, int]](s, nil)
	)
	act := func(t *testcase.T) []T {
		return iterkit.Collect2(iterator.Get(t), func(k string, v int) T {
			return T{S: k, I: v}
		})
	}

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq2[string, int] {
			return iterkit.Empty2[string, int]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs := act(t)
			assert.Empty(t, vs)
		})
	})

	s.When(`iterator is nil`, func(s *testcase.Spec) {
		iterator.LetValue(s, nil)

		s.Then(`no values returned`, func(t *testcase.T) {
			vs := act(t)
			assert.Empty(t, vs)
		})
	})

	s.When(`iterator has elements`, func(s *testcase.Spec) {
		values := let.Var(s, func(t *testcase.T) []T {
			return random.Slice(t.Random.IntBetween(3, 7), func() T {
				return T{
					S: t.Random.String(),
					I: t.Random.Int(),
				}
			})
		})
		iterator.Let(s, func(t *testcase.T) iter.Seq2[string, int] {
			return func(yield func(string, int) bool) {
				for _, kv := range values.Get(t) {
					if !yield(kv.S, kv.I) {
						return
					}
				}
			}
		})

		s.Then(`it will collect the values`, func(t *testcase.T) {
			assert.Equal(t, values.Get(t), act(t))
		})
	})
}

func ExampleCollect2Map() {
	var values iter.Seq2[string, int] = func(yield func(string, int) bool) {
		if !yield("foo", 42) {
			return
		}
		if !yield("bar", 7) {
			return
		}
		if !yield("baz", 13) {
			return
		}
	}

	vs := iterkit.Collect2Map(values)

	_ = vs // map[string]int{"foo": 42, "bar": 7, "baz": 13}
}

func TestCollect2Map(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		exp := random.Map(t.Random.IntBetween(3, 7), func() (string, int) {
			return t.Random.String(), t.Random.Int()
		})
		got := iterkit.Collect2Map(maps.All(exp))
		assert.ContainsExactly(t, exp, got)
	})

	s.Test("nil", func(t *testcase.T) {
		var itr iter.Seq2[string, int]
		assert.Nil(t, iterkit.Collect2Map(itr))
	})
}

func ExampleCollectEPull() {
	var itr iter.Seq2[int, error] = iterkit.AsSeqE(iterkit.IntRange(1, 10))
	vs, err := iterkit.CollectEPull(iter.Pull2(itr))
	_ = vs
	_ = err
}

func TestCollectPull(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq2[int, error] {
			return iterkit.FromSliceE(values.Get(t))
		})
		stopCalled = let.Var(s, func(t *testcase.T) bool {
			return false
		})
		next, stop = let.Var2(s, func(t *testcase.T) (func() (int, error, bool), func()) {
			next, stop := iter.Pull2(itr.Get(t))
			return next, func() {
				stopCalled.Set(t, true)
				stop()
			}
		})
	)
	act := func(t *testcase.T) ([]int, error) {
		return iterkit.CollectEPull(next.Get(t), stop.Get(t))
	}

	s.Then("values are collected", func(t *testcase.T) {
		vs, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, values.Get(t), vs)
	})

	s.Then("stop is called", func(t *testcase.T) {
		_, _ = act(t)

		assert.True(t, stopCalled.Get(t))
	})

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq2[int, error] {
			return iterkit.Empty2[int, error]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs, err := act(t)
			assert.NoError(t, err)
			assert.Empty(t, vs)
		})

		s.Then("stop is called", func(t *testcase.T) {
			_, _ = act(t)

			assert.True(t, stopCalled.Get(t))
		})
	})

	s.When("iterator panics", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				panic("boom")
			}
		})

		s.Then("panic bubles up", func(t *testcase.T) {
			assert.Panic(t, func() {
				act(t)
			})
		})

		s.Then("stop is called", func(t *testcase.T) {
			testcase.Sandbox(func() {
				act(t)
			})

			assert.True(t, stopCalled.Get(t))
		})
	})

	s.When("iterator yields an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		itr.Let(s, func(t *testcase.T) iter.Seq2[int, error] {
			errIndex := t.Random.IntN(len(values.Get(t)))
			return func(yield func(int, error) bool) {
				for i, v := range values.Get(t) {
					if i == errIndex {
						if !yield(0, expErr.Get(t)) {
							return
						}
					}
					if !yield(v, nil) {
						return
					}
				}
			}
		})

		s.Then("error bubles up", func(t *testcase.T) {
			vs, err := act(t)
			assert.ErrorIs(t, err, expErr.Get(t))
			assert.ContainsExactly(t, vs, values.Get(t))
		})

		s.Then("stop is called", func(t *testcase.T) {
			testcase.Sandbox(func() {
				act(t)
			})

			assert.True(t, stopCalled.Get(t))
		})
	})

	s.Test("supports collection without stop func", func(t *testcase.T) {
		defer stop.Get(t)()
		vs, err := iterkit.CollectEPull(next.Get(t))
		assert.NoError(t, err)
		assert.Equal(t, vs, values.Get(t))
	})
}

func ExampleCount() {
	i := iterkit.FromSlice[int]([]int{1, 2, 3})
	total := iterkit.Count[int](i)
	_ = total // 3
}

func TestCount1(t *testing.T) {
	i := iterkit.FromSlice[int]([]int{1, 2, 3})
	total := iterkit.Count[int](i)
	assert.Equal(t, 3, total)
}

func ExampleCount2() {
	itr := maps.All(map[string]int{
		"foo": 2,
		"bar": 4,
		"baz": 8,
	})
	iterkit.Count2(itr) // 3
}

func TestCount2(t *testing.T) {
	var itr iter.Seq2[string, int] = maps.All(map[string]int{
		"foo": 2,
		"bar": 4,
		"baz": 8,
	})
	total := iterkit.Count2(itr)
	assert.Equal(t, 3, total)
}

func ExampleCountE() {
	i := iterkit.FromSliceE([]int{1, 2, 3})

	n, err := iterkit.CountE(i)
	_, _ = n, err
}

func TestCountE(t *testing.T) {
	var itr iter.Seq2[string, int] = maps.All(map[string]int{
		"foo": 2,
		"bar": 4,
		"baz": 8,
	})
	total := iterkit.Count2(itr)
	assert.Equal(t, 3, total)
}

func ExampleMap() {
	rawNumbers := iterkit.FromSlice([]string{"1", "2", "42"})
	numbers := iterkit.Map[int](rawNumbers, func(v string) int {
		return len(v)
	})
	_ = numbers
}

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	inputStream := let.Var(s, func(t *testcase.T) iter.Seq[string] {
		return iterkit.FromSlice([]string{`a`, `b`, `c`})
	})
	transform := testcase.Var[func(string) string]{ID: `iterkit.MapTransformFunc`}

	subject := func(t *testcase.T) iter.Seq[string] {
		return iterkit.Map(inputStream.Get(t), transform.Get(t))
	}

	s.When(`map used, the new iterator will have the changed values`, func(s *testcase.Spec) {
		transform.Let(s, func(t *testcase.T) func(string) string {
			return func(in string) string {
				return strings.ToUpper(in)
			}
		})

		s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
			vs := iterkit.Collect[string](subject(t))

			t.Must.Equal([]string{`A`, `B`, `C`}, vs)
		})
	})

	s.Describe(`map used in a daisy chain style`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) iter.Seq[string] {
			toUpper := func(s string) string {
				return strings.ToUpper(s)
			}

			withIndex := func() func(s string) string {
				var index int
				return func(s string) string {
					defer func() { index++ }()
					return fmt.Sprintf(`%s%d`, s, index)
				}
			}

			i := inputStream.Get(t)
			i = iterkit.Map(i, toUpper)
			i = iterkit.Map(i, withIndex())

			return i
		}

		s.Then(`it will execute all the map steps in the final iterator composition`, func(t *testcase.T) {
			values := iterkit.Collect(subject(t))
			t.Must.Equal([]string{`A0`, `B1`, `C2`}, values)
		})
	})
}

func ExampleMap2() {
	itr := maps.All(map[int]string{1: "1", 2: "2", 3: "42"})

	numbers := iterkit.Map2[int, int](itr, func(k int, v string) (int, int) {
		return k, len(v)
	})

	_ = numbers
}

func TestMap2(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	var (
		yieldedValues = let.Var(s, func(t *testcase.T) []iterkit.KV[string, int] {
			return make([]iterkit.KV[string, int], 0)
		})
		length = let.IntB(s, 3, 7)
		itr    = let.Var(s, func(t *testcase.T) iter.Seq2[string, int] {
			return func(yield func(string, int) bool) {
				for range length.Get(t) {
					k := t.Random.String()
					v := t.Random.Int()
					testcase.Append(t, yieldedValues, iterkit.KV[string, int]{K: k, V: v})
					if !yield(k, v) {
						return
					}
				}
			}
		})
		transform = let.Var(s, func(t *testcase.T) func(string, int) (string, string) {
			return func(k string, v int) (string, string) {
				return k, strconv.Itoa(v)
			}
		})
	)
	act := func(t *testcase.T) iter.Seq2[string, string] {
		return iterkit.Map2(itr.Get(t), transform.Get(t))
	}

	var transformKV = func(t *testcase.T, kvs []iterkit.KV[string, int]) []iterkit.KV[string, string] {
		return slicekit.Map(kvs, func(kv iterkit.KV[string, int]) iterkit.KV[string, string] {
			nk, nv := transform.Get(t)(kv.K, kv.V)
			return iterkit.KV[string, string]{K: nk, V: nv}
		})
	}

	s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
		got := iterkit.Collect2KV(act(t))
		exp := transformKV(t, yieldedValues.Get(t))
		assert.ContainsExactly(t, exp, got)
	})

	s.Then("it respects if iteration is interupted", func(t *testcase.T) {
		expLen := t.Random.IntB(1, length.Get(t))
		got := iterkit.Collect2KV(iterkit.Head2(act(t), expLen))
		assert.Equal(t, expLen, len(got))
		assert.Equal(t, len(yieldedValues.Get(t)), expLen)
	})
}

func TestMapE_wSeqE(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	var (
		yielded = let.Var(s, func(t *testcase.T) []int {
			return []int{}
		})
		length   = let.IntB(s, 7, 12)
		iterator = let.Var(s, func(t *testcase.T) iterkit.SeqE[int] {
			return func(yield func(int, error) bool) {
				for range length.Get(t) {
					n := t.Random.Int()
					cont := yield(n, nil)
					testcase.Append(t, yielded, n)
					if !cont {
						return
					}
				}
			}
		})
		transform = let.Var(s, func(t *testcase.T) func(int) (string, error) {
			return func(n int) (string, error) {
				return strconv.Itoa(n), nil
			}
		})
	)
	act := func(t *testcase.T) iterkit.SeqE[string] {
		return iterkit.MapE(iterator.Get(t), transform.Get(t))
	}

	s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
		got, err := iterkit.CollectE(act(t))
		assert.NoError(t, err)
		exp, err := slicekit.MapErr(yielded.Get(t), transform.Get(t))
		assert.NoError(t, err)
		assert.ContainsExactly(t, exp, got)
	})

	s.Then("it respects if iteration is interupted", func(t *testcase.T) {
		expLen := t.Random.IntB(1, length.Get(t))
		got, err := iterkit.CollectE(iterkit.Head2(act(t), expLen))
		assert.NoError(t, err)
		assert.Equal(t, len(got), expLen)
		assert.Equal(t, len(yielded.Get(t)), expLen)
	})

	s.When("error occurs during transformation", func(s *testcase.Spec) {
		expErr := let.Error(s)
		errCount := let.VarOf(s, 0)

		transform.Let(s, func(t *testcase.T) func(int) (string, error) {
			trf := transform.Super(t)
			ok := length.Get(t) / 2
			return func(i int) (string, error) {
				ok--
				if 0 < ok {
					return trf(i)
				}
				errCount.Set(t, errCount.Get(t)+1)
				return "", expErr.Get(t)
			}
		})

		s.Then("error is propagated back", func(t *testcase.T) {
			_, err := iterkit.CollectE(act(t))
			assert.ErrorIs(t, err, expErr.Get(t))
		})

		s.Then("it won't stop iteration because transform had an error on a given element", func(t *testcase.T) {
			vs, _ := iterkit.CollectE(act(t))
			assert.NotEmpty(t, vs, "expected that some of the values are still processed (length/2)")
			assert.True(t, 1 < errCount.Get(t), "expected that error in transform doesn't ent the iteration")
		})
	})

	s.When("error occurs in upstream iterator", func(s *testcase.Spec) {
		expErr := let.Error(s)
		errCount := let.VarOf(s, 0)

		iterator.Let(s, func(t *testcase.T) iterkit.SeqE[int] {
			i := iterator.Super(t)
			ok := length.Get(t) / 2
			return func(yield func(int, error) bool) {
				for v, err := range i {
					ok--
					if 0 < ok {
						if !yield(v, err) {
							return
						}
						continue
					}

					errCount.Set(t, errCount.Get(t)+1)
					if !yield(0, expErr.Get(t)) {
						return
					}
				}
			}
		})

		s.Then("error is propagated back", func(t *testcase.T) {
			_, err := iterkit.CollectE(act(t))
			assert.ErrorIs(t, err, expErr.Get(t))
		})

		s.Then("it won't stop iteration because transform had an error on a given element", func(t *testcase.T) {
			vs, _ := iterkit.CollectE(act(t))
			assert.NotEmpty(t, vs, "expected that some of the values are still processed (length/2)")
			assert.True(t, 1 < errCount.Get(t), "expected that error in transform doesn't ent the iteration")
		})
	})
}

func ExampleMapE() {
	rawNumbers := iterkit.FromSlice([]string{"1", "2", "42"})
	numbers := iterkit.MapE[int](rawNumbers, strconv.Atoi)
	_ = numbers
}

func TestMapE(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	var (
		inputStream = let.Var(s, func(t *testcase.T) iter.Seq[string] {
			return iterkit.FromSlice([]string{`a`, `b`, `c`})
		})
		transform = let.Var[func(string) (string, error)](s, func(t *testcase.T) func(string) (string, error) {
			return func(in string) (string, error) {
				return strings.ToUpper(in), nil
			}
		})
	)
	act := func(t *testcase.T) iter.Seq2[string, error] {
		return iterkit.MapE(inputStream.Get(t), transform.Get(t))
	}

	s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
		vs, err := iterkit.CollectE[string](act(t))
		t.Must.NoError(err)
		t.Must.Equal([]string{`A`, `B`, `C`}, vs)
	})

	s.When(`some error happen during mapping`, func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		transform.Let(s, func(t *testcase.T) func(string) (string, error) {
			return func(string) (string, error) {
				return "", expectedErr.Get(t)
			}
		})

		s.Then(`error returned`, func(t *testcase.T) {
			vs, err := iterkit.CollectE(act(t))
			assert.ErrorIs(t, err, expectedErr.Get(t))
			assert.Empty(t, vs)
		})
	})
}

func TestMapE_daisyChain(t *testing.T) {

	s := testcase.NewSpec(t)

	var (
		inputStream = let.Var(s, func(t *testcase.T) iter.Seq[string] {
			return iterkit.FromSlice([]string{`a`, `b`, `c`})
		})
		interimErr = let.VarOf[error](s, nil)
	)

	act := let.Act(func(t *testcase.T) iter.Seq2[string, error] {
		toUpper := func(s string) (string, error) {
			return strings.ToUpper(s), interimErr.Get(t)
		}

		withIndex := func() func(s string) (string, error) {
			var index int
			return func(s string) (string, error) {
				defer func() { index++ }()
				return fmt.Sprintf(`%s%d`, s, index), nil
			}
		}

		src := inputStream.Get(t)
		i := iterkit.MapE(src, toUpper)
		i = iterkit.MapE(i, withIndex())
		return i
	})

	s.Then(`it will execute all the map steps in the final iterator composition`, func(t *testcase.T) {
		values, err := iterkit.CollectE(act(t))
		t.Must.NoError(err)
		t.Must.Equal([]string{`A0`, `B1`, `C2`}, values)
	})

	s.And("if an interim step has an error", func(s *testcase.Spec) {
		interimErr.Let(s, func(t *testcase.T) error {
			return t.Random.Error()
		})

		s.Then("error is propagated back in the returned finisher", func(t *testcase.T) {
			_, err := iterkit.CollectE(act(t))

			assert.ErrorIs(t, err, interimErr.Get(t))
		})
	})
}

func TestFirst1_NextValueDecodable_TheFirstNextValueDecoded(t *testing.T) {
	var expected int = 42
	i := iterkit.FromSlice([]int{expected, 4, 2})

	actually, found := iterkit.First[int](i)
	assert.Equal(t, expected, actually)
	assert.True(t, found)
}

func TestFirst1_WhenNextSayThereIsNoValueToBeDecoded_NotFoundReturned(t *testing.T) {
	_, found := iterkit.First[Entity](iterkit.Empty[Entity]())
	assert.False(t, found)
}

func ExampleFirst2() {
	var itr iter.Seq2[string, int] = func(yield func(string, int) bool) {
		for i := 0; i < 42; i++ {
			if !yield(strconv.Itoa(i), i) {
				return
			}
		}
	}

	k, v, ok := iterkit.First2(itr)
	_, _, _ = k, v, ok
}
func TestFirst2(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty", func(t *testcase.T) {
		_, _, found := iterkit.First2(iterkit.Empty2[Entity, error]())
		assert.False(t, found)
	})

	s.Test("iter has values", func(t *testcase.T) {
		var (
			once sync.Once
			expK = t.Random.String()
			expV = t.Random.Int()
		)

		var itr iter.Seq2[string, int] = func(yield func(string, int) bool) {
			for {
				var (
					k = t.Random.String()
					v = t.Random.Int()
				)
				once.Do(func() {
					k = expK
					v = expV
				})
				if !yield(k, v) {
					return
				}
			}
		}

		gotK, gotV, ok := iterkit.First2(itr)
		assert.True(t, ok)
		assert.Equal(t, expK, gotK)
		assert.Equal(t, expV, gotV)
	})
}

func ExampleFirstE() {
	var itr iter.Seq2[string, error] = func(yield func(string, error) bool) {
		for i := 0; i < 42; i++ {
			if !yield(strconv.Itoa(i), nil) {
				return
			}
		}
	}

	v, ok, err := iterkit.FirstE(itr)
	_, _, _ = v, ok, err
}
func TestFirstE(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty", func(t *testcase.T) {
		_, found, err := iterkit.FirstE(iterkit.Empty2[Entity, error]())
		assert.NoError(t, err)
		assert.False(t, found)
	})

	s.Test("iter has values", func(t *testcase.T) {
		var (
			once sync.Once
			exp  = t.Random.String()
		)

		var itr iterkit.SeqE[string] = func(yield func(string, error) bool) {
			for {
				var (
					v = t.Random.String()
				)
				once.Do(func() {
					v = exp
				})
				if !yield(v, nil) {
					return
				}
			}
		}

		got, ok, err := iterkit.FirstE(itr)
		assert.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, exp, got)
	})

	s.Test("iter has an error", func(t *testcase.T) {
		var (
			once   sync.Once
			expVal = t.Random.String()
			expErr = t.Random.Error()
		)

		var itr iterkit.SeqE[string] = func(yield func(string, error) bool) {
			for {
				var (
					val string
					err error
				)
				once.Do(func() { val, err = expVal, expErr })
				if !yield(val, err) {
					return
				}
			}
		}

		got, ok, err := iterkit.FirstE(itr)
		assert.ErrorIs(t, expErr, err)
		assert.True(t, ok)
		assert.Equal(t, expVal, got)
	})
}

func ExampleChanE() {
	ch := make(chan int)

	i := iterkit.ChanE(ch)

	go func() {
		defer close(ch)
		ch <- 42
	}()

	for v := range i {
		fmt.Println(v) // 42 once
	}
}

func TestChanE(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ch = let.Var(s, func(t *testcase.T) chan int {
			return make(chan int)
		})
	)
	act := func(t *testcase.T) iterkit.SeqE[int] {
		return iterkit.ChanE(ch.Get(t))
	}

	s.When("channel receives values from another goroutine", func(s *testcase.Spec) {
		values := let.Var(s, func(t *testcase.T) []int {
			vs := random.Slice(t.Random.IntBetween(3, 7), func() int {
				return t.Random.IntBetween(1, 42)
			})
			assert.NotEmpty(t, vs)
			return vs
		})

		push := let.Var(s, func(t *testcase.T) func() {
			return func() {
				defer close(ch.Get(t))
				for _, v := range values.Get(t) {
					select {
					case ch.Get(t) <- v:
					case <-t.Done():
						return
					}
				}
			}
		})

		s.Before(func(t *testcase.T) {
			go push.Get(t)()
		})

		s.Then("all values are received", func(t *testcase.T) {
			vs, err := iterkit.CollectE(act(t))
			assert.NoError(t, err)
			assert.Equal(t, values.Get(t), vs)
		})

		s.Then("iteration can be closed early", func(t *testcase.T) {
			next, stop := iter.Pull2(act(t))

			v, err, ok := next()
			assert.True(t, ok)
			assert.NoError(t, err)
			assert.Equal(t, v, values.Get(t)[0])

			assert.NotPanic(t, stop)
		})

		s.Test("race", func(t *testcase.T) {
			itr := act(t)
			doRange := func() {
				for v, err := range itr {
					assert.NoError(t, err)
					assert.NotEmpty(t, v)
				}
			}
			testcase.Race(doRange, doRange, doRange)
		})

		s.Then("collecting them on different goroutines are considered safe", func(t *testcase.T) {
			itr := act(t)

			var (
				m   sync.Mutex
				got []int
			)
			var collect = func() {
				var vs []int
				for v := range itr {
					vs = append(vs, v)
				}
				m.Lock()
				defer m.Unlock()
				got = append(got, vs...)
			}

			testcase.Race(
				collect,
				collect,
			)

			assert.ContainsExactly(t, values.Get(t), got)
		})

		s.And("if the channel is not closed due to we are still expecting values", func(s *testcase.Spec) {
			push.Let(s, func(t *testcase.T) func() {
				t.Log("given the push process doesn't close at the end")
				return func() {
					for _, v := range values.Get(t) {
						select {
						case ch.Get(t) <- v:
						case <-t.Done():
							return
						}
					}
				}
			})

			s.Then("the iteration doesn't stops after receiving values but expect more until the close signal stop the iteration", func(t *testcase.T) {
				itr := act(t)

				next, stop := iter.Pull2(itr)
				defer stop()

				assert.Within(t, time.Second, func(ctx context.Context) {
					for _, exp := range values.Get(t) {
						got, err, ok := next()
						assert.True(t, ok)
						assert.NoError(t, err)
						assert.Equal(t, exp, got)
					}
				})

				w := assert.NotWithin(t, 250*time.Millisecond, func(ctx context.Context) {
					_, _, ok := next()
					assert.False(t, ok, "we didn't expected any more values")
				})

				close(ch.Get(t))

				assert.Within(t, 250*time.Millisecond, func(ctx context.Context) {
					w.Wait()
				})
			})
		})
	})

	s.When("channel is nil", func(s *testcase.Spec) {
		ch.LetValue(s, nil)

		s.Then("we have got a non-blocking empty iterator", func(t *testcase.T) {
			assert.Within(t, time.Second, func(ctx context.Context) {
				vs, err := iterkit.CollectE(act(t))
				assert.NoError(t, err)
				assert.Empty(t, vs)
			})
		})
	})
}

func ExampleChan() {
	ch := make(chan int)

	i := iterkit.Chan(ch)

	go func() {
		defer close(ch)
		ch <- 42
	}()

	for v := range i {
		fmt.Println(v) // 42 once
	}
}

func TestChan1(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ch = let.Var(s, func(t *testcase.T) chan int {
			return make(chan int)
		})
	)
	act := func(t *testcase.T) iter.Seq[int] {
		return iterkit.Chan(ch.Get(t))
	}

	s.When("channel receives values from another goroutine", func(s *testcase.Spec) {
		values := let.Var(s, func(t *testcase.T) []int {
			vs := random.Slice(t.Random.IntBetween(3, 7), func() int {
				return t.Random.IntBetween(1, 42)
			})
			assert.NotEmpty(t, vs)
			return vs
		})

		push := let.Var(s, func(t *testcase.T) func() {
			return func() {
				defer close(ch.Get(t))
				for _, v := range values.Get(t) {
					select {
					case ch.Get(t) <- v:
					case <-t.Done():
						return
					}
				}
			}
		})

		s.Before(func(t *testcase.T) {
			go push.Get(t)()
		})

		s.Then("all values are received", func(t *testcase.T) {
			assert.Equal(t, values.Get(t), iterkit.Collect(act(t)))
		})

		s.Then("iteration can be closed early", func(t *testcase.T) {
			next, stop := iter.Pull(act(t))

			v, ok := next()
			assert.True(t, ok)
			assert.Equal(t, v, values.Get(t)[0])

			assert.NotPanic(t, stop)
		})

		s.Test("race", func(t *testcase.T) {
			itr := act(t)
			doRange := func() {
				for v := range itr {
					assert.NotEmpty(t, v)
				}
			}
			testcase.Race(doRange, doRange, doRange)
		})

		s.Then("collecting them on different goroutines are considered safe", func(t *testcase.T) {
			itr := act(t)

			var (
				m   sync.Mutex
				got []int
			)
			var collect = func() {
				var vs []int
				for v := range itr {
					vs = append(vs, v)
				}
				m.Lock()
				defer m.Unlock()
				got = append(got, vs...)
			}

			testcase.Race(
				collect,
				collect,
			)

			assert.ContainsExactly(t, values.Get(t), got)
		})

		s.And("if the channel is not closed due to we are still expecting values", func(s *testcase.Spec) {
			push.Let(s, func(t *testcase.T) func() {
				t.Log("given the push process doesn't close at the end")
				return func() {
					for _, v := range values.Get(t) {
						select {
						case ch.Get(t) <- v:
						case <-t.Done():
							return
						}
					}
				}
			})

			s.Then("the iteration doesn't stops after receiving values but expect more until the close signal stop the iteration", func(t *testcase.T) {
				itr := act(t)

				next, stop := iter.Pull(itr)
				defer stop()

				assert.Within(t, time.Second, func(ctx context.Context) {
					for _, exp := range values.Get(t) {
						got, ok := next()
						assert.True(t, ok)
						assert.Equal(t, exp, got)
					}
				})

				w := assert.NotWithin(t, 250*time.Millisecond, func(ctx context.Context) {
					_, ok := next()
					assert.False(t, ok, "we didn't expected any more values")
				})

				close(ch.Get(t))

				assert.Within(t, 250*time.Millisecond, func(ctx context.Context) {
					w.Wait()
				})
			})
		})
	})

	s.When("channel is nil", func(s *testcase.Spec) {
		ch.LetValue(s, nil)

		s.Then("we have got a non-blocking empty iterator", func(t *testcase.T) {
			assert.Within(t, time.Second, func(ctx context.Context) {
				assert.Empty(t, iterkit.Collect(act(t)))
			})
		})
	})
}

func ExampleBatch() {
	src := iterkit.IntRange(0, 1000)

	batched := iterkit.Batch(src)

	for vs := range batched {
		fmt.Printf("%#v\n", vs)
	}
}

func ExampleBatchE_withSize() {
	src := iterkit.IntRangeE(0, 1000)

	batched := iterkit.BatchE(src, iterkit.BatchSize(100))

	for vs := range batched {
		fmt.Printf("%#v\n", vs)
	}
}

func ExampleBatchE_withWaitLimit() {
	slowIterSeq := iterkit.IntRangeE(0, 1000)

	batched := iterkit.BatchE(slowIterSeq, iterkit.BatchWaitLimit(time.Second))

	// Batching will occure either when the batching size reached
	// or when the wait limit duration passed
	for vs := range batched {
		fmt.Printf("%#v\n", vs)
	}
}

func TestBatch(t *testing.T) {
	const defaultBatchSize = 64

	s := testcase.NewSpec(t)

	var (
		values = let.Var[[]int](s, func(t *testcase.T) []int {
			return random.Slice[int](t.Random.IntB(50, 200), func() int {
				return t.Random.Int()
			})
		})
		src = let.Var[iterkit.SeqE[int]](s, func(t *testcase.T) iterkit.SeqE[int] {
			return iterkit.FromSliceE(values.Get(t))
		})
		opts = let.VarOf[[]iterkit.BatchOption](s, nil)
	)
	act := func(t *testcase.T) iterkit.SeqE[[]int] {
		return iterkit.BatchE(src.Get(t), opts.Get(t)...)
	}

	var ThenIterates = func(s *testcase.Spec) {
		s.H().Helper()

		s.Then("the batches contain all elements", func(t *testcase.T) {
			i := act(t)
			var got []int
			for vs, err := range i {
				t.Must.NoError(err)
				t.Must.NotEmpty(vs)
				got = append(got, vs...)
			}
			t.Must.Equal(values.Get(t), got,
				"expected both to contain all elements and also that the order is not affected")
		})

		s.Then("early break cause no issues", func(t *testcase.T) {
			assert.NotPanic(t, func() {
				for range act(t) {
					break
				}
			})
		})
	}
	var batchSizeCases = func(s *testcase.Spec) {
		s.When("size is not configured", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c := option.ToConfig(opts.Get(t))
				assert.Empty(t, c.Size)
			})

			s.Then("iterate with default value(s)", func(t *testcase.T) {
				i := act(t)
				var got []int
				for vs, err := range i {
					t.Must.NoError(err)
					t.Must.NotEmpty(vs)
					t.Must.True(len(vs) <= defaultBatchSize, "iteration ")
					got = append(got, vs...)
				}
				t.Must.NotEmpty(got)
				t.Must.ContainsExactly(values.Get(t), got)
			})

			ThenIterates(s)
		})

		s.When("size is configured", func(s *testcase.Spec) {
			s.H().Helper()

			size := let.Var[int](s, nil)

			opts.Let(s, func(t *testcase.T) []iterkit.BatchOption {
				o := opts.Super(t)
				if t.Random.Bool() {
					o = append(o, iterkit.BatchSize(size.Get(t)))
				} else {
					o = append(o, iterkit.BatchConfig{Size: size.Get(t)})
				}
				return o
			})

			s.When("size is a valid positive value", func(s *testcase.Spec) {
				size.Let(s, func(t *testcase.T) int {
					return t.Random.IntB(1, len(values.Get(t)))
				})

				s.Then("batching size is used", func(t *testcase.T) {
					i := act(t)
					var got []int
					for vs, err := range i {
						t.Must.NoError(err)
						t.Log(len(vs) <= size.Get(t), len(vs), size.Get(t))
						t.Must.True(len(vs) <= size.Get(t))
						t.Must.NotEmpty(vs)
						got = append(got, vs...)
					}
					t.Must.NotEmpty(got)
					t.Must.ContainsExactly(values.Get(t), got)
				})

				ThenIterates(s)
			})

			s.Context("invalid size valie is ignored", func(s *testcase.Spec) {
				size.Let(s, func(t *testcase.T) int {
					// negative value is not acceptable
					// zero int makes no sense for batch size, so also ignored
					return t.Random.IntBetween(-100, 0)
				})

				s.Then("iterate with default value(s)", func(t *testcase.T) {
					i := act(t)
					var got []int
					for vs, err := range i {
						t.Must.NoError(err)
						t.Must.NotEmpty(vs)
						t.Must.True(len(vs) <= defaultBatchSize, "iteration ")
						got = append(got, vs...)
					}
					t.Must.NotEmpty(got)
					t.Must.ContainsExactly(values.Get(t), got)
				})

				ThenIterates(s)
			})
		})
	}

	batchSizeCases(s)

	s.When("wait limit is set", func(s *testcase.Spec) {
		timeout := let.DurationBetween(s, time.Millisecond, 250*time.Millisecond)

		opts.Let(s, func(t *testcase.T) []iterkit.BatchOption {
			o := opts.Super(t)
			if t.Random.Bool() {
				o = append(o, iterkit.BatchWaitLimit(timeout.Get(t)))
			} else {
				o = append(o, iterkit.BatchConfig{WaitLimit: timeout.Get(t)})
			}
			return o
		})

		ThenIterates(s)

		batchSizeCases(s)

		s.Context("a timeout that is less or equal to zero will be ignored", func(s *testcase.Spec) {
			timeout.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(t.Random.IntB(-1*int(time.Minute), 0))
			})

			ThenIterates(s)
		})

		s.When("the source iterator is slower than the batch wait time", func(s *testcase.Spec) {
			src.Let(s, func(t *testcase.T) iter.Seq2[int, error] {
				in := make(chan int)
				out := iterkit.Chan(in)

				var push = func() {
					defer close(in)
					for _, v := range values.Get(t) {
						select {
						case in <- v:
						case <-t.Done():
							return
						}
					}
					// wait forever to trigger batching
					<-t.Done()
				}

				go push()

				return iterkit.AsSeqE(out)
			})

			s.Then("batch timeout takes action and we get  corresponds to the configuration", func(t *testcase.T) {
				i := act(t)
				next, stop := iter.Pull2(i)
				defer stop()
				vs, err, ok := next()
				t.Must.True(ok, "expected that batching is triggered due to wait time limit exceeding")
				assert.NoError(t, err)
				assert.NotEmpty(t, vs)
				assert.Contains(t, values.Get(t), vs)
			})
		})
	})

	s.When("error occurs during the stream", func(s *testcase.Spec) {
		expErr := let.Error(s)

		src.Let(s, func(t *testcase.T) iterkit.SeqE[int] {
			var errIndex = t.Random.IntN(len(values.Get(t)))
			return func(yield func(int, error) bool) {
				for i, v := range values.Get(t) {
					if i == errIndex {
						var zero int
						if !yield(zero, expErr.Get(t)) {
							return
						}
					}
					if !yield(v, nil) {
						return
					}
				}
			}
		})

		s.Then("values are yielded", func(t *testcase.T) {
			var gvs []int
			for vs, err := range act(t) {
				if err == nil {
					gvs = append(gvs, vs...)
				}
			}
			assert.ContainsExactly(t, gvs, values.Get(t))
		})

		s.Then("the error yielded", func(t *testcase.T) {
			var errs []error
			for _, err := range act(t) {
				if err != nil {
					errs = append(errs, err)
				}
			}
			assert.True(t, len(errs) == 1)
			assert.ErrorIs(t, errs[0], expErr.Get(t))
		})
	})

	s.Context("behaves like a iter.Seq2[T, error]", iterkitcontract.IterSeq2(func(t testing.TB) iter.Seq2[[]int, error] {
		src := iterkit.IntRangeE(1, 100)
		return iterkit.BatchE(src, iterkit.BatchSize(25))
	}).Spec)
}

func ExampleBatch_withSize() {
	src := iterkit.IntRange(0, 1000)

	batched := iterkit.Batch(src, iterkit.BatchSize(100))

	for vs := range batched {
		fmt.Printf("%#v\n", vs)
	}
}

func ExampleBatch_withWaitLimit() {
	slowIterSeq := iterkit.IntRange(0, 1000)

	batched := iterkit.Batch(slowIterSeq, iterkit.BatchWaitLimit(time.Second))

	// Batching will occure either when the batching size reached
	// or when the wait limit duration passed
	for vs := range batched {
		fmt.Printf("%#v\n", vs)
	}
}

func TestBatch1(t *testing.T) {
	const defaultBatchSize = 64

	s := testcase.NewSpec(t)

	var (
		values = let.Var[[]int](s, func(t *testcase.T) []int {
			return random.Slice[int](t.Random.IntB(50, 200), func() int {
				return t.Random.Int()
			})
		})
		src = let.Var[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.FromSlice(values.Get(t))
		})
		opts = let.VarOf[[]iterkit.BatchOption](s, nil)
	)
	act := func(t *testcase.T) iter.Seq[[]int] {
		return iterkit.Batch(src.Get(t), opts.Get(t)...)
	}

	var ThenIterates = func(s *testcase.Spec) {
		s.H().Helper()

		s.Then("the batches contain all elements", func(t *testcase.T) {
			i := act(t)
			var got []int
			for vs := range i {
				t.Must.NotEmpty(vs)
				got = append(got, vs...)
			}
			t.Must.Equal(values.Get(t), got,
				"expected both to contain all elements and also that the order is not affected")
		})

		s.Then("early break cause no issues", func(t *testcase.T) {
			assert.NotPanic(t, func() {
				for range act(t) {
					break
				}
			})
		})
	}
	var batchSizeCases = func(s *testcase.Spec) {
		s.When("size is not configured", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				c := option.ToConfig(opts.Get(t))
				assert.Empty(t, c.Size)
			})

			s.Then("iterate with default value(s)", func(t *testcase.T) {
				i := act(t)
				var got []int
				for vs := range i {
					t.Must.NotEmpty(vs)
					t.Must.True(len(vs) <= defaultBatchSize, "iteration ")
					got = append(got, vs...)
				}
				t.Must.NotEmpty(got)
				t.Must.ContainsExactly(values.Get(t), got)
			})

			ThenIterates(s)
		})

		s.When("size is configured", func(s *testcase.Spec) {
			s.H().Helper()

			size := let.Var[int](s, nil)

			opts.Let(s, func(t *testcase.T) []iterkit.BatchOption {
				o := opts.Super(t)
				if t.Random.Bool() {
					o = append(o, iterkit.BatchSize(size.Get(t)))
				} else {
					o = append(o, iterkit.BatchConfig{Size: size.Get(t)})
				}
				return o
			})

			s.When("size is a valid positive value", func(s *testcase.Spec) {
				size.Let(s, func(t *testcase.T) int {
					return t.Random.IntB(1, len(values.Get(t)))
				})

				s.Then("batching size is used", func(t *testcase.T) {
					i := act(t)
					var got []int
					for vs := range i {
						t.Log(len(vs) <= size.Get(t), len(vs), size.Get(t))
						t.Must.True(len(vs) <= size.Get(t))
						t.Must.NotEmpty(vs)
						got = append(got, vs...)
					}
					t.Must.NotEmpty(got)
					t.Must.ContainsExactly(values.Get(t), got)
				})

				ThenIterates(s)
			})

			s.Context("invalid size valie is ignored", func(s *testcase.Spec) {
				size.Let(s, func(t *testcase.T) int {
					// negative value is not acceptable
					// zero int makes no sense for batch size, so also ignored
					return t.Random.IntBetween(-100, 0)
				})

				s.Then("iterate with default value(s)", func(t *testcase.T) {
					i := act(t)
					var got []int
					for vs := range i {
						t.Must.NotEmpty(vs)
						t.Must.True(len(vs) <= defaultBatchSize, "iteration ")
						got = append(got, vs...)
					}
					t.Must.NotEmpty(got)
					t.Must.ContainsExactly(values.Get(t), got)
				})

				ThenIterates(s)
			})
		})
	}

	batchSizeCases(s)

	s.When("wait limit is set", func(s *testcase.Spec) {
		timeout := let.DurationBetween(s, time.Millisecond, 250*time.Millisecond)

		opts.Let(s, func(t *testcase.T) []iterkit.BatchOption {
			o := opts.Super(t)
			if t.Random.Bool() {
				o = append(o, iterkit.BatchWaitLimit(timeout.Get(t)))
			} else {
				o = append(o, iterkit.BatchConfig{WaitLimit: timeout.Get(t)})
			}
			return o
		})

		ThenIterates(s)

		batchSizeCases(s)

		s.Context("a timeout that is less or equal to zero will be ignored", func(s *testcase.Spec) {
			timeout.Let(s, func(t *testcase.T) time.Duration {
				return time.Duration(t.Random.IntB(-1*int(time.Minute), 0))
			})

			ThenIterates(s)
		})

		s.When("the source iterator is slower than the batch wait time", func(s *testcase.Spec) {
			src.Let(s, func(t *testcase.T) iter.Seq[int] {
				in := make(chan int)
				out := iterkit.Chan(in)

				var push = func() {
					defer close(in)
					for _, v := range values.Get(t) {
						select {
						case in <- v:
						case <-t.Done():
							return
						}
					}
					// wait forever to trigger batching
					<-t.Done()
				}

				go push()

				return out
			})

			s.Then("batch timeout takes action and we get  corresponds to the configuration", func(t *testcase.T) {
				i := act(t)
				next, stop := iter.Pull(i)
				defer stop()
				vs, ok := next()
				t.Must.True(ok, "expected that batching is triggered due to wait time limit exceeding")
				assert.NotEmpty(t, vs)
				assert.Contains(t, values.Get(t), vs)
			})
		})
	})

	s.Context("behaves like a iter.Seq[T]", iterkitcontract.IterSeq(func(t testing.TB) iter.Seq[[]int] {
		src := iterkit.IntRange(1, 100)
		return iterkit.Batch(src, iterkit.BatchSize(25))
	}).Spec)
}

func TestError(t *testing.T) {
	expectedError := errors.New("Boom!")
	vs, err := iterkit.CollectE(iterkit.Error[any](expectedError))
	assert.Empty(t, vs)
	assert.ErrorIs(t, err, expectedError)
}

func TestErrorF(t *testing.T) {
	expectedError := errors.New("Boom!")
	vs, err := iterkit.CollectE(iterkit.ErrorF[any]("wrap:%w", expectedError))
	assert.Empty(t, vs)
	assert.ErrorIs(t, err, expectedError)
	assert.Contains(t, err.Error(), "wrap:"+expectedError.Error())
}

func ExampleScanner() {
	reader := strings.NewReader("a\nb\nc\nd")
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanLines)
	i := iterkit.BufioScanner[string](sc, nil)
	for text, err := range i {
		fmt.Println(text, err)
	}
}

func ExampleScanner_Split() {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i := iterkit.BufioScanner[string](scanner, nil)
	for line, err := range i {
		fmt.Println(line, err)
	}
}

func TestScanner_SingleLineGiven_EachLineFetched(t *testing.T) {
	readCloser := NewReadCloser(strings.NewReader("Hello, World!"))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)
	vs, err := iterkit.CollectE(i)
	assert.NoError(t, err)
	assert.True(t, readCloser.IsClosed)
	assert.NotEmpty(t, vs)
	assert.ContainsExactly(t, vs, []string{"Hello, World!"})
}

func TestScanner_nilCloserGiven_EachLineFetched(t *testing.T) {
	readCloser := NewReadCloser(strings.NewReader("foo\nbar\nbaz"))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), nil)

	next, stop := iter.Pull2(i)
	defer stop()

	v, err, ok := next()
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "foo", v)
	v, err, ok = next()
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "bar", v)
	v, err, ok = next()
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "baz", v)
	_, err, ok = next()
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestScanner_ClosableIOGiven_OnCloseItIsClosed(t *testing.T) {
	readCloser := NewReadCloser(strings.NewReader(`Hy`))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)
	for _, err := range i {
		assert.NoError(t, err)
	}
}

func TestScanner_MultipleLineGiven_EachLineFetched(t *testing.T) {
	readCloser := NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!"))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	next, stop := iter.Pull2(i)
	defer stop()

	v, err, ok := next()
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Hello, World!", v)

	v, err, ok = next()
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "How are you?", v)

	v, err, ok = next()
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "Thanks I'm fine!", v)

	_, _, ok = next()
	assert.False(t, ok)
}

func TestScanner_NilReaderGiven_ErrorReturned(t *testing.T) {
	readCloser := NewReadCloser(new(BrokenReader))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)
	vs, err := iterkit.CollectE(i)
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
	assert.Empty(t, vs)
}

func TestScanner_Split(t *testing.T) {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i := iterkit.BufioScanner[string](scanner, nil)

	lines, err := iterkit.CollectE(i)
	assert.Must(t).NoError(err)
	assert.Equal(t, 4, len(lines))
	assert.Equal(t, `a`, lines[0])
	assert.Equal(t, `b`, lines[1])
	assert.Equal(t, `c`, lines[2])
	assert.Equal(t, `d`, lines[3])
}

func ExampleSync() {
	src := iterkit.IntRange(0, 100)
	itr, cancel := iterkit.Sync(src)
	defer cancel()

	var g tasker.JobGroup[tasker.FireAndForget]
	for range 2 {
		g.Go(func(ctx context.Context) error {
			for v := range itr {
				_ = v // use v
			}
			return nil
		})
	}

	g.Join()
}

func TestSync(t *testing.T) {
	src := iterkit.IntRange(0, 100)
	itr, cancel := iterkit.Sync(src)
	defer cancel()

	var (
		m   sync.Mutex
		got []int
	)
	var collect = func() {
		var vs []int
		for v := range itr {
			vs = append(vs, v)
		}
		m.Lock()
		defer m.Unlock()
		got = append(got, vs...)
	}

	testcase.Race(collect, collect, collect)

	exp := iterkit.Collect(iterkit.IntRange(0, 100))
	assert.Must(t).ContainsExactly(exp, got)
}

func ExampleSyncE() {
	src := iterkit.IntRangeE(0, 100)
	itr, cancel := iterkit.SyncE(src)
	defer cancel()

	var g tasker.JobGroup[tasker.FireAndForget]
	for range 2 {
		g.Go(func(ctx context.Context) error {
			for v := range itr {
				_ = v // use v
			}
			return nil
		})
	}

	g.Join()
}

func TestSyncE(t *testing.T) {
	var (
		exp []iterkit.KV[string, error]
		got []iterkit.KV[string, error]
	)
	exp = random.Slice(100, func() iterkit.KV[string, error] {
		return iterkit.KV[string, error]{
			K: rnd.String(),
			V: rnd.Error(),
		}
	})

	var src iter.Seq2[string, error] = func(yield func(string, error) bool) {
		for _, kv := range exp {
			if !yield(kv.K, kv.V) {
				return
			}
		}
	}

	itr, cancel := iterkit.Sync2(src)
	defer cancel()

	var m sync.Mutex
	var collect = func() {
		kvs := iterkit.Collect2KV(itr)
		m.Lock()
		defer m.Unlock()
		got = append(got, kvs...)
	}

	testcase.Race(collect, collect, collect)
	assert.ContainsExactly(t, exp, got)
}

func ExampleSync2() {
	src := iterkit.IntRange(0, 100)
	itr, cancel := iterkit.Sync2(iterkit.AsSeqE(src))
	defer cancel()

	var g tasker.JobGroup[tasker.FireAndForget]
	for range 2 {
		g.Go(func(ctx context.Context) error {
			for v, err := range itr {
				_ = err // handle err
				_ = v   // use v
			}
			return nil
		})
	}

	g.Join()
}

func TestSync2(t *testing.T) {
	var (
		exp []iterkit.KV[string, int]
		got []iterkit.KV[string, int]
	)
	exp = random.Slice(100, func() iterkit.KV[string, int] {
		return iterkit.KV[string, int]{
			K: rnd.String(),
			V: rnd.Int(),
		}
	})

	var src iter.Seq2[string, int] = func(yield func(string, int) bool) {
		for _, kv := range exp {
			if !yield(kv.K, kv.V) {
				return
			}
		}
	}

	itr, cancel := iterkit.Sync2(src)
	defer cancel()

	var m sync.Mutex
	var collect = func() {
		kvs := iterkit.Collect2KV(itr)
		m.Lock()
		defer m.Unlock()
		got = append(got, kvs...)
	}

	testcase.Race(collect, collect, collect)
	assert.ContainsExactly(t, exp, got)
}

func TestMerge(t *testing.T) {
	t.Run("EmptyIterators", func(t *testing.T) {
		iter := iterkit.Merge[int]()
		vs := iterkit.Collect(iter)
		assert.Empty(t, vs)
	})

	t.Run("SingleIterator", func(t *testing.T) {
		iter1 := iterkit.FromSlice([]int{1, 2, 3})
		mergedIter := iterkit.Merge(iter1)
		valuses := iterkit.Collect(mergedIter)
		assert.Equal(t, valuses, []int{1, 2, 3})
	})

	t.Run("MultipleIterators", func(t *testing.T) {
		iter1 := iterkit.FromSlice([]int{1, 2})
		iter2 := iterkit.FromSlice([]int{3, 4})
		iter3 := iterkit.FromSlice([]int{5, 6})
		vs := iterkit.Collect(iterkit.Merge(iter1, iter2, iter3))
		assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, vs)
	})

	t.Run("IteratorsWithError", func(t *testing.T) {
		iter1 := iterkit.FromSlice([]int{1, 2})
		expErr := rnd.Error()
		iter2, _ := iterkit.SplitSeqE(iterkit.Error[int](expErr))
		mergedIter := iterkit.Merge(iter1, iter2)
		values := []int{}
		for v := range mergedIter {
			values = append(values, v)
		}
		assert.Equal(t, []int{1, 2}, values)
	})
}

func TestMerge2(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty", func(t *testcase.T) {
		itr := iterkit.Merge2[int, int]()
		vs := iterkit.Collect2KV(itr)
		assert.Empty(t, vs)
	})

	s.Test("single", func(t *testcase.T) {
		kvs1 := random.Slice(5, func() iterkit.KV[string, int] {
			return iterkit.KV[string, int]{
				K: t.Random.String(),
				V: t.Random.Int(),
			}
		})
		var itr1 iter.Seq2[string, int] = func(yield func(string, int) bool) {
			for _, kv := range kvs1 {
				if !yield(kv.K, kv.V) {
					return
				}
			}
		}

		assert.Equal(t, kvs1, iterkit.Collect2KV(iterkit.Merge2(itr1)))
	})

	s.Test("multi", func(t *testcase.T) {
		kvs1 := random.Slice(5, func() iterkit.KV[string, int] {
			return iterkit.KV[string, int]{
				K: t.Random.String(),
				V: t.Random.Int(),
			}
		})
		var itr1 iter.Seq2[string, int] = func(yield func(string, int) bool) {
			for _, kv := range kvs1 {
				if !yield(kv.K, kv.V) {
					return
				}
			}
		}
		exp := append(append([]iterkit.KV[string, int]{}, kvs1...), kvs1...)
		got := iterkit.Collect2KV(iterkit.Merge2(itr1, itr1))
		assert.Equal(t, exp, got)
	})
}

func TestMergeE(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty", func(t *testcase.T) {
		itr := iterkit.MergeE[int]()
		vs, err := iterkit.CollectE(itr)
		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	s.Test("single", func(t *testcase.T) {
		kvs1 := random.Slice(5, func() iterkit.KV[string, error] {
			return iterkit.KV[string, error]{
				K: t.Random.String(),
				V: t.Random.Error(),
			}
		})
		var itr1 iter.Seq2[string, error] = func(yield func(string, error) bool) {
			for _, kv := range kvs1 {
				if !yield(kv.K, kv.V) {
					return
				}
			}
		}

		assert.Equal(t, kvs1, iterkit.Collect2KV(iterkit.MergeE(itr1)))
	})

	s.Test("multi", func(t *testcase.T) {
		kvs1 := random.Slice(5, func() iterkit.KV[string, error] {
			return iterkit.KV[string, error]{
				K: t.Random.String(),
				V: t.Random.Error(),
			}
		})
		var itr1 iter.Seq2[string, error] = func(yield func(string, error) bool) {
			for _, kv := range kvs1 {
				if !yield(kv.K, kv.V) {
					return
				}
			}
		}
		exp := append(append([]iterkit.KV[string, error]{}, kvs1...), kvs1...)
		got := iterkit.Collect2KV(iterkit.MergeE(itr1, itr1))
		assert.Equal(t, exp, got)
	})
}

func ExampleCharRangeE() {
	for char, err := range iterkit.CharRangeE('A', 'Z') {
		_ = err // nil
		// prints characters between A and Z
		// A, B, C, D... Z
		fmt.Println(string(char))
	}
}

func TestCharRange_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	vs, err := iterkit.CollectE(iterkit.CharRangeE('A', 'C'))
	assert.NoError(it, err)
	assert.Equal(it, []rune{'A', 'B', 'C'}, vs)

	vs, err = iterkit.CollectE(iterkit.CharRangeE('a', 'c'))
	assert.NoError(it, err)
	assert.Equal(it, []rune{'a', 'b', 'c'}, vs)

	vs, err = iterkit.CollectE(iterkit.CharRangeE('1', '9'))
	assert.NoError(it, err)
	assert.Equal(it, []rune{'1', '2', '3', '4', '5', '6', '7', '8', '9'}, vs)
}

func TestCharRange(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		min = let.Var(s, func(t *testcase.T) rune {
			chars := []rune{'A', 'B', 'C'}
			return t.Random.Pick(chars).(rune)
		})
		max = let.Var(s, func(t *testcase.T) rune {
			chars := []rune{'E', 'F', 'G'}
			return t.Random.Pick(chars).(rune)
		})
	)
	act := let.Var(s, func(t *testcase.T) iter.Seq2[rune, error] {
		return iterkit.CharRangeE(min.Get(t), max.Get(t))
	})

	s.Then("it returns an iterator that contains the defined character range from min to max", func(t *testcase.T) {
		actual, err := iterkit.CollectE(act.Get(t))
		assert.NoError(t, err)

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

		vs, err := iterkit.CollectE(act.Get(t))
		t.Must.NoError(err)
		t.Must.Equal([]rune{'A', 'B', 'C', 'D'}, vs)
	})
}

func ExampleCharRange() {
	for char := range iterkit.CharRange('A', 'Z') {
		// prints characters between A and Z
		// A, B, C, D... Z
		fmt.Println(string(char))
	}
}

func TestCharRange1_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	vs := iterkit.Collect(iterkit.CharRange('A', 'C'))
	assert.Equal(it, []rune{'A', 'B', 'C'}, vs)

	vs = iterkit.Collect(iterkit.CharRange('a', 'c'))
	assert.Equal(it, []rune{'a', 'b', 'c'}, vs)

	vs = iterkit.Collect(iterkit.CharRange('1', '9'))
	assert.Equal(it, []rune{'1', '2', '3', '4', '5', '6', '7', '8', '9'}, vs)
}

func TestCharRange1(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		min = let.Var(s, func(t *testcase.T) rune {
			chars := []rune{'A', 'B', 'C'}
			return t.Random.Pick(chars).(rune)
		})
		max = let.Var(s, func(t *testcase.T) rune {
			chars := []rune{'E', 'F', 'G'}
			return t.Random.Pick(chars).(rune)
		})
	)
	subject := let.Var(s, func(t *testcase.T) iter.Seq[rune] {
		return iterkit.CharRange(min.Get(t), max.Get(t))
	})

	s.Then("it returns an iterator that contains the defined character range from min to max", func(t *testcase.T) {
		actual := iterkit.Collect(subject.Get(t))

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
		t.Must.Equal([]rune{'A', 'B', 'C', 'D'}, vs)
	})
}

func TestChar_implementsIterator(t *testing.T) {
	iterkitcontract.IterSeq[rune](func(tb testing.TB) iter.Seq[rune] {
		t := testcase.ToT(&tb)
		minChars := []rune{'A', 'B', 'C'}
		min := t.Random.Pick(minChars).(rune)
		maxChars := []rune{'E', 'F', 'G'}
		max := t.Random.Pick(maxChars).(rune)
		return iterkit.CharRange(min, max)
	}).Test(t)
}

func ExampleIntRangeE() {
	for n, _ := range iterkit.IntRangeE(1, 9) {
		// prints characters between 1 and 9
		// 1, 2, 3, 4, 5, 6, 7, 8, 9
		fmt.Println(n)
	}
}

func TestIntRangeE(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		begin = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(3, 7)
		})
		end = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(8, 13)
		})
	)
	act := let.Var(s, func(t *testcase.T) iter.Seq2[int, error] {
		return iterkit.IntRangeE(begin.Get(t), end.Get(t))
	})

	s.Then("it returns an iterator that contains the defined numeric range from min to max", func(t *testcase.T) {
		actual, err := iterkit.CollectE(act.Get(t))
		assert.NoError(t, err)

		var expected []int
		for i := begin.Get(t); i <= end.Get(t); i++ {
			expected = append(expected, i)
		}

		t.Must.NotEmpty(expected)
		t.Must.Equal(expected, actual)
	})

	s.Test("smoke", func(t *testcase.T) {
		vs, err := iterkit.CollectE(iterkit.IntRangeE(1, 9))
		t.Must.NoError(err)
		t.Must.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}, vs)

		vs, err = iterkit.CollectE(iterkit.IntRangeE(4, 7))
		t.Must.NoError(err)
		t.Must.Equal([]int{4, 5, 6, 7}, vs)

		vs, err = iterkit.CollectE(iterkit.IntRangeE(5, 1))
		t.Must.NoError(err)
		t.Must.NotNil(vs)
		t.Must.Equal([]int{}, vs)
	})

	s.Context("it behaves like a iter.Seq2[int, error]",
		iterkitcontract.IterSeq2(func(t testing.TB) iter.Seq2[int, error] {
			return iterkit.IntRangeE(1, 7)
		}).Spec)
}

func ExampleIntRange() {
	for n := range iterkit.IntRange(1, 9) {
		// prints characters between 1 and 9
		// 1, 2, 3, 4, 5, 6, 7, 8, 9
		fmt.Println(n)
	}
}

func TestIntRange(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		begin = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(3, 7)
		})
		end = let.Var(s, func(t *testcase.T) int {
			return t.Random.IntB(8, 13)
		})
	)
	subject := let.Var(s, func(t *testcase.T) iter.Seq[int] {
		return iterkit.IntRange(begin.Get(t), end.Get(t))
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

	s.Test("smoke", func(t *testcase.T) {
		vs := iterkit.Collect(iterkit.IntRange(1, 9))
		t.Must.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}, vs)

		vs = iterkit.Collect(iterkit.IntRange(4, 7))
		t.Must.Equal([]int{4, 5, 6, 7}, vs)

		vs = iterkit.Collect(iterkit.IntRange(5, 1))
		t.Must.Equal([]int{}, vs)
	})

	s.Context("it behaves like a iter.Seq[int]", iterkitcontract.IterSeq(func(t testing.TB) iter.Seq[int] {
		return iterkit.IntRange(3, 7)
	}).Spec)
}

func TestInt_implementsIterator(t *testing.T) {
	iterkitcontract.IterSeq[int](func(tb testing.TB) iter.Seq[int] {
		t := testcase.ToT(&tb)
		min := t.Random.IntB(3, 7)
		max := t.Random.IntB(8, 13)
		return iterkit.IntRange(min, max)
	}).Test(t)
}

func Test_spikeIterPull(t *testing.T) {
	itr := func(yield func(int) bool) {
		t.Log("iter func called (start)")
		defer t.Log("iter func exiting (stop)")
		if !yield(1) {
			t.Log("stop called at #1")
			return
		}
		t.Log("after #1")
		if !yield(2) {
			t.Log("stop called at #2")
			return
		}
		t.Log("after #2")
		if !yield(3) {
			t.Log("stop called at #3")
			return
		}
		t.Log("after #3")
	}

	next, stop := iter.Pull(itr)

	t.Log("before #1 next")
	_, _ = next()
	t.Log("after #1 next")

	t.Log("before #2 next")
	_, _ = next()
	t.Log("after #2 next")

	t.Log("before #3 next")
	_, _ = next()
	t.Log("after #3 next")

	t.Log("before stop")
	stop()
	t.Log("after stop")

}

func ExampleCollectE() {
	var itr iter.Seq2[int, error] = func(yield func(int, error) bool) {
		for i := 0; i < 42; i++ {
			if !yield(i, nil) {
				return
			}
		}
	}

	vs, err := iterkit.CollectE(itr)
	_, _ = vs, err
}
func TestCollectE(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		iterator = let.Var(s, func(t *testcase.T) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				for _, v := range values.Get(t) {
					if !yield(v, nil) {
						return
					}
				}
			}
		})
	)
	act := let.Act2(func(t *testcase.T) ([]int, error) {
		return iterkit.CollectE(iterator.Get(t))
	})

	s.Then("it should collect the values without an issue", func(t *testcase.T) {
		vs, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, values.Get(t), vs)
	})

	s.When("the error func returns an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		iterator.Let(s, func(t *testcase.T) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				if !yield(42, nil) {
					return
				}
				_ = yield(0, expErr.Get(t))
			}
		})

		s.Then("we expect to get back this error", func(t *testcase.T) {
			_, err := act(t)
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq2[int, error] {
			return iterkit.Empty2[int, error]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs, err := act(t)
			assert.NoError(t, err)
			assert.Empty(t, vs)
		})
	})

	s.When(`iterator is nil`, func(s *testcase.Spec) {
		iterator.LetValue(s, nil)

		s.Then(`no values returned`, func(t *testcase.T) {
			vs, err := act(t)
			assert.NoError(t, err)
			assert.Empty(t, vs)
		})
	})
}

func ExampleOnSeqEValue() {
	var (
		input  iter.Seq2[int, error]
		output iter.Seq2[string, error]
	)

	output = iterkit.OnSeqEValue(input, func(itr iter.Seq[int]) iter.Seq[string] {
		// we receive an iterator without the error second value
		// we do our iterator manipulation like it doesn't have an error
		// then we return it back
		itr = iterkit.Map(itr, func(v int) int { return v * 3 })
		itr = iterkit.Filter(itr, func(i int) bool { return i%2 == 0 })
		return iterkit.Map(itr, strconv.Itoa)
	})

	// the returned iter have the pipeline applied,
	// but the elements still contain the potential error value in case something went wrong.
	_ = output
}

func ExampleAsSeqE() {
	seq1Iter := iterkit.FromSlice([]int{1, 2, 3})
	errIter := iterkit.AsSeqE(seq1Iter)
	for v, err := range errIter {
		if err != nil {
			// will be always nil for the []int slice
		}
		_ = v // 1, 2, 3...
	}
}

func TestAsSeqE_iterSeq(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.FromSlice(values.Get(t))
		})
	)
	act := let.Act(func(t *testcase.T) iterkit.SeqE[int] {
		return iterkit.AsSeqE(itr.Get(t))
	})

	s.Then("it turns the iter.Seq[T] into a iter.Seq2[T, error] while having all the values yielded", func(t *testcase.T) {
		vs, err := iterkit.CollectE(act(t))
		assert.NoError(t, err)
		assert.Equal(t, vs, values.Get(t))
	})

	s.Then("iterating will not yield any error", func(t *testcase.T) {
		vs, err := iterkit.CollectE(act(t))
		assert.NoError(t, err)
		assert.Equal(t, vs, values.Get(t))
	})
}

func ExampleSplitSeqE() {
	var sourceSeqE iter.Seq2[int, error]

	i, errFunc := iterkit.SplitSeqE(sourceSeqE)
	for v := range i {
		fmt.Println(v)
	}
	if err := errFunc(); err != nil {
		fmt.Println(err.Error())
	}
}

func TestSplitSeqE(t *testing.T) {
	s := testcase.NewSpec(t)

	type E struct {
		Val int
		Err error
	}

	var (
		elements = let.Var(s, func(t *testcase.T) []E {
			return random.Slice(t.Random.IntBetween(3, 7), func() E {
				return E{Val: t.Random.Int()}
			})
		})
		valuesGet = func(t *testcase.T) []int {
			es := elements.Get(t)
			es = slicekit.Filter(es, func(v E) bool {
				return v.Err == nil // only non error values
			})
			eToVal := func(e E) int { return e.Val }
			return slicekit.Map(es, eToVal)
		}
		errIter = let.Var(s, func(t *testcase.T) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				for _, e := range elements.Get(t) {
					if !yield(e.Val, e.Err) {
						return
					}
				}
			}
		})
	)
	act := let.Act2(func(t *testcase.T) (iter.Seq[int], func() error) {
		return iterkit.SplitSeqE(errIter.Get(t))
	})

	s.Then("values can be collected", func(t *testcase.T) {
		itr, eFunc := act(t)
		vs := iterkit.Collect(itr)
		assert.Equal(t, vs, valuesGet(t))
		assert.NotNil(t, eFunc)
		assert.NoError(t, eFunc())
	})

	s.When("one of the iteration yield returns with an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		elements.Let(s, func(t *testcase.T) []E {
			es := elements.Super(t)
			slicekit.Insert(&es, t.Random.IntN(len(es)), E{Err: expErr.Get(t)})
			return es
		})

		s.Then("the error yielded back", func(t *testcase.T) {
			itr, eFunc := act(t)
			_ = iterkit.Collect(itr)
			assert.NotNil(t, eFunc)
			assert.ErrorIs(t, expErr.Get(t), eFunc())
		})

		s.Then("values are still collected", func(t *testcase.T) {
			i, _ := act(t)
			vs := iterkit.Collect(i)
			assert.Equal(t, vs, valuesGet(t))
		})

		s.And("if multiple elements has error", func(s *testcase.Spec) {
			othErr := let.Error(s)

			elements.Let(s, func(t *testcase.T) []E {
				es := elements.Super(t)
				slicekit.Insert(&es, t.Random.IntN(len(es)), E{Err: othErr.Get(t)})
				return es
			})

			s.Then("both error is propagated back", func(t *testcase.T) {
				itr, eFunc := act(t)
				_ = iterkit.Collect(itr)
				assert.NotNil(t, eFunc)
				assert.ErrorIs(t, eFunc(), expErr.Get(t))
				assert.ErrorIs(t, eFunc(), othErr.Get(t))
			})
		})

		s.Test("race", func(t *testcase.T) {
			itr, errFunc := act(t)

			testcase.Race(func() {
				for v := range itr {
					_ = v
				}
			}, func() {
				for range elements.Get(t) {
					_ = errFunc()
				}
			})
		})
	})
}

func TestOnSeqEValue(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	type Value struct {
		N int
		E error
	}

	var (
		itrValues = let.Var(s, func(t *testcase.T) []Value {
			return random.Slice(t.Random.IntBetween(3, 7), func() Value {
				return Value{N: t.Random.Int()}
			})
		})

		itr = let.Var(s, func(t *testcase.T) iter.Seq2[int, error] {
			return func(yield func(int, error) bool) {
				for _, v := range itrValues.Get(t) {
					if !yield(v.N, v.E) {
						return
					}
				}
			}
		})

		pipeline = let.Var(s, func(t *testcase.T) func(i iter.Seq[int]) iter.Seq[string] {
			return func(i iter.Seq[int]) iter.Seq[string] {
				return iterkit.Map(i, strconv.Itoa)
			}
		})
	)
	act := let.Act(func(t *testcase.T) iter.Seq2[string, error] {
		return iterkit.OnSeqEValue(itr.Get(t), pipeline.Get(t))
	})

	s.Then("we expect that iteration has the pipeline applied to the value", func(t *testcase.T) {
		itr := act(t)

		exp := slicekit.Map(itrValues.Get(t), func(v Value) string {
			return strconv.Itoa(v.N)
		})

		vs, err := iterkit.CollectE(itr)
		assert.NoError(t, err)
		assert.Equal(t, exp, vs)
	})

	s.When("iterator operation used that requires multiple event triggering", func(s *testcase.Spec) {

	})
}

func TestOnSeqEValue_batch(tt *testing.T) {
	t := testcase.NewT(tt)
	_ = t
}

func TestOnce(tt *testing.T) {
	t := testcase.NewT(tt)
	vs := random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
	itr := iterkit.FromSlice(vs)

	t.Log("given we have an iterator that can be iterated multiple times")
	t.Random.Repeat(3, 7, func() {
		assert.Equal(t, vs, iterkit.Collect(itr))
	})

	t.Log("but with iterkit.Once it will only iterate once")
	itrOnce := iterkit.Once(itr)
	assert.Equal(t, vs, iterkit.Collect(itrOnce))
	assert.Empty(t, iterkit.Collect(itrOnce))

	tt.Run("race", func(t *testing.T) {
		sub := iterkit.Once(itr)
		testcase.Race(func() {
			iterkit.Collect(sub)
		}, func() {
			iterkit.Collect(sub)
		})
	})
}

func TestOnce2(tt *testing.T) {
	t := testcase.NewT(tt)
	kvs := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, int] {
		return iterkit.KV[string, int]{
			K: t.Random.String(),
			V: t.Random.Int(),
		}
	})
	var itr iter.Seq2[string, int] = func(yield func(string, int) bool) {
		for _, e := range kvs {
			if !yield(e.K, e.V) {
				return
			}
		}
	}

	t.Log("given we have an iterator that can be iterated multiple times")
	t.Random.Repeat(3, 7, func() {
		assert.Equal(t, kvs, iterkit.Collect2KV(itr))
	})

	t.Log("but with iterkit.Once it will only iterate once")
	itrOnce := iterkit.Once2(itr)
	assert.Equal(t, kvs, iterkit.Collect2KV(itrOnce))
	assert.Empty(t, iterkit.Collect2KV(itrOnce))

	tt.Run("race", func(t *testing.T) {
		sub := iterkit.Once2(itr)
		blk := func() {
			for k, v := range sub {
				_, _ = k, v
				runtime.Gosched()
			}
		}
		testcase.Race(blk, blk, blk)
	})
}

func TestOnceE(tt *testing.T) {
	t := testcase.NewT(tt)
	kvs := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, error] {
		return iterkit.KV[string, error]{
			K: t.Random.String(),
			V: t.Random.Error(),
		}
	})
	var itr iter.Seq2[string, error] = func(yield func(string, error) bool) {
		for _, e := range kvs {
			if !yield(e.K, e.V) {
				return
			}
		}
	}

	t.Log("given we have an iterator that can be iterated multiple times")
	t.Random.Repeat(3, 7, func() {
		assert.Equal(t, kvs, iterkit.Collect2KV(itr))
	})

	t.Log("but with iterkit.Once it will only iterate once")
	itrOnce := iterkit.OnceE(itr)
	assert.Equal(t, kvs, iterkit.Collect2KV(itrOnce))
	assert.Empty(t, iterkit.Collect2KV(itrOnce))

	tt.Run("race", func(t *testing.T) {
		sub := iterkit.OnceE(itr)
		blk := func() {
			for k, v := range sub {
				_, _ = k, v
				runtime.Gosched()
			}
		}
		testcase.Race(blk, blk, blk)
	})
}

func TestFromPull_smoke(tt *testing.T) {
	t := testcase.NewT(tt)
	vs := random.Slice(5, t.Random.Int)
	itr := iterkit.FromSlice(vs)
	fromPullIter := iterkit.FromPull(iter.Pull(itr))
	got := iterkit.Collect(fromPullIter)
	assert.Equal(t, vs, got)
}

func TestFromPull2_smoke(tt *testing.T) {
	t := testcase.NewT(tt)
	kvs := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, int] {
		return iterkit.KV[string, int]{
			K: t.Random.String(),
			V: t.Random.Int(),
		}
	})
	var stopped bool
	var itr iter.Seq2[string, int] = func(yield func(string, int) bool) {
		for _, e := range kvs {
			if !yield(e.K, e.V) {
				return
			}
		}
		stopped = true
	}
	fromPullIter := iterkit.FromPull2(iter.Pull2(itr))
	got := iterkit.Collect2KV(fromPullIter)
	assert.Equal(t, kvs, got)
	assert.True(t, stopped)
}

func TestFromPullE(tt *testing.T) {
	s := testcase.NewSpec(tt)

	s.Test("smoke", func(t *testcase.T) {
		kvs := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, error] {
			return iterkit.KV[string, error]{K: t.Random.String()}
		})
		var stopped bool
		var itr iter.Seq2[string, error] = func(yield func(string, error) bool) {
			for _, e := range kvs {
				if !yield(e.K, e.V) {
					return
				}
			}
			stopped = true
		}
		fromPullIter := iterkit.FromPullE(iter.Pull2(itr))
		got := iterkit.Collect2KV(fromPullIter)
		assert.Equal(t, kvs, got)
		assert.True(t, stopped)
	})

	s.Test("err", func(t *testcase.T) {
		kvs := random.Slice(t.Random.IntBetween(3, 7), func() string {
			return t.Random.String()
		})
		expErr := t.Random.Error()
		var itr iter.Seq2[string, error] = func(yield func(string, error) bool) {
			errIndex := t.Random.IntN(len(kvs))
			for i, e := range kvs {
				if i == errIndex {
					if !yield("", expErr) {
						return
					}
				}
				if !yield(e, nil) {
					return
				}
			}
		}
		fromPullIter := iterkit.FromPullE(iter.Pull2(itr))
		got, err := iterkit.CollectE(fromPullIter)
		assert.Equal(t, kvs, got)
		assert.ErrorIs(t, err, expErr)
	})
}

func TestFromPullIter(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := random.Slice(5, t.Random.Int)
	errIter := iterkit.AsSeqE(iterkit.FromSlice(exp))
	pullIter := iterkit.ToPullIter(errIter)
	gotSeqE := iterkit.FromPullIter(pullIter)

	got, err := iterkit.CollectE(gotSeqE)
	assert.NoError(t, err)
	assert.Equal(t, exp, got)
}

func TestPullIter(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		exp := random.Slice(5, t.Random.Int)
		errIter := iterkit.AsSeqE(iterkit.FromSlice(exp))
		pullIter := iterkit.ToPullIter(errIter)
		fromPullSeqE := iterkit.FromPullIter(pullIter)

		got, err := iterkit.CollectE(fromPullSeqE)
		assert.NoError(t, err)
		assert.Equal(t, exp, got)
	})

	s.Test("error", func(t *testcase.T) {
		expErr := t.Random.Error()
		var errIter iterkit.SeqE[int] = func(yield func(int, error) bool) {
			if !yield(42, expErr) {
				return
			}
			yield(0, expErr)
		}

		pullIter := iterkit.ToPullIter(errIter)

		_, err := iterkit.CollectPullIter(pullIter)
		assert.ErrorIs(t, err, expErr)
	})
}

func TestCollect1Pull_smoke(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := random.Slice(5, t.Random.Int)
	errIter := iterkit.AsSeqE(iterkit.FromSlice(exp))
	pullIter := iterkit.ToPullIter(errIter)

	got, err := iterkit.CollectPullIter(pullIter)
	assert.NoError(t, err)
	assert.Equal(t, exp, got)
}

func ExampleFrom() {
	src := iterkit.From(func(yield func(int) bool) error {
		for v := range 42 {
			if !yield(v) {
				return nil
			}
		}
		return nil
	})

	_ = src
}

func TestFrom(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("empty", func(t *testcase.T) {
		vs, err := iterkit.CollectE(iterkit.From(func(yield func(int) bool) error {
			return nil
		}))

		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	s.Test("values + no error", func(t *testcase.T) {
		var expVS = random.Slice(t.Random.IntBetween(0, 100), t.Random.Int)
		vs, err := iterkit.CollectE(iterkit.From(func(yield func(int) bool) error {
			for _, v := range expVS {
				if !yield(v) {
					return nil
				}
			}
			return nil
		}))

		assert.NoError(t, err)
		assert.Equal(t, vs, expVS)
	})

	s.Test("error at the end", func(t *testcase.T) {
		var (
			expVS  = append([]int{}, random.Slice(t.Random.IntBetween(0, 100), t.Random.Int)...)
			expErr = t.Random.Error()
		)
		vs, err := iterkit.CollectE(iterkit.From(func(yield func(int) bool) error {
			for _, v := range expVS {
				if !yield(v) {
					return nil
				}
			}
			return expErr
		}))

		assert.ErrorIs(t, err, expErr)
		assert.Equal(t, vs, expVS)
	})

	s.Test("iteration interrupted", func(t *testcase.T) {
		var (
			length    = t.Random.IntBetween(30, 100)
			inVS      = random.Slice(length, t.Random.Int)
			notExpErr = t.Random.Error()
		)
		src := iterkit.From(func(yield func(int) bool) error {
			for _, v := range inVS {
				if !yield(v) {
					return nil
				}
			}
			return notExpErr
		})

		n := length / 2
		exp := inVS[0:n]

		vs, err := iterkit.CollectE(iterkit.Head2(src, n))
		assert.NoError(t, err)
		assert.Equal(t, exp, vs)
	})

	s.Test("not handled interuption handled", func(t *testcase.T) {
		src := iterkit.From(func(yield func(int) bool) error {
			for i := range 100 {
				_ = yield(i) // intentionally not handled
			}
			return nil
		})

		assert.NotPanic(t, func() {
			var i int = 50
			for range src {
				i--
				if i <= 0 {
					break
				}
			}
		})
	})
}

func ExampleReverse() {
	var i iter.Seq[int]
	i = iterkit.Reverse(i)
}

func TestReverse(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		input := random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)

		exp := slicekit.Clone(input)
		slices.Reverse(exp)

		i := iterkit.FromSlice(input)
		got := iterkit.Collect(iterkit.Reverse(i))

		assert.Equal(t, exp, got)
	})
}

func ExampleReverse2() {
	var i iter.Seq2[string, int]
	i = iterkit.Reverse2(i)
}

func TestReverse2(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		input := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, int] {
			return iterkit.KV[string, int]{
				K: t.Random.Domain(),
				V: t.Random.Int(),
			}
		})
		exp := slicekit.Clone(input)
		slices.Reverse(exp)

		i := iterkit.FromKV(input)
		got := iterkit.Collect2KV(iterkit.Reverse2(i))

		assert.Equal(t, exp, got)
	})
}

func ExampleReverseE() {
	var i iter.Seq2[string, error]
	i = iterkit.ReverseE(i)
}

func TestReverseE(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		input := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, error] {
			return iterkit.KV[string, error]{
				K: t.Random.Domain(),
				V: t.Random.Error(),
			}
		})
		exp := slicekit.Clone(input)
		slices.Reverse(exp)

		i := iterkit.FromKV(input)
		got := iterkit.Collect2KV(iterkit.ReverseE(i))

		assert.Equal(t, exp, got)
	})
}

func TestToK_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		input := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, error] {
			return iterkit.KV[string, error]{
				K: t.Random.Domain(),
				V: t.Random.Error(),
			}
		})

		exp := slicekit.Map(input, func(kv iterkit.KV[string, error]) string {
			return kv.K
		})

		got := iterkit.Collect(iterkit.ToK(iterkit.FromKV(input)))

		assert.Equal(t, exp, got)
	})
}

func TestToV_smoke(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		input := random.Slice(t.Random.IntBetween(3, 7), func() iterkit.KV[string, error] {
			return iterkit.KV[string, error]{
				K: t.Random.Domain(),
				V: t.Random.Error(),
			}
		})

		exp := slicekit.Map(input, func(kv iterkit.KV[string, error]) error {
			return kv.V
		})

		got := iterkit.Collect(iterkit.ToV(iterkit.FromKV(input)))

		assert.Equal(t, exp, got)
	})
}

func ExampleMap2ToSeq() {
	var (
		i2 iter.Seq2[string, int]
		i1 = iterkit.Map2ToSeq(i2, func(_ string, n int) int {
			return n
		})
	)
	_ = i1
}

func TestMap2ToSeq(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		vs := random.Slice(t.Random.IntBetween(0, 7), func() iterkit.KV[string, int] {
			return iterkit.KV[string, int]{
				K: t.Random.HexN(5),
				V: t.Random.Int(),
			}
		})

		exp := slicekit.Map(vs, func(v iterkit.KV[string, int]) int { return v.V })
		src := iterkit.FromKV(vs)
		out := iterkit.Map2ToSeq(src, func(s string, i int) int { return i })
		got := iterkit.Collect(out)

		if len(exp) == 0 {
			assert.Empty(t, got)
			return
		}

		assert.Equal(t, exp, got)
	})
}

func ExampleMapToSeq2() {
	var (
		i1 iter.Seq[int]
		i2 = iterkit.MapToSeq2(i1, func(n int) (string, int) {
			return strconv.Itoa(n), n
		})
	)
	_ = i2
}

func TestMapToSeq2(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		vs := random.Slice(t.Random.IntBetween(0, 7), func() int {
			return t.Random.Int()
		})

		exp := slicekit.Map(vs, func(n int) iterkit.KV[string, int] {
			return iterkit.KV[string, int]{
				K: strconv.Itoa(n),
				V: n,
			}
		})
		src := iterkit.FromSlice(vs)
		out := iterkit.MapToSeq2(src, func(n int) (string, int) {
			return strconv.Itoa(n), n
		})
		got := iterkit.Collect2KV(out)
		assert.Equal(t, exp, got)
	})
}
