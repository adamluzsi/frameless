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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/iterkit/iterkitcontract"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/tasker"
	. "go.llib.dev/frameless/spechelper/testent"

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

func ExampleLast() {
	itr := iterkit.IntRange(0, 10)

	n, ok := iterkit.Last(itr)
	_ = ok // true
	_ = n  // 10
}

func TestLast(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		var expected int = 42
		i := iterkit.Slice([]int{4, 2, expected})
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

func TestErrorf(t *testing.T) {
	i := iterkit.ErrorF[any]("%s", "hello world!")
	vs, err := iterkit.CollectErrIter(i)
	assert.Empty(t, vs)
	assert.Error(t, err)
	assert.Equal(t, "hello world!", err.Error())
}

var _ iter.Seq[string] = iterkit.Slice([]string{"A", "B", "C"})

func TestNewSlice_SliceGiven_SliceIterableAndValuesReturnedWithDecode(t *testing.T) {
	i := iterkit.Slice([]int{42, 4, 2})
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

func TestForEach(t *testing.T) {
	s := testcase.NewSpec(t)

	itr := testcase.Var[iter.Seq[int]]{ID: "frameless.Iterator"}
	fn := testcase.Var[func(int) error]{ID: "ForEach fn"}
	var subject = func(t *testcase.T) error {
		return iterkit.ForEach[int](itr.Get(t), fn.Get(t))
	}

	s.When(`iterator has values`, func(s *testcase.Spec) {
		elements := testcase.Let(s, func(t *testcase.T) []int { return []int{1, 2, 3} })
		itr.Let(s, func(t *testcase.T) iter.Seq[int] { return iterkit.Slice(elements.Get(t)) })

		s.And(`function block given`, func(s *testcase.Spec) {
			iteratedOnes := testcase.Let(s, func(t *testcase.T) map[int]struct{} { return make(map[int]struct{}) })
			fnErr := testcase.Let(s, func(t *testcase.T) error { return nil })

			fn.Let(s, func(t *testcase.T) func(int) error {
				return func(n int) error {
					iteratedOnes.Get(t)[n] = struct{}{}
					return fnErr.Get(t)
				}
			})

			s.Then(`it will iterate over all the elements without a problem`, func(t *testcase.T) {
				assert.Must(t).Nil(subject(t))

				iterated := iteratedOnes.Get(t)
				for _, n := range elements.Get(t) {
					_, ok := iterated[n]
					assert.True(t, ok, assert.Message(fmt.Sprintf(`expected that %d will be iterated by the function`, n)))
				}
			})

			s.And(`an error returned by the function`, func(s *testcase.Spec) {
				const expectedErr errorkit.Error = `boom`
				fnErr.Let(s, func(t *testcase.T) error { return expectedErr })

				s.Then(`it will return the error`, func(t *testcase.T) {
					t.Must.ErrorIs(expectedErr, subject(t))
				})

				s.Then(`it will cancel the iteration`, func(t *testcase.T) {
					_ = subject(t)
					t.Must.True(len(elements.Get(t)) > 1)
					t.Must.Equal(len(iteratedOnes.Get(t)), 1)
				})
			})

			s.And(`break error returned from the block`, func(s *testcase.Spec) {
				fnErr.Let(s, func(t *testcase.T) error { return iterkit.Break })

				s.Then(`it finish without an error`, func(t *testcase.T) {
					t.Must.Nil(subject(t))
				})

				s.Then(`it will cancel the iteration`, func(t *testcase.T) {
					_ = subject(t)
					t.Must.True(len(elements.Get(t)) > 1)
					t.Must.Equal(len(iteratedOnes.Get(t)), 1)
				})
			})
		})

	})

	s.Test("ForEach supports an optional ErrFunc(s)", func(t *testcase.T) {
		var (
			expErr1FromErrFunc = t.Random.Error()
			expErr2FromErrFunc = t.Random.Error()
			expErrFromForEach  = t.Random.Error()
		)
		errFunc1 := func() error {
			return expErr1FromErrFunc
		}
		errFunc2 := func() error {
			return expErr2FromErrFunc
		}
		forEach := func(i int) error {
			return expErrFromForEach
		}
		var got = iterkit.ForEach(iterkit.IntRange(1, 3), forEach, errFunc1, errFunc2)
		assert.ErrorIs(t, got, expErrFromForEach)
		assert.ErrorIs(t, got, expErr1FromErrFunc)
		assert.ErrorIs(t, got, expErr2FromErrFunc)
	})
}

func TestForEach_CompatbilityWithEmptyInterface(t *testing.T) {
	slice := []int{1, 2, 3, 4, 5}

	var found []int
	assert.Must(t).Nil(iterkit.ForEach[int](iterkit.Slice[int](slice), func(n int) error {
		found = append(found, n)
		return nil
	}))

	assert.Must(t).ContainExactly(slice, found)
}

func ExampleFilter() {
	var i iter.Seq[int]
	i = iterkit.Slice([]int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	i = iterkit.Filter[int](i, func(n int) bool { return n > 2 })

	for v := range i {
		fmt.Println(v)
	}
}

func TestFilter(t *testing.T) {
	t.Run("given the iterator has set of elements", func(t *testing.T) {
		originalInput := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		iterator := func() iter.Seq[int] { return iterkit.Slice[int](originalInput) }

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
				assert.Contain(t, originalInput, k)
				got = append(got, k)
				return true
			})

			assert.NotNil(t, i)
			kvs := iterkit.CollectKV(i)
			assert.ContainExactly(t, originalInput, got)
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

			assert.ContainExactly(t, exp, got)
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
		return iterkit.Filter[int](iterkit.Slice[int](values), logic)
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
	raw := iterkit.Slice([]int{1, 2, 42})

	_ = iterkit.Reduce[[]int](raw, nil, func(vs []int, v int) []int {
		return append(vs, v)
	})
}

func TestReduce(t *testing.T) {
	s := testcase.NewSpec(t)
	var (
		src = testcase.Let(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iterator = testcase.Let(s, func(t *testcase.T) iter.Seq[string] {
			return iterkit.Slice(src.Get(t))
		})
		initial = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = testcase.Let(s, func(t *testcase.T) func(int, string) int {
			return func(r int, v string) int {
				return r + len(v)
			}
		})
	)
	act := func(t *testcase.T) int {
		return iterkit.Reduce(iterator.Get(t), initial.Get(t), reducer.Get(t))
	}

	s.Then("it will execute the reducing", func(t *testcase.T) {
		r := act(t)
		t.Must.Equal(1+2+3+4+initial.Get(t), r)
	})
}

func ExampleReduceErr() {
	raw := iterkit.Slice([]string{"1", "2", "42"})

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
		src = testcase.Let(s, func(t *testcase.T) []string {
			return []string{
				t.Random.StringNC(1, random.CharsetAlpha()),
				t.Random.StringNC(2, random.CharsetAlpha()),
				t.Random.StringNC(3, random.CharsetAlpha()),
				t.Random.StringNC(4, random.CharsetAlpha()),
			}
		})
		iter = testcase.Let(s, func(t *testcase.T) iter.Seq[string] {
			return iterkit.Slice(src.Get(t))
		})
		initial = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.Int()
		})
		reducer = testcase.Let(s, func(t *testcase.T) func(int, string) (int, error) {
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
		t.Must.Nil(err)
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

func ExamplePaginate() {
	ctx := context.Background()
	fetchMoreFoo := func(ctx context.Context, offset int) ([]Foo, bool, error) {
		const limit = 10
		query := url.Values{}
		query.Set("limit", strconv.Itoa(limit))
		query.Set("offset", strconv.Itoa(offset))
		resp, err := http.Get("https://api.mydomain.com/v1/foos?" + query.Encode())
		if err != nil {
			return nil, false, err
		}

		var values []FooDTO
		defer resp.Body.Close()
		dec := json.NewDecoder(resp.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&values); err != nil {
			return nil, false, err
		}

		vs, err := dtokit.Map[[]Foo](ctx, values)
		if err != nil {
			return nil, false, err
		}
		probablyHasNextPage := len(vs) == limit
		return vs, probablyHasNextPage, nil
	}

	foos, release := iterkit.Paginate(ctx, fetchMoreFoo)
	_ = foos // foos can be called like any iterator,
	// and under the hood, the fetchMoreFoo function will be used dynamically,
	// to retrieve more values when the previously called values are already used up.
	_ = release
}

func TestPaginate(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ctx  = let.Context(s)
		more = testcase.Let[func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error)](s, nil)
	)
	act := func(t *testcase.T) (iter.Seq[Foo], func() error) {
		return iterkit.Paginate(ctx.Get(t), more.Get(t))
	}

	s.When("more function returns no more values", func(s *testcase.Spec) {
		more.Let(s, func(t *testcase.T) func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
			return func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
				return nil, false, nil
			}
		})

		s.Then("iteration finishes and we get the empty result", func(t *testcase.T) {
			vs, err := iterkit.CollectErr(act(t))
			assert.NoError(t, err)
			assert.Empty(t, vs)
		})
	})

	s.When("the more function return a last page", func(s *testcase.Spec) {
		value := testcase.LetValue(s, Foo{ID: "42", Foo: "foo", Bar: "bar", Baz: "baz"})
		more.Let(s, func(t *testcase.T) func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
			return func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
				return []Foo{value.Get(t)}, false, nil
			}
		})

		s.Then("we can collect that single value and return back", func(t *testcase.T) {
			vs, err := iterkit.CollectErr(act(t))
			assert.NoError(t, err)
			assert.Equal(t, []Foo{value.Get(t)}, vs)
		})
	})

	s.When("the more func says there is more, but yields an empty result set", func(s *testcase.Spec) {
		more.Let(s, func(t *testcase.T) func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
			return func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
				return nil, true, nil
			}
		})

		s.Then("it is treated as NoMore", func(t *testcase.T) {
			assert.Within(t, time.Second, func(ctx context.Context) {
				vs, err := iterkit.CollectErr(act(t))
				assert.NoError(t, err)
				assert.Empty(t, vs)
			})
		})
	})

	s.When("the more function returns back many pages", func(s *testcase.Spec) {
		values := testcase.LetValue[[]Foo](s, nil)

		more.Let(s, func(t *testcase.T) func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
			var (
				pages = t.Random.IntBetween(3, 5)
				cur   int
			)

			return func(ctx context.Context, offset int) ([]Foo, bool, error) {
				assert.Equal(t, len(values.Get(t)), offset,
					"expect that the offset represents the already consumed value count")

				defer func() { cur++ }()
				var vs []Foo
				t.Random.Repeat(3, 7, func() {
					vs = append(vs, rnd.Make(Foo{}).(Foo))
				})
				testcase.Append[Foo](t, values, vs...)
				hasMore := cur < pages
				return vs, hasMore, nil
			}
		})

		s.Then("all the values received back till more function had no more values to be retrieved", func(t *testcase.T) {
			vs, err := iterkit.CollectErr(act(t))
			assert.NoError(t, err)
			assert.Equal(t, vs, values.Get(t))
		})
	})

	s.When("more func encountered an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		more.Let(s, func(t *testcase.T) func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
			return func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
				return nil, false, expErr.Get(t)
			}
		})

		s.Then("the error is bubbled up to the iterator consumer", func(t *testcase.T) {
			_, err := iterkit.CollectErr(act(t))
			assert.ErrorIs(t, expErr.Get(t), err)
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

func TestHead(t *testing.T) {
	t.Run("less", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3})
		i = iterkit.Head(i, 2)
		vs := iterkit.Collect(i)
		assert.Equal(t, []int{1, 2}, vs)
	})
	t.Run("more", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3})
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

func TestTake(t *testing.T) {
	t.Run("NoElementsToTake", func(t *testing.T) {
		i := iterkit.Empty[int]()
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 5)
		assert.Empty(t, vs)
	})

	t.Run("EnoughElementsToTake", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3, 4, 5})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 3)
		assert.Equal(t, []int{1, 2, 3}, vs)

		rem := iterkit.TakeAll(next)
		assert.Equal(t, rem, []int{4, 5})
	})

	t.Run("MoreElementsToTakeThanAvailable", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 5)
		assert.Equal(t, []int{1, 2, 3}, vs)
		_, ok := next()
		assert.False(t, ok, "expected no next value")
	})

	t.Run("ZeroElementsToTake", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, 0)
		assert.Empty(t, vs)

		rem := iterkit.TakeAll(next)
		assert.Equal(t, rem, []int{1, 2, 3})
	})

	t.Run("NegativeNumberOfElementsToTake", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3})
		next, stop := iter.Pull(i)
		defer stop()
		vs := iterkit.Take(next, -5)
		assert.Empty(t, vs)
	})
}

func ExampleTakeAll() {
	i := iterkit.Slice([]int{1, 2, 3, 4, 5})
	next, stop := iter.Pull(i)
	defer stop()
	vs := iterkit.TakeAll(next)
	_ = vs // []int{1, 2, 3, 4, 5}
}

func TestTakeAll(t *testing.T) {
	i := iterkit.Slice([]int{1, 2, 3, 4, 5})
	next, stop := iter.Pull(i)
	defer stop()
	vs := iterkit.TakeAll(next)
	assert.Equal(t, []int{1, 2, 3, 4, 5}, vs)
}

func TestLimit_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterkit.Limit(iterkit.IntRange(2, 6), 3)
	vs := iterkit.Collect(subject)
	it.Must.Equal([]int{2, 3, 4}, vs)
}

func TestLimit(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		itr = testcase.Let[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.IntRange(1, iterLen)
		})
		n = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen-1)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[int] {
		return iterkit.Limit(itr.Get(t), n.Get(t))
	})

	s.Then("it will limit the returned results to the expected number", func(t *testcase.T) {
		vs := iterkit.Collect(subject.Get(t))
		t.Must.Equal(n.Get(t), len(vs))
	})

	s.Then("it will limited amount of value", func(t *testcase.T) {
		vs := iterkit.Collect(subject.Get(t))

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
			assert.Empty(t, iterkit.Collect(subject.Get(t)))
		})
	})

	s.When("the source iterator has less values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			vs := iterkit.Collect(subject.Get(t))
			t.Must.Equal(iterLen, len(vs))
		})
	})

	s.When("the source iterator has more values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen-1)

		s.Then("it will iterate only the limited number", func(t *testcase.T) {
			got := iterkit.Collect(subject.Get(t))
			t.Must.NotEmpty(got)

			total := iterkit.Collect(iterkit.IntRange(1, iterLen))
			t.Must.NotEmpty(got)

			t.Logf("%v < %v", got, total)
			t.Must.True(len(got) < len(total), "got count is less than total")
		})
	})
}

func TestLimit_implementsIterator(t *testing.T) {
	iterkitcontract.Iterator[int](func(tb testing.TB) iter.Seq[int] {
		t := testcase.ToT(&tb)
		return iterkit.Limit(
			iterkit.IntRange(1, 99),
			t.Random.IntB(1, 12),
		)
	}).Test(t)
}

var _ iter.Seq[any] = iterkit.SingleValue[any]("")

type ExampleStruct struct {
	Name string
}

var rnd = random.New(random.CryptoSeed{})

var RandomName = fmt.Sprintf("%d", rand.Int())

func TestNewSingleElement_StructGiven_StructReceivedWithDecode(t *testing.T) {

	var expected = ExampleStruct{Name: RandomName}

	i := iterkit.SingleValue[ExampleStruct](expected)

	actually, found := iterkit.First(i)
	assert.True(t, found)
	assert.Equal(t, expected, actually)
}

func TestNewSingleElement_StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat(t *testing.T) {

	var expected = ExampleStruct{Name: RandomName}

	i := iterkit.SingleValue(expected)
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
}

func TestOffset_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterkit.Offset(iterkit.IntRange(2, 6), 2)
	vs := iterkit.Collect(subject)
	it.Must.Equal([]int{4, 5, 6}, vs)
}

func TestOffset(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		makeIter = func() iter.Seq[int] {
			return iterkit.IntRange(1, iterLen)
		}
		itr = testcase.Let(s, func(t *testcase.T) iter.Seq[int] {
			return makeIter()
		})
		offset = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[int] {
		return iterkit.Offset(itr.Get(t), offset.Get(t))
	})

	s.Then("it will limit the results by skipping by the offset number", func(t *testcase.T) {
		got := iterkit.Collect(subject.Get(t))
		all := iterkit.Collect(makeIter())

		var exp = make([]int, 0)
		for i := offset.Get(t); i < len(all); i++ {
			exp = append(exp, all[i])
		}

		t.Must.Equal(exp, got)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			i := subject.Get(t)

			assert.Empty(t, iterkit.Collect(i))
		})
	})

	s.When("the source iterator has less values than the defined offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got := iterkit.Collect(subject.Get(t))

			t.Must.Empty(got)
		})
	})

	s.When("the source iterator has as many values as the offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			assert.Empty(t, iterkit.Collect(subject.Get(t)))
		})
	})

	s.When("the source iterator has more values than the defined offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen-1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got := iterkit.Collect(subject.Get(t))
			t.Must.NotEmpty(got)
			t.Must.Equal([]int{iterLen}, got)
		})
	})
}

func TestOffset_implementsIterator(t *testing.T) {
	iterkitcontract.Iterator[int](func(tb testing.TB) iter.Seq[int] {
		t := testcase.ToT(&tb)
		return iterkit.Offset(
			iterkit.IntRange(1, 99),
			t.Random.IntB(1, 12),
		)
	}).Test(t)
}

func ExampleEmpty() {
	_ = iterkit.Empty[any]()
}

func TestEmpty(t *testing.T) {
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
	var itr iter.Seq[int]

	ints := iterkit.Collect(itr)
	_ = ints
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
			return iterkit.Slice([]int{1, 2, 3})
		})

		s.Then(`it will collect the values`, func(t *testcase.T) {
			vs := act(t)
			t.Must.Equal([]int{1, 2, 3}, vs)
		})
	})
}

func ExampleCollectKV() {
	var itr iter.Seq2[string, int]

	ints := iterkit.CollectKV(itr)
	_ = ints
}

func TestCollectKV(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		iterator = let.Var[iter.Seq2[string, int]](s, nil)
	)
	act := func(t *testcase.T) []iterkit.KV[string, int] {
		return iterkit.CollectKV(iterator.Get(t))
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

func ExampleCollectPull() {
	var itr iter.Seq[int] = iterkit.IntRange(1, 10)
	vs := iterkit.CollectPull(iter.Pull(itr))
	_ = vs
}

func TestCollectPull(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice(values.Get(t))
		})
		stopCalled = let.Var(s, func(t *testcase.T) bool {
			return false
		})
		next, stop = let.Var2(s, func(t *testcase.T) (func() (int, bool), func()) {
			next, stop := iter.Pull(itr.Get(t))
			return next, func() {
				stopCalled.Set(t, true)
				stop()
			}
		})
	)
	act := func(t *testcase.T) []int {
		return iterkit.CollectPull(next.Get(t), stop.Get(t))
	}

	s.Then("values are collected", func(t *testcase.T) {
		vs := act(t)

		assert.Equal(t, values.Get(t), vs)
	})

	s.Then("stop is called", func(t *testcase.T) {
		_ = act(t)

		assert.True(t, stopCalled.Get(t))
	})

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		itr.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs := act(t)

			assert.Empty(t, vs)
		})

		s.Then("stop is called", func(t *testcase.T) {
			_ = act(t)

			assert.True(t, stopCalled.Get(t))
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

		s.Then("stop is called", func(t *testcase.T) {
			testcase.Sandbox(func() {
				act(t)
			})

			assert.True(t, stopCalled.Get(t))
		})
	})

	s.Test("supports collection without stop func", func(t *testcase.T) {
		defer stop.Get(t)()
		vs := iterkit.CollectPull(next.Get(t))
		assert.Equal(t, vs, values.Get(t))
	})
}

func TestCollectErr(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		iterator = let.Var(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice(values.Get(t))
		})
		errFunc = let.Var(s, func(t *testcase.T) func() error {
			return errorkit.NullErrFunc
		})
	)
	act := let.Act2(func(t *testcase.T) ([]int, error) {
		return iterkit.CollectErr(iterator.Get(t), errFunc.Get(t))
	})

	s.Then("it should collect the values without an issue", func(t *testcase.T) {
		vs, err := act(t)
		assert.NoError(t, err)
		assert.Equal(t, values.Get(t), vs)
	})

	s.When("the error func returns an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		errFunc.Let(s, func(t *testcase.T) func() error {
			return func() error {
				return expErr.Get(t)
			}
		})

		s.Then("we expect to get back this error", func(t *testcase.T) {
			_, err := act(t)
			assert.ErrorIs(t, err, expErr.Get(t))
		})
	})

	s.When("the error func is nil", func(s *testcase.Spec) {
		errFunc.LetValue(s, nil)

		s.Then("it is ignored and values just collected as usual", func(t *testcase.T) {
			vs, err := act(t)
			assert.NoError(t, err)
			assert.Equal(t, values.Get(t), vs)
		})
	})

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
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

func ExampleCount() {
	i := iterkit.Slice[int]([]int{1, 2, 3})
	total := iterkit.Count[int](i)
	_ = total // 3
}

func TestCount(t *testing.T) {
	i := iterkit.Slice[int]([]int{1, 2, 3})
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

func ExampleMap() {
	rawNumbers := iterkit.Slice([]string{"1", "2", "42"})
	numbers := iterkit.Map[int](rawNumbers, func(v string) int {
		return len(v)
	})
	_ = numbers
}

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	inputStream := testcase.Let(s, func(t *testcase.T) iter.Seq[string] {
		return iterkit.Slice([]string{`a`, `b`, `c`})
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

func ExampleMapErr() {
	rawNumbers := iterkit.Slice([]string{"1", "2", "42"})
	numbers, finish := iterkit.MapErr[int](rawNumbers, strconv.Atoi)
	_ = finish
	_ = numbers
}

func TestMapErr(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	var (
		inputStream = testcase.Let(s, func(t *testcase.T) iter.Seq[string] {
			return iterkit.Slice([]string{`a`, `b`, `c`})
		})
		transform = testcase.Var[func(string) (string, error)]{ID: `iterkit.MapTransformFunc`}
		errFuncs  = testcase.LetValue[[]func() error](s, nil)
	)
	act := func(t *testcase.T) (iter.Seq[string], func() error) {
		return iterkit.MapErr(inputStream.Get(t), transform.Get(t), errFuncs.Get(t)...)
	}

	s.When(`map used, the new iterator will have the changed values`, func(s *testcase.Spec) {
		transform.Let(s, func(t *testcase.T) func(string) (string, error) {
			return func(in string) (string, error) {
				return strings.ToUpper(in), nil
			}
		})

		s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
			vs, err := iterkit.CollectErr[string](act(t))
			t.Must.Nil(err)
			t.Must.Equal([]string{`A`, `B`, `C`}, vs)
		})

		s.And(`some error happen during mapping`, func(s *testcase.Spec) {
			expectedErr := let.Error(s)

			transform.Let(s, func(t *testcase.T) func(string) (string, error) {
				return func(string) (string, error) {
					return "", expectedErr.Get(t)
				}
			})

			s.Then(`error returned`, func(t *testcase.T) {
				vs, err := iterkit.CollectErr(act(t))
				assert.ErrorIs(t, err, expectedErr.Get(t))
				assert.Empty(t, vs)
			})

			s.Then("the error return is idempotent", func(t *testcase.T) {
				i, errp := act(t)
				assert.Empty(t, iterkit.Collect(i))

				t.Random.Repeat(3, 7, func() {
					assert.ErrorIs(t, errp(), expectedErr.Get(t),
						"expected that error is consistently returned")
				})
			})
		})

		s.And("the passed optional finish function(s) report an issue", func(s *testcase.Spec) {
			expErr := let.Error(s)

			errFuncs.Let(s, func(t *testcase.T) []func() error {
				return []func() error{func() error { return expErr.Get(t) }}
			})

			s.Then("completion contains the errors", func(t *testcase.T) {
				_, err := iterkit.CollectErr(act(t))

				assert.ErrorIs(t, err, expErr.Get(t))
			})
		})
	})

	s.Describe(`map used in a daisy chain style`, func(s *testcase.Spec) {
		interimErr := testcase.LetValue[error](s, nil)

		act := func(t *testcase.T) (iter.Seq[string], func() error) {
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

			i := inputStream.Get(t)
			var fin func() error
			i, fin = iterkit.MapErr(i, toUpper)
			i, fin = iterkit.MapErr(i, withIndex(), fin)
			return i, fin
		}

		s.Then(`it will execute all the map steps in the final iterator composition`, func(t *testcase.T) {
			values, err := iterkit.CollectErr(act(t))
			t.Must.Nil(err)
			t.Must.Equal([]string{`A0`, `B1`, `C2`}, values)
		})

		s.And("if an interim step has an error", func(s *testcase.Spec) {
			interimErr.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Then("error is propagated back in the returned finisher", func(t *testcase.T) {
				_, err := iterkit.CollectErr(act(t))

				assert.ErrorIs(t, err, interimErr.Get(t))
			})
		})
	})
}

func TestFirst_NextValueDecodable_TheFirstNextValueDecoded(t *testing.T) {

	var expected int = 42
	i := iterkit.Slice([]int{expected, 4, 2})

	actually, found := iterkit.First[int](i)
	assert.Equal(t, expected, actually)
	assert.True(t, found)
}

func TestFirst_WhenNextSayThereIsNoValueToBeDecoded_NotFoundReturned(t *testing.T) {
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

func TestChan(t *testing.T) {
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

			assert.ContainExactly(t, values.Get(t), got)
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

const defaultBatchSize = 64

func TestBatch(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = let.Var[[]int](s, func(t *testcase.T) []int {
			return random.Slice[int](t.Random.IntB(50, 200), func() int {
				return t.Random.Int()
			})
		})
		src = let.Var[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice(values.Get(t))
		})
		size = let.Var(s, func(t *testcase.T) int {
			return len(values.Get(t)) * 2
		})
	)
	act := func(t *testcase.T) iter.Seq[[]int] {
		return iterkit.Batch(src.Get(t), size.Get(t))
	}

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
			t.Must.ContainExactly(values.Get(t), got)
		})
	})

	s.When("size is an invalid value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			return random.Pick(t.Random,
				t.Random.IntB(1, 7)*-1, // negative value is not acceptable
				0,                      // zero int makes no sense for batch size
			)
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
			t.Must.ContainExactly(values.Get(t), got)
		})
	})
}

func ExampleBatchWithWaitLimit() {
	var slow iter.Seq[int] = func(yield func(int) bool) {
		for {
			if 5 < rnd.IntBetween(0, 10) { // random slowness
				time.Sleep(time.Second / 3)
			}

			if !yield(rnd.Int()) {
				return
			}
		}
	}

	batched := iterkit.BatchWithWaitLimit(slow, 7, time.Second)

	for vs := range batched {
		fmt.Printf("%#v\n", vs)
	}
}

func TestBatchWithWaitLimit(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = let.Var[[]int](s, func(t *testcase.T) []int {
			var vs []int
			for i, l := 0, t.Random.IntB(3, 7); i < l; i++ {
				vs = append(vs, t.Random.Int())
			}
			return vs
		})
		src = let.Var[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice(values.Get(t))
		})
		size = let.Var(s, func(t *testcase.T) int {
			t.Log("given that size is a valid positive value")
			return t.Random.IntBetween(1, len(values.Get(t)))
		})
		timeout = let.DurationBetween(s, time.Millisecond, 250*time.Millisecond)
	)
	act := func(t *testcase.T) iter.Seq[[]int] {
		return iterkit.BatchWithWaitLimit(src.Get(t), size.Get(t), timeout.Get(t))
	}

	ThenIterates := func(s *testcase.Spec) {
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
			for range act(t) {
				break
			}
		})
	}

	ThenIterates(s)

	s.Then("batch size corresponds to the size argument", func(t *testcase.T) {
		i := act(t)
		var got []int
		for vs := range i {
			t.Must.True(len(vs) <= size.Get(t))
			t.Must.NotEmpty(vs)
			got = append(got, vs...)
		}
		t.Must.NotEmpty(got)
		t.Must.ContainExactly(values.Get(t), got)
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
			t.Must.Contain(values.Get(t), vs)
		})
	})

	s.When("timeout is an invalid value", func(s *testcase.Spec) {
		timeout.Let(s, func(t *testcase.T) time.Duration {
			return time.Duration(t.Random.IntB(500, 1000)) * time.Microsecond * -1
		})

		s.Then("it will panic", func(t *testcase.T) {
			assert.Panic(t, func() { act(t) })
		})

	})
}

func TestError(t *testing.T) {
	expectedError := errors.New("Boom!")
	vs, err := iterkit.CollectErrIter(iterkit.Error[any](expectedError))
	assert.Empty(t, vs)
	assert.ErrorIs(t, err, expectedError)
}

func TestErrorF(t *testing.T) {
	expectedError := errors.New("Boom!")
	vs, err := iterkit.CollectErrIter(iterkit.ErrorF[any]("wrap:%w", expectedError))
	assert.Empty(t, vs)
	assert.ErrorIs(t, err, expectedError)
	assert.Contain(t, err.Error(), "wrap:"+expectedError.Error())
}

func ExampleScanner() {
	reader := strings.NewReader("a\nb\nc\nd")
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanLines)
	i, e := iterkit.BufioScanner[string](sc, nil)
	for text := range i {
		fmt.Println(text)
	}
	_ = e() // reports potential errors with the iteration
}

func ExampleScanner_Split() {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i, e := iterkit.BufioScanner[string](scanner, nil)
	for line := range i {
		fmt.Println(line)
	}
	fmt.Println(e())
}

func TestScanner_SingleLineGiven_EachLineFetched(t *testing.T) {

	readCloser := NewReadCloser(strings.NewReader("Hello, World!"))
	i, e := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)
	for v := range i {
		_ = v
	}
	_ = e()
}

func TestScanner_nilCloserGiven_EachLineFetched(t *testing.T) {
	readCloser := NewReadCloser(strings.NewReader("foo\nbar\nbaz"))
	i, e := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), nil)

	next, stop := iter.Pull(i)
	defer stop()

	v, ok := next()
	assert.True(t, ok)
	assert.Equal(t, "foo", v)
	v, ok = next()
	assert.True(t, ok)
	assert.Equal(t, "bar", v)
	v, ok = next()
	assert.True(t, ok)
	assert.Equal(t, "baz", v)
	_, ok = next()
	assert.False(t, ok)
	assert.NoError(t, e())
}

func TestScanner_ClosableIOGiven_OnCloseItIsClosed(t *testing.T) {

	readCloser := NewReadCloser(strings.NewReader(`Hy`))
	i, e := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)
	for _ = range i {
	}
	assert.NoError(t, e())
}

func TestScanner_MultipleLineGiven_EachLineFetched(t *testing.T) {

	readCloser := NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!"))
	i, e := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	next, stop := iter.Pull(i)
	defer stop()

	v, ok := next()
	assert.True(t, ok)
	assert.Equal(t, "Hello, World!", v)

	v, ok = next()
	assert.True(t, ok)
	assert.Equal(t, "How are you?", v)

	v, ok = next()
	assert.True(t, ok)
	assert.Equal(t, "Thanks I'm fine!", v)

	_, ok = next()
	assert.False(t, ok)

	assert.NoError(t, e())
}

func TestScanner_NilReaderGiven_ErrorReturned(t *testing.T) {
	readCloser := NewReadCloser(new(BrokenReader))
	i, e := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.Empty(t, iterkit.Collect(i))
	assert.ErrorIs(t, e(), io.ErrUnexpectedEOF)
}

func TestScanner_Split(t *testing.T) {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i, e := iterkit.BufioScanner[string](scanner, nil)

	lines, err := iterkit.CollectErr[string](i, e)
	assert.Must(t).Nil(err)
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
	assert.Must(t).ContainExactly(exp, got)
}

func ExampleSync2() {
	src := iterkit.IntRange(0, 100)
	itr, cancel := iterkit.Sync2(iterkit.ToErrIter(src))
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
		kvs := iterkit.CollectKV(itr)
		m.Lock()
		defer m.Unlock()
		got = append(got, kvs...)
	}

	testcase.Race(collect, collect, collect)
	assert.ContainExactly(t, exp, got)
}

func TestMerge(t *testing.T) {
	t.Run("EmptyIterators", func(t *testing.T) {
		iter := iterkit.Merge[int]()
		vs := iterkit.Collect(iter)
		assert.Empty(t, vs)
	})

	t.Run("SingleIterator", func(t *testing.T) {
		iter1 := iterkit.Slice([]int{1, 2, 3})
		mergedIter := iterkit.Merge(iter1)
		valuses := iterkit.Collect(mergedIter)
		assert.Equal(t, valuses, []int{1, 2, 3})
	})

	t.Run("MultipleIterators", func(t *testing.T) {
		iter1 := iterkit.Slice([]int{1, 2})
		iter2 := iterkit.Slice([]int{3, 4})
		iter3 := iterkit.Slice([]int{5, 6})
		vs := iterkit.Collect(iterkit.Merge(iter1, iter2, iter3))
		assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, vs)
	})

	t.Run("IteratorsWithError", func(t *testing.T) {
		iter1 := iterkit.Slice([]int{1, 2})
		expErr := rnd.Error()
		iter2, _ := iterkit.FromErrIter(iterkit.Error[int](expErr))
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
		vs := iterkit.CollectKV(itr)
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

		assert.Equal(t, kvs1, iterkit.CollectKV(iterkit.Merge2(itr1)))
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
		got := iterkit.CollectKV(iterkit.Merge2(itr1, itr1))
		assert.Equal(t, exp, got)
	})
}

func ExampleCharRange() {
	for char := range iterkit.CharRange('A', 'Z') {
		// prints characters between A and Z
		// A, B, C, D... Z
		fmt.Println(string(char))
	}
}

func TestCharRange_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	vs := iterkit.Collect(iterkit.CharRange('A', 'C'))
	it.Must.Equal([]rune{'A', 'B', 'C'}, vs)

	vs = iterkit.Collect(iterkit.CharRange('a', 'c'))
	it.Must.Equal([]rune{'a', 'b', 'c'}, vs)

	vs = iterkit.Collect(iterkit.CharRange('1', '9'))
	it.Must.Equal([]rune{'1', '2', '3', '4', '5', '6', '7', '8', '9'}, vs)
}

func TestCharRange(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		min = testcase.Let(s, func(t *testcase.T) rune {
			chars := []rune{'A', 'B', 'C'}
			return t.Random.Pick(chars).(rune)
		})
		max = testcase.Let(s, func(t *testcase.T) rune {
			chars := []rune{'E', 'F', 'G'}
			return t.Random.Pick(chars).(rune)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[rune] {
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
	iterkitcontract.Iterator[rune](func(tb testing.TB) iter.Seq[rune] {
		t := testcase.ToT(&tb)
		minChars := []rune{'A', 'B', 'C'}
		min := t.Random.Pick(minChars).(rune)
		maxChars := []rune{'E', 'F', 'G'}
		max := t.Random.Pick(maxChars).(rune)
		return iterkit.CharRange(min, max)
	}).Test(t)
}

func ExampleIntRange() {
	for n := range iterkit.IntRange(1, 9) {
		// prints characters between 1 and 9
		// 1, 2, 3, 4, 5, 6, 7, 8, 9
		fmt.Println(n)
	}
}

func TestIntRange_smoke(t *testing.T) {
	it := assert.MakeIt(t)

	vs := iterkit.Collect(iterkit.IntRange(1, 9))
	it.Must.Equal([]int{1, 2, 3, 4, 5, 6, 7, 8, 9}, vs)

	vs = iterkit.Collect(iterkit.IntRange(4, 7))
	it.Must.Equal([]int{4, 5, 6, 7}, vs)

	vs = iterkit.Collect(iterkit.IntRange(5, 1))
	it.Must.Equal([]int{}, vs)
}

func TestIntRange(t *testing.T) {
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
}

func TestInt_implementsIterator(t *testing.T) {
	iterkitcontract.Iterator[int](func(tb testing.TB) iter.Seq[int] {
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

func ExampleReverse() {
	itr := iterkit.IntRange(1, 3) // []int{1, 2, 3}
	itr = iterkit.Reverse(itr)    // []int{3, 2, 1}
	for range itr {
	}
}

func TestReverse(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		var in []int
		t.Random.Repeat(3, 7, func() {
			in = append(in, t.Random.IntBetween(1, 10))
		})
		og := slicekit.Clone(in)

		var exp []int
		for i := len(in) - 1; i >= 0; i-- {
			exp = append(exp, in[i])
		}

		var got []int

		for v := range iterkit.Reverse(iterkit.Slice(in)) {
			got = append(got, v)
		}

		assert.Equal(t, in, og, "it was not expected to alter the input slice")
		assert.Equal(t, got, exp, "expected that the iteration order is reversed")
	})

	s.Test("on nil iterator", func(t *testcase.T) {
		assert.Empty(t, iterkit.Collect(iterkit.Reverse[int](nil)))
	})

	s.Test("on empty iterator", func(t *testcase.T) {
		assert.Empty(t, iterkit.Collect(iterkit.Reverse(iterkit.Empty[int]())))
	})
}

func ExampleCollectErrIter() {
	var itr iter.Seq2[int, error] = func(yield func(int, error) bool) {
		for i := 0; i < 42; i++ {
			if !yield(i, nil) {
				return
			}
		}
	}

	vs, err := iterkit.CollectErrIter(itr)
	_, _ = vs, err
}
func TestCollectErrIter(t *testing.T) {
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
		return iterkit.CollectErrIter(iterator.Get(t))
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

func ExampleOnErrIterValue() {
	var (
		input  iter.Seq2[int, error]
		output iter.Seq2[string, error]
	)

	output = iterkit.OnErrIterValue(input, func(itr iter.Seq[int]) iter.Seq[string] {
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

func ExampleToErrIter() {
	seq1Iter := iterkit.Slice([]int{1, 2, 3})
	errIter := iterkit.ToErrIter(seq1Iter)
	for v, err := range errIter {
		if err != nil {
			// will be always nil for the []int slice
		}
		_ = v // 1, 2, 3...
	}
}

func TestToErrIter(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		values = let.Var(s, func(t *testcase.T) []int {
			return random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
		})
		itr = let.Var(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice(values.Get(t))
		})
		errFuncs = let.Var(s, func(t *testcase.T) []iterkit.ErrFunc {
			return nil
		})
	)
	act := let.Act(func(t *testcase.T) iterkit.ErrIter[int] {
		return iterkit.ToErrIter(itr.Get(t), errFuncs.Get(t)...)
	})

	s.Then("it turns the iter.Seq[T] into a iter.Seq2[T, error] while having all the values yielded", func(t *testcase.T) {
		vs, err := iterkit.CollectErrIter(act(t))
		assert.NoError(t, err)
		assert.Equal(t, vs, values.Get(t))
	})

	s.When("ErrFunc provided", func(s *testcase.Spec) {
		expErr := let.Var[error](s, nil)

		errFuncs.Let(s, func(t *testcase.T) []iterkit.ErrFunc {
			efs := errFuncs.Super(t)
			efs = append(efs, func() error { return expErr.Get(t) })
			return efs
		})

		s.And("it returns no error", func(s *testcase.Spec) {
			expErr.LetValue(s, nil)

			s.Then("iterating will not yield any error", func(t *testcase.T) {
				vs, err := iterkit.CollectErrIter(act(t))
				assert.NoError(t, err)
				assert.Equal(t, vs, values.Get(t))
			})

			s.And("but there is another err function passed as well that has an error", func(s *testcase.Spec) {
				othErr := let.Error(s)

				errFuncs.Let(s, func(t *testcase.T) []iterkit.ErrFunc {
					efs := errFuncs.Super(t)
					efs = append(efs, func() error { return othErr.Get(t) })
					return efs
				})

				s.Then("that error is also checked by the iterator", func(t *testcase.T) {
					_, err := iterkit.CollectErrIter(act(t))
					assert.ErrorIs(t, err, othErr.Get(t))
				})
			})
		})

		s.And("it yields an error", func(s *testcase.Spec) {
			expErr.Let(s, func(t *testcase.T) error {
				return t.Random.Error()
			})

			s.Then("the error is forwarded back", func(t *testcase.T) {
				_, err := iterkit.CollectErrIter(act(t))
				assert.ErrorIs(t, err, expErr.Get(t))
			})

			s.And("there is another err func that has an error as well", func(s *testcase.Spec) {
				othErr := let.Error(s)

				errFuncs.Let(s, func(t *testcase.T) []iterkit.ErrFunc {
					efs := errFuncs.Super(t)
					efs = append(efs, func() error { return othErr.Get(t) })
					return efs
				})

				s.Then("the first error is forwarded back", func(t *testcase.T) {
					_, err := iterkit.CollectErrIter(act(t))
					assert.ErrorIs(t, err, expErr.Get(t))
				})

				s.Then("the error from the other error function is returned", func(t *testcase.T) {
					_, err := iterkit.CollectErrIter(act(t))
					assert.ErrorIs(t, err, othErr.Get(t))
				})

				s.Then("we expect that both error yielded back as a combined one", func(t *testcase.T) {
					var gotErr error
					for _, err := range act(t) {
						if err != nil {
							gotErr = err
							break
						}
					}
					assert.ErrorIs(t, gotErr, expErr.Get(t))
					assert.ErrorIs(t, gotErr, othErr.Get(t))
				})
			})
		})
	})
}

func ExampleFromErrIter() {
	var sourceErrIter iter.Seq2[int, error]

	i, errFunc := iterkit.FromErrIter(sourceErrIter)
	for v := range i {
		fmt.Println(v)
	}
	if err := errFunc(); err != nil {
		fmt.Println(err.Error())
	}
}

func TestFromErrIter(t *testing.T) {
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
		return iterkit.FromErrIter(errIter.Get(t))
	})

	s.Then("values can be collected", func(t *testcase.T) {
		vs, err := iterkit.CollectErr(act(t))
		assert.NoError(t, err)
		assert.Equal(t, vs, valuesGet(t))
	})

	s.When("one of the iteration yield returns with an error", func(s *testcase.Spec) {
		expErr := let.Error(s)

		elements.Let(s, func(t *testcase.T) []E {
			es := elements.Super(t)
			slicekit.Insert(&es, t.Random.IntN(len(es)), E{Err: expErr.Get(t)})
			return es
		})

		s.Then("the error yielded back", func(t *testcase.T) {
			_, err := iterkit.CollectErr(act(t))
			assert.ErrorIs(t, err, expErr.Get(t))
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
				_, err := iterkit.CollectErr(act(t))
				assert.ErrorIs(t, err, expErr.Get(t))
				assert.ErrorIs(t, err, othErr.Get(t))
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

func TestOnErrIterValue(t *testing.T) {
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
		return iterkit.OnErrIterValue(itr.Get(t), pipeline.Get(t))
	})

	s.Then("we expect that iteration has the pipeline applied to the value", func(t *testcase.T) {
		itr := act(t)

		exp := slicekit.Map(itrValues.Get(t), func(v Value) string {
			return strconv.Itoa(v.N)
		})

		vs, err := iterkit.CollectErrIter(itr)
		assert.NoError(t, err)
		assert.Equal(t, exp, vs)
	})

	s.When("iterator operation used that requires multiple event triggering", func(s *testcase.Spec) {

	})
}

func TestOnErrIterValue_batch(tt *testing.T) {
	t := testcase.NewT(tt)
	_ = t
}

func TestNullErrFunc(t *testing.T) {
	assert.NotPanic(t, func() { _ = errorkit.NullErrFunc() })
	assert.NoError(t, errorkit.NullErrFunc())
}

func TestOnce(tt *testing.T) {
	t := testcase.NewT(tt)
	vs := random.Slice(t.Random.IntBetween(3, 7), t.Random.Int)
	itr := iterkit.Slice(vs)

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
		assert.Equal(t, kvs, iterkit.CollectKV(itr))
	})

	t.Log("but with iterkit.Once it will only iterate once")
	itrOnce := iterkit.Once2(itr)
	assert.Equal(t, kvs, iterkit.CollectKV(itrOnce))
	assert.Empty(t, iterkit.CollectKV(itrOnce))

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

func TestFromPull_smoke(tt *testing.T) {
	t := testcase.NewT(tt)
	vs := random.Slice(5, t.Random.Int)
	itr := iterkit.Slice(vs)
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
	var itr iter.Seq2[string, int] = func(yield func(string, int) bool) {
		for _, e := range kvs {
			if !yield(e.K, e.V) {
				return
			}
		}
	}
	fromPullIter := iterkit.FromPull2(iter.Pull2(itr))
	got := iterkit.CollectKV(fromPullIter)
	assert.Equal(t, kvs, got)
}

func TestFromPullIter(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := random.Slice(5, t.Random.Int)
	errIter := iterkit.ToErrIter(iterkit.Slice(exp))
	pullIter := iterkit.ToPullIter(errIter)
	gotErrIter := iterkit.FromPullIter(pullIter)

	got, err := iterkit.CollectErrIter(gotErrIter)
	assert.NoError(t, err)
	assert.Equal(t, exp, got)
}

func TestPullIter(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		exp := random.Slice(5, t.Random.Int)
		errIter := iterkit.ToErrIter(iterkit.Slice(exp))
		pullIter := iterkit.ToPullIter(errIter)
		fromPullErrIter := iterkit.FromPullIter(pullIter)

		got, err := iterkit.CollectErrIter(fromPullErrIter)
		assert.NoError(t, err)
		assert.Equal(t, exp, got)
	})

	s.Test("error", func(t *testcase.T) {
		expErr := t.Random.Error()
		var errIter iterkit.ErrIter[int] = func(yield func(int, error) bool) {
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

func TestCollectPullIter(tt *testing.T) {
	t := testcase.NewT(tt)

	exp := random.Slice(5, t.Random.Int)
	errIter := iterkit.ToErrIter(iterkit.Slice(exp))
	pullIter := iterkit.ToPullIter(errIter)

	got, err := iterkit.CollectPullIter(pullIter)
	assert.NoError(t, err)
	assert.Equal(t, exp, got)
}
