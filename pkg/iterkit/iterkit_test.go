package iterkit_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math/rand"
	"net/http"
	"net/url"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/dtokit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/iterkit/iterkitcontract"
	"go.llib.dev/frameless/pkg/iterkit/ranges"
	. "go.llib.dev/frameless/spechelper/testent"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/pp"
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

func (this *ReadCloser) Read(p []byte) (n int, err error) {
	return this.io.Read(p)
}

func (this *ReadCloser) Close() error {
	if this.IsClosed {
		return errors.New("already closed")
	}

	this.IsClosed = true
	return nil
}

type BrokenReader struct{}

func (b *BrokenReader) Read(p []byte) (n int, err error) { return 0, io.ErrUnexpectedEOF }

type x struct{ data string }

func TestLast_NextValueDecodable_TheLastNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	i := iterkit.Slice[int]([]int{4, 2, expected})
	actually, found := iterkit.Last[int](i)
	assert.True(t, found)
	assert.Equal(t, expected, actually)
}

func TestLast_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	_, found := iterkit.Last[Entity](iterkit.Empty[Entity]())
	assert.False(t, found)
}

func TestErrorf(t *testing.T) {
	i, r := iterkit.Errorf[any]("%s", "hello world!")
	assert.Empty(t, iterkit.Collect(i))

	err := r()
	assert.Error(t, err)
	assert.Equal(t, "hello world!", err.Error())
}

var _ iter.Seq[string] = iterkit.Slice([]string{"A", "B", "C"})

func TestNewSlice_SliceGiven_SliceIterableAndValuesReturnedWithDecode(t *testing.T) {
	t.Parallel()

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

		s.Test("if the iterator closed early on values are not retrieved any further", func(t *testcase.T) {
			i, r := act(t)
			assert.NoError(t, r())
			assert.Empty(t, iterkit.Collect(i))
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
			_, err := iterkit.Collect(act(t))
			assert.ErrorIs(t, expErr.Get(t), err)
		})
	})
}

func ExampleHead() {
	infStream := iterkit.Func[int](func() (v int, ok bool, err error) {
		return 42, true, nil
	})

	i := iterkit.Head(infStream, 3)

	vs, err := iterkit.Collect(i)
	_, _ = vs, err // []{42, 42, 42}, nil
}

func TestHead(t *testing.T) {
	t.Run("less", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3})
		i = iterkit.Head(i, 2)
		vs, err := iterkit.Collect(i)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2}, vs)
	})
	t.Run("more", func(t *testing.T) {
		i := iterkit.Slice([]int{1, 2, 3})
		i = iterkit.Head(i, 5)
		vs, err := iterkit.Collect(i)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, vs)
	})
	t.Run("closes", func(t *testing.T) {
		var (
			expErr  = rnd.Error()
			closedN int
		)

		stub := iterkit.Stub(iterkit.Slice([]int{1, 2, 3, 4, 5}))
		stub.StubClose = func() error {
			closedN++
			return expErr
		}

		i := iterkit.Head[int](stub, 3)

		vs, err := iterkit.Collect(i)
		assert.ErrorIs(t, expErr, err)
		assert.Equal(t, []int{1, 2, 3}, vs)
		assert.ErrorIs(t, expErr, i.Close())
		assert.Equal(t, 1, closedN,
			"expected that close only called once")
	})
	t.Run("err", func(t *testing.T) {
		expErr := rnd.Error()
		i := iterkit.Error[int](expErr)
		i = iterkit.Head(i, 42)
		assert.False(t, i.Next())
		assert.ErrorIs(t, expErr, i.Err())
		assert.NoError(t, i.Close())
	})
	t.Run("inf iterator", func(t *testing.T) {
		assert.Within(t, time.Second, func(ctx context.Context) {
			infStream := iterkit.Func[int](func() (v int, ok bool, err error) {
				if ctx.Err() != nil {
					return v, false, nil
				}
				return 42, true, nil
			})

			i := iterkit.Head(infStream, 3)

			vs, err := iterkit.Collect(i)
			assert.NoError(t, err)
			assert.Equal(t, []int{42, 42, 42}, vs)
		})
	})
}

func TestTake(t *testing.T) {
	t.Run("NoElementsToTake", func(t *testing.T) {
		iter := iterkit.Empty[int]()
		vs, err := iterkit.Take(iter, 5)
		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	t.Run("EnoughElementsToTake", func(t *testing.T) {
		iter := iterkit.Slice([]int{1, 2, 3, 4, 5})
		vs, err := iterkit.Take(iter, 3)
		assert.Equal(t, []int{1, 2, 3}, vs)
		assert.NoError(t, err)
		rem, err := iterkit.Collect(iter)
		assert.NoError(t, err)
		assert.Equal(t, rem, []int{4, 5})
	})

	t.Run("MoreElementsToTakeThanAvailable", func(t *testing.T) {
		iter := iterkit.Slice([]int{1, 2, 3})
		vs, err := iterkit.Take(iter, 5)
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, vs)
		assert.False(t, iter.Next())
	})

	t.Run("ZeroElementsToTake", func(t *testing.T) {
		iter := iterkit.Slice([]int{1, 2, 3})
		vs, err := iterkit.Take(iter, 0)
		assert.NoError(t, err)
		assert.Empty(t, vs)

		rem, err := iterkit.Collect(iter)
		assert.NoError(t, err)
		assert.Equal(t, rem, []int{1, 2, 3})
	})

	t.Run("NegativeNumberOfElementsToTake", func(t *testing.T) {
		iter := iterkit.Slice([]int{1, 2, 3})
		vs, err := iterkit.Take(iter, -5)
		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	t.Run("IteratorWithError", func(t *testing.T) {
		expErr := rnd.Error()
		iter := iterkit.Error[int](expErr)
		vs, err := iterkit.Take(iter, 5)
		assert.ErrorIs(t, err, expErr)
		assert.Empty(t, vs)
	})
}

func TestLimit_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterkit.Limit(ranges.Int(2, 6), 3)
	vs, err := iterkit.Collect(subject)
	it.Must.NoError(err)
	it.Must.Equal([]int{2, 3, 4}, vs)
}

func TestLimit(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		iter = testcase.Let[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return ranges.Int(1, iterLen)
		})
		n = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen-1)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[int] {
		return iterkit.Limit(iter.Get(t), n.Get(t))
	})

	s.Then("it will limit the returned results to the expected number", func(t *testcase.T) {
		vs, err := iterkit.Collect(subject.Get(t))
		t.Must.NoError(err)
		t.Must.Equal(n.Get(t), len(vs))
	})

	s.Then("it will limited amount of value", func(t *testcase.T) {
		vs, err := iterkit.Collect(subject.Get(t))
		t.Must.NoError(err)

		t.Log("n", n.Get(t))
		var exp []int
		for i := 0; i < n.Get(t); i++ {
			exp = append(exp, i+1)
		}

		t.Must.Equal(exp, vs)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		iter.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			iter := subject.Get(t)
			t.Must.False(iter.Next())
			t.Must.NoError(iter.Err())
			t.Must.NoError(iter.Close())
		})
	})

	s.When("the source iterator has less values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			vs, err := iterkit.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.Equal(iterLen, len(vs))
		})
	})

	s.When("the source iterator has more values than the limit number", func(s *testcase.Spec) {
		n.LetValue(s, iterLen-1)

		s.Then("it will iterate only the limited number", func(t *testcase.T) {
			got, err := iterkit.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.NotEmpty(got)

			total, err := iterkit.Collect(ranges.Int(1, iterLen))
			t.Must.NoError(err)
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
			ranges.Int(1, 99),
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
	t.Parallel()

	var expected = ExampleStruct{Name: RandomName}

	i := iterkit.SingleValue[ExampleStruct](expected)
	defer i.Close()

	actually, found, err := iterkit.First[ExampleStruct](i)
	assert.Must(t).Nil(err)
	assert.True(t, found)
	assert.Equal(t, expected, actually)
}

func TestNewSingleElement_StructGivenAndNextCalledMultipleTimes_NextOnlyReturnTrueOnceAndStayFalseAfterThat(t *testing.T) {
	t.Parallel()

	var expected = ExampleStruct{Name: RandomName}

	i := iterkit.SingleValue(&expected)
	defer i.Close()

	assert.True(t, i.Next())

	checkAmount := random.New(random.CryptoSeed{}).IntBetween(1, 100)
	for n := 0; n < checkAmount; n++ {
		assert.False(t, i.Next())
	}

}

func TestNewSingleElement_CloseCalled_DecodeWarnsAboutThis(t *testing.T) {
	t.Parallel()

	i := iterkit.SingleValue(&ExampleStruct{Name: RandomName})
	i.Close()
	assert.False(t, i.Next())
	assert.Must(t).Nil(i.Err())
}

func TestOffset_smoke(t *testing.T) {
	it := assert.MakeIt(t)
	subject := iterkit.Offset(ranges.Int(2, 6), 2)
	vs, err := iterkit.Collect(subject)
	it.Must.NoError(err)
	it.Must.Equal([]int{4, 5, 6}, vs)
}

func TestOffset(t *testing.T) {
	s := testcase.NewSpec(t)

	const iterLen = 10
	var (
		makeIter = func() iter.Seq[int] {
			return ranges.Int(1, iterLen)
		}
		iter = testcase.Let(s, func(t *testcase.T) iter.Seq[int] {
			return makeIter()
		})
		offset = testcase.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(3, iterLen)
		})
	)
	subject := testcase.Let(s, func(t *testcase.T) iter.Seq[int] {
		return iterkit.Offset(iter.Get(t), offset.Get(t))
	})

	s.Then("it will limit the results by skipping by the offset number", func(t *testcase.T) {
		got, err := iterkit.Collect(subject.Get(t))
		t.Must.NoError(err)

		all, err := iterkit.Collect(makeIter())
		t.Must.NoError(err)

		var exp = make([]int, 0)
		for i := offset.Get(t); i < len(all); i++ {
			exp = append(exp, all[i])
		}

		t.Must.Equal(exp, got)
	})

	s.When("the iterator is empty", func(s *testcase.Spec) {
		iter.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then("it will iterate over without an issue and returns no value", func(t *testcase.T) {
			iter := subject.Get(t)
			t.Must.False(iter.Next())
			t.Must.NoError(iter.Err())
			t.Must.NoError(iter.Close())
		})
	})

	s.When("the source iterator has less values than the defined offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen+1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got, err := iterkit.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.Empty(got)
		})
	})

	s.When("the source iterator has as many values as the offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got, err := iterkit.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.Empty(got)
		})
	})

	s.When("the source iterator has more values than the defined offset number", func(s *testcase.Spec) {
		offset.LetValue(s, iterLen-1)

		s.Then("it will collect the total amount of the iterator", func(t *testcase.T) {
			got, err := iterkit.Collect(subject.Get(t))
			t.Must.NoError(err)
			t.Must.NotEmpty(got)
			t.Must.Equal([]int{iterLen}, got)
		})
	})
}

func TestOffset_implementsIterator(t *testing.T) {
	iterkitcontract.Iterator[int](func(tb testing.TB) iter.Seq[int] {
		t := testcase.ToT(&tb)
		return iterkit.Offset(
			ranges.Int(1, 99),
			t.Random.IntB(1, 12),
		)
	}).Test(t)
}

func ExampleEmpty() {
	iterkit.Empty[any]()
}

func TestEmpty(suite *testing.T) {
	suite.Run("#Close", func(spec *testing.T) {

		spec.Run("when called once", func(t *testing.T) {
			t.Parallel()
			subject := iterkit.Empty[any]()
			assert.Must(t).Nil(subject.Close())
		})

		spec.Run("when called multiple", func(t *testing.T) {
			t.Parallel()

			subject := iterkit.Empty[any]()

			times := rand.Intn(42) + 1

			for i := 0; i < times; i++ {
				assert.Must(t).Nil(subject.Close())
			}
		})

	})

	suite.Run("#Next", func(spec *testing.T) {

		spec.Run("when called once", func(t *testing.T) {
			t.Parallel()

			subject := iterkit.Empty[any]()

			assert.False(t, subject.Next())
		})

		spec.Run("when called multiple", func(t *testing.T) {
			t.Parallel()

			subject := iterkit.Empty[any]()

			times := rand.Intn(42) + 1

			for i := 0; i < times; i++ {
				assert.False(t, subject.Next())
			}
		})

	})

	suite.Run("#Err", func(t *testing.T) {
		t.Parallel()

		assert.Must(t).Nil(iterkit.Empty[any]().Err())
	})

	suite.Run("#Value", func(t *testing.T) {
		t.Parallel()
		subject := iterkit.Empty[int]()
		assert.Equal(t, 0, subject.Value())
	})
}

func TestWithCallback(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	s.When(`no callback is defined`, func(s *testcase.Spec) {
		s.Then(`it will execute iterator calls like it is not even there`, func(t *testcase.T) {
			expected := []int{1, 2, 3}
			input := iterkit.Slice(expected)
			i := iterkit.WithCallback[int](input)

			actually, err := iterkit.Collect(i)
			assert.Must(t).Nil(err)
			assert.Equal(t, 3, len(actually))
			assert.Must(t).ContainExactly(expected, actually)
		})

		s.Then(`if actually no option is given, it returns the original iterator`, func(t *testcase.T) {
			expected := []int{1, 2, 3}
			input := iterkit.Slice(expected)
			i := iterkit.WithCallback[int](input)
			assert.Equal(t, input, i)
			actually, err := iterkit.Collect(i)
			assert.Must(t).Nil(err)
			assert.Equal(t, 3, len(actually))
			assert.Must(t).ContainExactly(expected, actually)
		})
	})

	s.When(`OnClose callback is given`, func(s *testcase.Spec) {
		s.Then(`the callback is called after the iterator.eClose`, func(t *testcase.T) {
			var closeHook []string

			m := iterkit.Stub[int](iterkit.Slice[int]([]int{1, 2, 3}))
			m.StubClose = func() error {
				closeHook = append(closeHook, `during`)
				return nil
			}

			callbackErr := random.New(random.CryptoSeed{}).Error()

			i := iterkit.WithCallback[int](m,
				iterkit.OnClose(func() error {
					closeHook = append(closeHook, `after`)
					return callbackErr
				}),
			)

			assert.Must(t).ErrorIs(callbackErr, i.Close())
			assert.Equal(t, 2, len(closeHook))
			assert.Equal(t, `during`, closeHook[0])
			assert.Equal(t, `after`, closeHook[1])
		})

		s.And(`error happen during closing in hook`, func(s *testcase.Spec) {
			s.And(`and the callback has no issue`, func(s *testcase.Spec) {
				s.Then(`error received`, func(t *testcase.T) {
					expectedErr := errors.New(`boom`)

					m := iterkit.Stub[int](iterkit.Slice[int]([]int{1, 2, 3}))
					m.StubClose = func() error { return expectedErr }
					i := iterkit.WithCallback[int](m,
						iterkit.OnClose(func() error {
							return nil
						}))

					assert.Equal(t, expectedErr, i.Close())
				})
			})
		})
	})
}

func TestCallbackOnClose(t *testing.T) {
	var closed bool
	expErr := random.New(random.CryptoSeed{}).Error()
	iter := iterkit.Slice([]int{1, 2, 3})
	iter = iterkit.WithCallback(iter, iterkit.OnClose(func() error {
		closed = true
		return expErr
	}))

	vs, err := iterkit.Collect(iter)
	assert.ErrorIs(t, err, expErr)
	assert.Equal(t, []int{1, 2, 3}, vs)
	assert.True(t, closed)
}

func ExampleIterator() {
	var iter iter.Seq[int]
	defer iter.Close()
	for iter.Next() {
		v := iter.Value()
		_ = v
	}
	if err := iter.Err(); err != nil {
		// handle error
	}
}

func TestCollect(t *testing.T) {
	s := testcase.NewSpec(t)
	s.NoSideEffect()

	var (
		iterator = testcase.Var[iter.Seq[int]]{ID: `iterator`}
		subject  = func(t *testcase.T) ([]int, error) {
			return iterkit.Collect(iterator.Get(t))
		}
	)

	s.When(`no elements in iterator`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Empty[int]()
		})

		s.Then(`no element appended to the slice`, func(t *testcase.T) {
			vs, err := subject(t)
			t.Must.Nil(err)
			t.Must.Empty(vs)
		})
	})

	s.When(`iterator has elements`, func(s *testcase.Spec) {
		iterator.Let(s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice([]int{1, 2, 3})
		})

		s.Then(`it will collect the values`, func(t *testcase.T) {
			vs, err := subject(t)
			t.Must.Nil(err)
			t.Must.Equal([]int{1, 2, 3}, vs)
		})
	})

	s.Describe(`iterator returns error during`, func(s *testcase.Spec) {
		const expectedErr errorkit.Error = "boom"

		s.Context(`Close`, func(s *testcase.Spec) {
			iterator.Let(s, func(t *testcase.T) iter.Seq[int] {
				i := iterkit.Stub[int](iterkit.Slice([]int{42, 43, 44}))
				i.StubClose = func() error { return expectedErr }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				_, err := subject(t)
				t.Must.Equal(err, expectedErr)
			})
		})

		s.Context(`Err`, func(s *testcase.Spec) {
			iterator.Let(s, func(t *testcase.T) iter.Seq[int] {
				i := iterkit.Stub[int](iterkit.Slice([]int{42, 43, 44}))
				i.StubErr = func() error { return expectedErr }
				return i
			})

			s.Then(`error forwarded to the caller`, func(t *testcase.T) {
				_, err := subject(t)
				t.Must.Equal(err, expectedErr)
			})
		})
	})
}

func TestCollect_emptySlice(t *testing.T) {
	T := 0
	slice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(T)), 0, 0).Interface()
	t.Logf(`%T`, slice)
	t.Logf(`%#v`, slice)
	vs, err := iterkit.Collect[int](iterkit.Slice[int]([]int{42}))
	assert.Must(t).Nil(err)
	assert.Equal(t, []int{42}, vs)
}

func TestCount_andCountTotalIterations_IteratorGiven_AllTheRecordsCounted(t *testing.T) {
	t.Parallel()

	i := iterkit.Slice[int]([]int{1, 2, 3})
	total, err := iterkit.Count[int](i)
	assert.Must(t).Nil(err)
	assert.Equal(t, 3, total)
}

func TestCount_errorOnCloseReturned(t *testing.T) {
	t.Parallel()

	s := iterkit.Slice[int]([]int{1, 2, 3})
	m := iterkit.Stub[int](s)

	expected := errors.New("boom")
	m.StubClose = func() error {
		return expected
	}

	_, err := iterkit.Count[int](m)
	assert.Equal(t, expected, err)
}

func ExampleMap() {
	rawNumbers := iterkit.Slice([]string{"1", "2", "42"})
	numbers := iterkit.Map[int](rawNumbers, strconv.Atoi)
	_ = numbers
}

func TestMap(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	inputStream := testcase.Let(s, func(t *testcase.T) iter.Seq[string] {
		return iterkit.Slice([]string{`a`, `b`, `c`})
	})
	transform := testcase.Var[func(string) (string, error)]{ID: `iterators.MapTransformFunc`}

	subject := func(t *testcase.T) iter.Seq[string] {
		return iterkit.Map(inputStream.Get(t), transform.Get(t))
	}

	s.When(`map used, the new iterator will have the changed values`, func(s *testcase.Spec) {
		transform.Let(s, func(t *testcase.T) func(string) (string, error) {
			return func(in string) (string, error) {
				return strings.ToUpper(in), nil
			}
		})

		s.Then(`the new iterator will return values with enhanced by the map step`, func(t *testcase.T) {
			vs, err := iterkit.Collect[string](subject(t))
			t.Must.Nil(err)
			t.Must.ContainExactly([]string{`A`, `B`, `C`}, vs)
		})

		s.And(`some error happen during mapping`, func(s *testcase.Spec) {
			expectedErr := errors.New(`boom`)
			transform.Let(s, func(t *testcase.T) func(string) (string, error) {
				return func(string) (string, error) {
					return "", expectedErr
				}
			})

			s.Then(`error returned`, func(t *testcase.T) {
				i := subject(t)
				t.Must.False(i.Next())
				t.Must.Equal(expectedErr, i.Err())
			})
		})

	})

	s.Describe(`map used in a daisy chain style`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) iter.Seq[string] {
			toUpper := func(s string) (string, error) {
				return strings.ToUpper(s), nil
			}

			withIndex := func() func(s string) (string, error) {
				var index int
				return func(s string) (string, error) {
					defer func() { index++ }()
					return fmt.Sprintf(`%s%d`, s, index), nil
				}
			}

			i := inputStream.Get(t)
			i = iterkit.Map(i, toUpper)
			i = iterkit.Map(i, withIndex())

			return i
		}

		s.Then(`it will execute all the map steps in the final iterator composition`, func(t *testcase.T) {
			values, err := iterkit.Collect(subject(t))
			t.Must.Nil(err)
			t.Must.ContainExactly([]string{`A0`, `B1`, `C2`}, values)
		})
	})

	s.Describe(`proxy like behavior for underlying iterator object`, func(s *testcase.Spec) {
		inputStream.Let(s, func(t *testcase.T) iter.Seq[string] {
			m := iterkit.Stub[string](iterkit.Empty[string]())
			m.StubErr = func() error {
				return errors.New(`ErrErr`)
			}
			m.StubClose = func() error {
				return errors.New(`ErrClose`)
			}
			return m
		})

		transform.Let(s, func(t *testcase.T) func(string) (string, error) {
			return func(s string) (string, error) { return s, nil }
		})

		s.Then(`close is the underlying iterators's close return value`, func(t *testcase.T) {
			err := subject(t).Close()
			t.Must.NotNil(err)
			t.Must.Equal(`ErrClose`, err.Error())
		})

		s.Then(`Err is the underlying iterators's Err return value`, func(t *testcase.T) {
			err := subject(t).Err()
			t.Must.NotNil(err)
			t.Must.Equal(`ErrErr`, err.Error())
		})
	})
}

func TestFirst_NextValueDecodable_TheFirstNextValueDecoded(t *testing.T) {
	t.Parallel()

	var expected int = 42
	i := iterkit.Slice([]int{expected, 4, 2})

	actually, found, err := iterkit.First[int](i)
	assert.Must(t).Nil(err)
	assert.Equal(t, expected, actually)
	assert.True(t, found)
}

func TestFirst_AfterFirstValue_IteratorIsClosed(t *testing.T) {
	t.Parallel()

	i := iterkit.Stub[Entity](iterkit.Slice[Entity]([]Entity{{Text: "hy!"}}))

	closed := false
	i.StubClose = func() error {
		closed = true
		return nil
	}

	_, _, err := iterkit.First[Entity](i)
	if err != nil {
		t.Fatal(err)
	}
	assert.True(t, closed)
}

func TestFirst_errors(t *testing.T) {
	FirstAndLastSharedErrorTestCases(t, iterkit.First[Entity])
}

func TestFirst_WhenNextSayThereIsNoValueToBeDecoded_ErrorReturnedAboutThis(t *testing.T) {
	t.Parallel()

	_, found, err := iterkit.First[Entity](iterkit.Empty[Entity]())
	assert.Must(t).Nil(err)
	assert.False(t, found)
}

func ExamplePipe() {
	var (
		i *iterkit.PipeIn[int]
		o *iterkit.PipeOut[int]
	)

	i, o = iterkit.Pipe[int]()
	_ = i // use it to send values
	_ = o // use it to consume values on each iteration (iter.Next())
}

func TestPipe_SimpleFeedScenario(t *testing.T) {
	t.Parallel()
	w, r := iterkit.Pipe[Entity]()

	expected := Entity{Text: "hitchhiker's guide to the galaxy"}

	go func() {
		defer w.Close()
		assert.True(t, w.Value(expected))
	}()

	assert.True(t, r.Next())             // first next should return the value mean to be sent
	assert.Equal(t, expected, r.Value()) // the exactly same value passed in
	assert.False(t, r.Next())            // no more values left, sender done with its work
	assert.Must(t).Nil(r.Err())          // No error sent so there must be no err received
	assert.Must(t).Nil(r.Close())        // Than I release this resource too
}

func TestPipe_FetchWithCollectAll(t *testing.T) {
	t.Parallel()
	w, r := iterkit.Pipe[*Entity]()

	var actually []*Entity
	var expected []*Entity = []*Entity{
		&Entity{Text: "hitchhiker's guide to the galaxy"},
		&Entity{Text: "The 5 Elements of Effective Thinking"},
		&Entity{Text: "The Art of Agile Development"},
		&Entity{Text: "The Phoenix Project"},
	}

	go func() {
		defer w.Close()

		for _, e := range expected {
			w.Value(e)
		}
	}()

	actually, err := iterkit.Collect[*Entity](r)
	assert.Must(t).Nil(err)             // When I collect everything with Collect All and close the resource
	assert.True(t, len(actually) > 0)   // the collection includes all the sent values
	assert.Equal(t, expected, actually) // which is exactly the same that mean to be sent.
}

func TestPipe_ReceiverCloseResourceEarly_FeederNoted(t *testing.T) {
	t.Parallel()

	// skip when only short test expected
	// this test is slow because it has sleep in it
	//
	// This could be fixed by implementing extra logic in the Pipe iterator,
	// but that would be over-engineering because after an iterator is closed,
	// it is highly unlikely that next value and decode will be called.
	// So this is only for the sake of describing the iterator behavior in this edge case
	if testing.Short() {
		t.Skip()
	}

	w, r := iterkit.Pipe[*Entity]()

	assert.Must(t).Nil(r.Close()) // I release the resource,
	// for example something went wrong during the processing on my side (receiver) and I can't continue work,
	// but I want to note this to the sender as well
	assert.Must(t).Nil(r.Close()) // multiple times because defer ensure and other reasons

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer w.Close()
		assert.Equal(t, false, w.Value(&Entity{Text: "hitchhiker's guide to the galaxy"}))
	}()

	wg.Wait()
	assert.False(t, r.Next()) // the sender is notified about this and stopped sending messages
}

func TestPipe_SenderSendErrorAboutProcessingToReceiver_ReceiverNotified(t *testing.T) {
	t.Parallel()

	w, r := iterkit.Pipe[Entity]()
	value := Entity{Text: "hitchhiker's guide to the galaxy"}
	expected := errors.New("boom")

	go func() {
		assert.True(t, w.Value(value))
		w.Error(expected)
		assert.Must(t).Nil(w.Close())
	}()

	assert.True(t, r.Next())           // everything goes smoothly, I'm notified about next value
	assert.Equal(t, value, r.Value())  // I even able to decode it as well
	assert.False(t, r.Next())          // Than the sender is notify me that I will not receive any more value
	assert.Equal(t, expected, r.Err()) // Also tells me that something went wrong during the processing
	assert.Must(t).Nil(r.Close())      // I release the resource because than and go on
	assert.Equal(t, expected, r.Err()) // The last error should be available later
}

func TestPipe_witchContexCancellation(t *testing.T) {
	t.Run("pipe-in Value is cancelled", func(t *testing.T) {

		ctx, cancel := context.WithCancel(context.Background())
		w, r := iterkit.PipeWithContext[Entity](ctx)
		defer r.Close()
		defer w.Close()

		timeout := time.Second / 4

		a := assert.NotWithin(t, timeout, func(ctx context.Context) {
			value := Entity{Text: "hitchhiker's guide to the galaxy"}
			assert.False(t, w.Value(value))
		}, "expected that writing is cancelled")

		cancel()

		assert.Within(t, timeout, func(ctx context.Context) {
			a.Wait()
		})

		assert.Within(t, timeout/10, func(ctx context.Context) {
			assert.ErrorIs(t, r.Err(), context.Canceled)
		})
	})

	t.Run("pipe-in Error is cancelled", func(t *testing.T) {

		ctx, cancel := context.WithCancel(context.Background())
		w, r := iterkit.PipeWithContext[Entity](ctx)
		defer r.Close()
		defer w.Close()

		timeout := time.Second / 4

		err := rnd.Error()

		a := assert.NotWithin(t, timeout, func(ctx context.Context) {
			rnd.Repeat(3, 5, func() {
				w.Error(err)
			})
		})

		cancel()

		assert.Within(t, timeout, func(ctx context.Context) {
			a.Wait()
		})

		assert.Within(t, timeout, func(ctx context.Context) {
			assert.Eventually(t, timeout, func(t assert.It) {
				got := r.Err()
				assert.ErrorIs(t, got, context.Canceled)
				assert.ErrorIs(t, got, err)
			})
		})
	})

	t.Run("pipe-out is cancelled", func(t *testing.T) {

		ctx, cancel := context.WithCancel(context.Background())
		w, r := iterkit.PipeWithContext[Entity](ctx)
		defer r.Close()
		defer w.Close()

		timeout := time.Second / 4

		a := assert.NotWithin(t, timeout, func(ctx context.Context) {
			assert.False(t, r.Next())
		})

		cancel()

		assert.Within(t, timeout, func(ctx context.Context) {
			a.Wait()
		})

		assert.Within(t, timeout/10, func(ctx context.Context) {
			assert.ErrorIs(t, r.Err(), context.Canceled)
		})

	})
}

func TestPipe_SenderSendErrorAboutProcessingToReceiver_ErrCheckPassBeforeAndReceiverNotifiedAfterTheError(t *testing.T) {
	// if there will be a use-case where iterator Err being checked before iter.Next
	// then this test will be resurrected and will be implemented.[int]
	t.Skip(`YAGNI`)

	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	expected := errors.New("Boom!")
	value := Entity{Text: "hitchhiker's guide to the galaxy"}

	w, r := iterkit.Pipe[Entity]()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		assert.True(t, w.Value(value))
		wg.Wait()
		w.Error(expected)
		assert.Must(t).Nil(w.Close())
	}()

	assert.Must(t).Nil(r.Err()) // no error so far
	wg.Done()
	assert.True(t, r.Next())           // everything goes smoothly, I'm notified about next value
	assert.Equal(t, value, r.Value())  // I even able to decode it as well
	assert.Equal(t, expected, r.Err()) // Also tells me that something went wrong during/after the processing
	assert.Must(t).Nil(r.Close())      // I release the resource because than and go on
	assert.Equal(t, expected, r.Err()) // The last error should be available later
}

func TestPipe_SenderSendNilAsErrorAboutProcessingToReceiver_ReceiverReceiveNothing(t *testing.T) {
	t.Parallel()

	value := Entity{Text: "hitchhiker's guide to the galaxy"}
	w, r := iterkit.Pipe[Entity]()

	go func() {
		for i := 0; i < 10; i++ {
			w.Error(nil)
		}

		assert.True(t, w.Value(value))
		assert.Must(t).Nil(w.Close())
	}()

	assert.True(t, r.Next())
	assert.Equal(t, value, r.Value())
	assert.False(t, r.Next())
	assert.Equal(t, nil, r.Err())
	assert.Must(t).Nil(r.Close())
	assert.Equal(t, nil, r.Err())
}

func TestPipeOut_Err_e2e(t *testing.T) {
	t.Parallel()

	expErr := rnd.Error()
	w, r := iterkit.Pipe[Entity]()

	go func() {
		defer w.Close()
		w.Value(Entity{Text: rnd.String()})
		w.Error(expErr)
	}()

	assert.Within(t, time.Second, func(ctx context.Context) {
		assert.NoError(t, r.Err())
		assert.True(t, r.Next())
		assert.NotEmpty(t, r.Value())
		assert.False(t, r.Next())
		assert.Equal(t, expErr, r.Err())
	})
}

func TestPipe_race(t *testing.T) {
	w, r := iterkit.Pipe[Entity]()

	testcase.Race(func() {
		w.Value(Entity{Text: rnd.String()})
		w.Error(rnd.Error())
		_ = w.Close()
	}, func() {
		assert.NoError(t, r.Err())
		for r.Next() {
			assert.NotEmpty(t, r.Value())
		}
		assert.Error(t, r.Err())
	})
}

func TestPipeOut_Err_whenCheckingErrBeforeConsumingValuesMakesItNonBlocking(t *testing.T) {
	t.Parallel()
	const timeout = 250 * time.Millisecond

	w, r := iterkit.Pipe[Entity]()

	t.Log("before consuming the input pipe, the .Err() is non-blocking")
	assert.Within(t, timeout, func(ctx context.Context) {
		assert.NoError(t, r.Err())
	})

	var (
		inCanFinish int32
		inIsDone    int32
	)
	go func() {
		defer atomic.AddInt32(&inIsDone, 1)
		defer w.Close()
		w.Value(Entity{Text: rnd.String()})
		for atomic.LoadInt32(&inCanFinish) == 0 {
			runtime.Gosched()
		}
	}()

	assert.True(t, r.Next())
	assert.NotEmpty(t, r.Value())

	t.Log("after consuming the pipe, the .Err() becomes blocking to ensure that last error response is received properly")
	assert.NotWithin(t, timeout, func(ctx context.Context) {
		assert.Nil(t, r.Err())
	})

	atomic.AddInt32(&inCanFinish, 1)

	assert.Eventually(t, time.Second, func(it assert.It) {
		assert.Equal(it, atomic.LoadInt32(&inIsDone), 1)
	})

	t.Log("after the IN pipe is done, the Err becomes non-blocking again")
	assert.Within(t, timeout, func(ctx context.Context) {
		assert.NoError(t, r.Err())
	})
}

func TestFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	type FN func() (value string, more bool, err error)
	var (
		fn  = testcase.Let[FN](s, nil)
		cbs = testcase.LetValue[[]iterkit.CallbackOption](s, nil)
	)
	act := testcase.Let(s, func(t *testcase.T) iter.Seq[string] {
		return iterkit.Func[string](fn.Get(t), cbs.Get(t)...)
	})

	s.When("func yields values", func(s *testcase.Spec) {
		values := testcase.Let(s, func(t *testcase.T) []string {
			var vs []string
			for i, m := 0, t.Random.IntB(1, 5); i < m; i++ {
				vs = append(vs, t.Random.String())
			}
			return vs
		})

		fn.Let(s, func(t *testcase.T) FN {
			var i int
			return func() (string, bool, error) {
				vs := values.Get(t)
				if !(i < len(vs)) {
					return "", false, nil
				}
				v := vs[i]
				i++
				return v, true, nil
			}
		})

		s.Test("then value collected without an issue", func(t *testcase.T) {
			vs, err := iterkit.Collect[string](act.Get(t))
			t.Must.Nil(err)
			t.Must.Equal(values.Get(t), vs)
		})
	})

	s.When("func yields an error", func(s *testcase.Spec) {
		expectedErr := testcase.Let(s, func(t *testcase.T) error {
			return t.Random.Error()
		})

		count := testcase.LetValue(s, 0)
		fn.Let(s, func(t *testcase.T) FN {
			return func() (string, bool, error) {
				count.Set(t, count.Get(t)+1)
				return t.Random.String(), t.Random.Bool(), expectedErr.Get(t)
			}
		})

		s.Test("then no value is fetched and error is returned with .Err()", func(t *testcase.T) {
			iter := act.Get(t)
			t.Must.False(iter.Next())
			t.Must.ErrorIs(expectedErr.Get(t), iter.Err())
		})

		s.Then("on repeated calls, function is called no more", func(t *testcase.T) {
			iter := act.Get(t)
			t.Must.False(iter.Next())
			t.Must.ErrorIs(expectedErr.Get(t), iter.Err())

			iter = act.Get(t)
			t.Must.False(iter.Next())
			t.Must.ErrorIs(expectedErr.Get(t), iter.Err())

			t.Must.Equal(1, count.Get(t))
		})
	})

	s.When("callback is provided", func(s *testcase.Spec) {
		fn.Let(s, func(t *testcase.T) FN {
			return func() (string, bool, error) {
				return "", false, nil
			}
		})

		closed := testcase.LetValue(s, false)
		cbs.Let(s, func(t *testcase.T) []iterkit.CallbackOption {
			return []iterkit.CallbackOption{
				iterkit.OnClose(func() error {
					closed.Set(t, true)
					return nil
				}),
			}
		})

		s.Test("then value collected without an issue", func(t *testcase.T) {
			vs, err := iterkit.Collect[string](act.Get(t))
			t.Must.Nil(err)
			t.Must.Empty(vs)
			t.Must.True(closed.Get(t))
		})
	})
}

const defaultBatchSize = 64

func TestBatch(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = testcase.Let[[]int](s, func(t *testcase.T) []int {
			return random.Slice[int](t.Random.IntB(50, 200), func() int {
				return t.Random.Int()
			})
		})
		src = testcase.Let[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice(values.Get(t))
		})
		size = testcase.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) * 2
		})
	)
	act := testcase.Let[iter.Seq[[]int]](s, func(t *testcase.T) iter.Seq[[]int] {
		return iterkit.Batch(src.Get(t), size.Get(t))
	})

	s.When("size is a valid positive value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(1, len(values.Get(t)))
		})

		s.Then("batching size is used", func(t *testcase.T) {
			iter := act.Get(t)
			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Log(len(iter.Value()) <= size.Get(t), len(iter.Value()), size.Get(t))
				t.Must.True(len(iter.Value()) <= size.Get(t))
				t.Must.NotEmpty(iter.Value())
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	})

	s.When("size is an invalid value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			// negative value is not acceptable
			return t.Random.IntB(1, 7) * -1
		})

		s.Then("iterate with default value(s)", func(t *testcase.T) {
			iter := act.Get(t)
			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Must.NotEmpty(iter.Value())
				t.Must.True(len(iter.Value()) <= defaultBatchSize, "iteration ")
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	})
}

func TestBatchWithTimeout(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		values = testcase.Let[[]int](s, func(t *testcase.T) []int {
			var vs []int
			for i, l := 0, t.Random.IntB(3, 7); i < l; i++ {
				vs = append(vs, t.Random.Int())
			}
			return vs
		})
		src = testcase.Let[iter.Seq[int]](s, func(t *testcase.T) iter.Seq[int] {
			return iterkit.Slice(values.Get(t))
		})
		size = testcase.Let(s, func(t *testcase.T) int {
			return len(values.Get(t)) * 2
		})
		timeout = testcase.LetValue[time.Duration](s, 0)
	)
	act := testcase.Let[iter.Seq[[]int]](s, func(t *testcase.T) iter.Seq[[]int] {
		return iterkit.BatchWithTimeout(src.Get(t), size.Get(t), timeout.Get(t))
	})

	ThenIterateWithDefaultValue := func(s *testcase.Spec) {
		s.Then("iterate with default value(s)", func(t *testcase.T) {
			iter := act.Get(t)

			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Must.NotEmpty(iter.Value())
				t.Must.True(len(iter.Value()) < defaultBatchSize, "iterate with default batch size")
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	}

	ThenIterateWithDefaultValue(s)

	s.When("size is a valid positive value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			return t.Random.IntB(1, len(values.Get(t)))
		})

		s.Then("batch size corresponds to the configuration", func(t *testcase.T) {
			iter := act.Get(t)
			var got []int
			for iter.Next() {
				t.Must.Equal(iter.Value(), iter.Value())
				t.Must.True(len(iter.Value()) <= size.Get(t))
				t.Must.NotEmpty(iter.Value())
				got = append(got, iter.Value()...)
			}
			t.Must.NotEmpty(got)
			t.Must.ContainExactly(values.Get(t), got)
		})
	})

	s.And("size is an invalid value", func(s *testcase.Spec) {
		size.Let(s, func(t *testcase.T) int {
			// negative value is not acceptable
			return t.Random.IntB(1, 7) * -1
		})

		ThenIterateWithDefaultValue(s)
	})

	s.When("timeout is valid positive value", func(s *testcase.Spec) {
		timeout.Let(s, func(t *testcase.T) time.Duration {
			return 100 * time.Millisecond
		})

		type Pipe struct {
			In  *iterkit.PipeIn[int]
			Out *iterkit.PipeOut[int]
		}
		pipe := testcase.Let[Pipe](s, func(t *testcase.T) Pipe {
			in, out := iterkit.Pipe[int]()
			t.Defer(in.Close)
			t.Defer(out.Close)
			go func() {
				for _, v := range values.Get(t) {
					if !in.Value(v) {
						break
					}
				}
				// wait forever to trigger batching
			}()
			return Pipe{
				In:  in,
				Out: out,
			}
		})
		src.Let(s, func(t *testcase.T) iter.Seq[int] {
			return pipe.Get(t).Out
		})

		s.Then("batch timeout corresponds to the configuration", func(t *testcase.T) {
			iter := act.Get(t)
			t.Must.True(iter.Next()) // trigger batching
			t.Must.ContainExactly(values.Get(t), iter.Value())
		})
	})

	s.When("timeout is an invalid value", func(s *testcase.Spec) {
		timeout.Let(s, func(t *testcase.T) time.Duration {
			return time.Duration(t.Random.IntB(500, 1000)) * time.Microsecond * -1
		})

		ThenIterateWithDefaultValue(s)
	})
}

var _ iter.Seq[any] = iterkit.Error[any](errors.New("boom"))

func TestNewError_ErrorGiven_NotIterableIteratorReturnedWithError(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom!")
	i := iterkit.Error[any](expectedError)
	assert.False(t, i.Next())
	assert.Must(t).Nil(i.Value())
	assert.Must(t).NotNil(expectedError, assert.Message(pp.Format(i.Err())))
	assert.Must(t).Nil(i.Close())
}

var _ iter.Seq[any] = iterkit.Stub[any](iterkit.Empty[any]())

func TestMock_Err(t *testing.T) {
	t.Parallel()

	originalError := errors.New("Boom! original")
	expectedError := errors.New("Boom! stub")

	m := iterkit.Stub[any](iterkit.Error[any](originalError))

	// default is the wrapped iterator
	assert.Must(t).NotNil(originalError, assert.Message(pp.Format(m.Err())))

	m.StubErr = func() error { return expectedError }
	assert.Must(t).NotNil(expectedError, assert.Message(pp.Format(m.Err())))

	m.ResetErr()
	assert.Must(t).NotNil(originalError, assert.Message(pp.Format(m.Err())))

}

func TestMock_Close(t *testing.T) {
	t.Parallel()

	expectedError := errors.New("Boom! stub")

	m := iterkit.Stub[any](iterkit.Empty[any]())

	// default is the wrapped iterator
	assert.Must(t).Nil(m.Close())

	m.StubClose = func() error { return expectedError }
	assert.Must(t).NotNil(expectedError, assert.Message(pp.Format(m.Close())))

	m.ResetClose()
	assert.Must(t).Nil(m.Close())
}

func TestMock_Next(t *testing.T) {
	t.Parallel()

	m := iterkit.Stub[any](iterkit.Empty[any]())

	assert.False(t, m.Next())

	m.StubNext = func() bool { return true }
	assert.True(t, m.Next())

	m.ResetNext()
	assert.False(t, m.Next())
}

func TestMock_Decode(t *testing.T) {
	t.Parallel()

	m := iterkit.Stub[int](iterkit.Slice[int]([]int{42, 43, 44}))

	assert.True(t, m.Next())
	assert.Equal(t, 42, m.Value())

	assert.True(t, m.Next())
	assert.Equal(t, 43, m.Value())

	m.StubValue = func() int {
		return 4242
	}
	assert.Equal(t, 4242, m.Value())

	m.ResetValue()
	assert.True(t, m.Next())
	assert.Equal(t, 44, m.Value())
}

func ExampleScanner() {
	reader := strings.NewReader("a\nb\nc\nd")
	sc := bufio.NewScanner(reader)
	sc.Split(bufio.ScanLines)
	i := iterkit.BufioScanner[string](sc, nil)
	for i.Next() {
		fmt.Println(i.Value())
	}
	fmt.Println(i.Err())
}

func ExampleScanner_Split() {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i := iterkit.BufioScanner[string](scanner, nil)
	for i.Next() {
		fmt.Println(i.Value())
	}
	fmt.Println(i.Err())
}

func TestScanner_SingleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("Hello, World!"))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.True(t, i.Next())
	assert.Equal(t, "Hello, World!", i.Value())
	assert.False(t, i.Next())
}

func TestScanner_nilCloserGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("foo\nbar\nbaz"))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), nil)

	assert.True(t, i.Next())
	assert.Equal(t, "foo", i.Value())
	assert.True(t, i.Next())
	assert.Equal(t, "bar", i.Value())
	assert.True(t, i.Next())
	assert.Equal(t, "baz", i.Value())
	assert.False(t, i.Next())
	assert.Must(t).Nil(i.Close())
}

func TestScanner_ClosableIOGiven_OnCloseItIsClosed(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader(`Hy`))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)
	assert.Must(t).Nil(i.Close())
	assert.Must(t).NotNil(i.Close(), "already closed")
}

func TestScanner_MultipleLineGiven_EachLineFetched(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(strings.NewReader("Hello, World!\nHow are you?\r\nThanks I'm fine!"))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.True(t, i.Next())
	assert.Equal(t, "Hello, World!", i.Value())

	assert.True(t, i.Next())
	assert.Equal(t, "How are you?", i.Value())

	assert.True(t, i.Next())
	assert.Equal(t, "Thanks I'm fine!", i.Value())

	assert.False(t, i.Next())
}

func TestScanner_NilReaderGiven_ErrorReturned(t *testing.T) {
	t.Parallel()

	readCloser := NewReadCloser(new(BrokenReader))
	i := iterkit.BufioScanner[string](bufio.NewScanner(readCloser), readCloser)

	assert.False(t, i.Next())
	assert.Must(t).NotNil(io.ErrUnexpectedEOF, assert.Message(pp.Format(i.Err())))
}

func TestScanner_Split(t *testing.T) {
	reader := strings.NewReader("a\nb\nc\nd")
	scanner := bufio.NewScanner(reader)
	scanner.Split(bufio.ScanLines)
	i := iterkit.BufioScanner[string](scanner, nil)

	lines, err := iterkit.Collect[string](i)
	assert.Must(t).Nil(err)
	assert.Equal(t, 4, len(lines))
	assert.Equal(t, `a`, lines[0])
	assert.Equal(t, `b`, lines[1])
	assert.Equal(t, `c`, lines[2])
	assert.Equal(t, `d`, lines[3])
}

func TestWithConcurrentAccess(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test(`it will protect against concurrent access`, func(t *testcase.T) {
		var i iter.Seq[int]
		i = iterkit.Slice([]int{1, 2})
		i = iterkit.WithConcurrentAccess(i)

		var wg sync.WaitGroup
		wg.Add(2)

		var a, b int
		flag := make(chan struct{})
		go func() {
			defer wg.Done()
			<-flag
			t.Log("a:start")
			assert.True(t, i.Next())
			time.Sleep(time.Millisecond)
			a = i.Value()
			t.Log("a:done")
		}()
		go func() {
			defer wg.Done()
			<-flag
			t.Log("b:start")
			assert.True(t, i.Next())
			time.Sleep(time.Millisecond)
			b = i.Value()
			t.Log("b:done")
		}()

		close(flag) // start
		t.Log("wait")
		wg.Wait()
		t.Log("wait done")

		assert.Must(t).ContainExactly([]int{1, 2}, []int{a, b})
	})

	s.Test(`classic behavior`, func(t *testcase.T) {
		var i iter.Seq[int]
		i = iterkit.Slice([]int{1, 2})
		i = iterkit.WithConcurrentAccess(i)

		var vs []int
		vs, err := iterkit.Collect(i)
		assert.Must(t).Nil(err)
		assert.Must(t).ContainExactly([]int{1, 2}, vs)
	})

	s.Test(`proxy like behavior for underlying iterator object`, func(t *testcase.T) {
		m := iterkit.Stub[int](iterkit.Empty[int]())
		m.StubErr = func() error {
			return errors.New(`ErrErr`)
		}
		m.StubClose = func() error {
			return errors.New(`ErrClose`)
		}
		i := iterkit.WithConcurrentAccess[int](m)

		err := i.Close()
		assert.Must(t).NotNil(err)
		assert.Equal(t, `ErrClose`, err.Error())

		err = i.Err()
		assert.Must(t).NotNil(err)
		assert.Equal(t, `ErrErr`, err.Error())
	})
}

func TestWithErr(t *testing.T) {
	t.Run("NoError", func(t *testing.T) {
		iter := iterkit.Slice([]int{1, 2, 3})
		errIter := iterkit.WithErr(iter, nil)
		assert.NotNil(t, errIter)
		vs, err := iterkit.Collect(errIter)
		assert.NoError(t, err)
		assert.Equal(t, vs, []int{1, 2, 3})
	})

	t.Run("WithError", func(t *testing.T) {
		iter := iterkit.Slice([]int{1, 2, 3})
		expErr := rnd.Error()
		errIter := iterkit.WithErr(iter, expErr)
		assert.NotNil(t, errIter)
		assert.False(t, errIter.Next())
		assert.ErrorIs(t, expErr, errIter.Err())
	})

	t.Run("NilIterator", func(t *testing.T) {
		iter := iter.Seq[int](nil)
		assert.Nil(t, iterkit.WithErr(iter, nil))
	})

	t.Run("NilIteratorWithError", func(t *testing.T) {
		iter := iter.Seq[int](nil)
		expErr := rnd.Error()
		errIter := iterkit.WithErr(iter, expErr)
		assert.NotNil(t, errIter)
		assert.False(t, errIter.Next())
		assert.Equal(t, expErr, errIter.Err())
	})

	t.Run("CloseClosesUnderlyingIterator", func(t *testing.T) {
		iter := iterkit.Stub(iterkit.Slice([]int{1, 2, 3}))
		closeCalled := false
		iter.StubClose = func() error {
			closeCalled = true
			return nil
		}
		expErr := rnd.Error()
		errIter := iterkit.WithErr[int](iter, expErr)
		assert.ErrorIs(t, errIter.Err(), expErr)
		assert.NoError(t, errIter.Close())
		assert.True(t, closeCalled)
	})

	t.Run("ClosePropagatesUnderlyingIteratorCloseError", func(t *testing.T) {
		expErr1 := rnd.Error()
		expErr2 := rnd.Error()
		iter := iterkit.Stub(iterkit.Slice([]int{1, 2, 3}))
		iter.StubClose = func() error {
			return expErr2
		}
		errIter := iterkit.WithErr[int](iter, expErr1)
		assert.ErrorIs(t, errIter.Err(), expErr1)
		assert.ErrorIs(t, expErr2, errIter.Close())
	})

	t.Run("ErrPropagatesUnderlyingIteratorError", func(t *testing.T) {
		expErr := rnd.Error()
		iter := iterkit.Stub(iterkit.Slice([]int{1, 2, 3}))
		iter.StubErr = func() error {
			return expErr
		}
		errIter := iterkit.WithErr[int](iter, expErr)
		assert.ErrorIs(t, errIter.Err(), expErr)
	})
}

func TestMerge(t *testing.T) {
	t.Run("EmptyIterators", func(t *testing.T) {
		iter := iterkit.Merge[int]()
		vs, err := iterkit.Collect(iter)
		assert.NoError(t, err)
		assert.Empty(t, vs)
	})

	t.Run("SingleIterator", func(t *testing.T) {
		iter1 := iterkit.Slice([]int{1, 2, 3})
		mergedIter := iterkit.Merge(iter1)
		valuses, err := iterkit.Collect(mergedIter)
		assert.NoError(t, err)
		assert.Equal(t, valuses, []int{1, 2, 3})
	})

	t.Run("MultipleIterators", func(t *testing.T) {
		iter1 := iterkit.Slice([]int{1, 2})
		iter2 := iterkit.Slice([]int{3, 4})
		iter3 := iterkit.Slice([]int{5, 6})
		vs, err := iterkit.Collect(iterkit.Merge(iter1, iter2, iter3))
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3, 4, 5, 6}, vs)
	})

	t.Run("IteratorsWithError", func(t *testing.T) {

		iter1 := iterkit.Slice([]int{1, 2})
		expErr := rnd.Error()
		iter2 := iterkit.Error[int](expErr)
		mergedIter := iterkit.Merge(iter1, iter2)
		values := []int{}
		for mergedIter.Next() {
			values = append(values, mergedIter.Value())
		}
		assert.ErrorIs(t, expErr, mergedIter.Err())
		assert.Equal(t, []int{1, 2}, values)
	})

	t.Run("CloseClosesUnderlyingIterators", func(t *testing.T) {
		iter1 := iterkit.Stub(iterkit.Slice([]int{1, 2}))
		closeCalled1 := false
		iter1.StubClose = func() error {
			closeCalled1 = true
			return nil
		}
		iter2 := iterkit.Stub(iterkit.Slice([]int{3, 4}))
		closeCalled2 := false
		iter2.StubClose = func() error {
			closeCalled2 = true
			return nil
		}
		mergedIter := iterkit.Merge[int](iter1, iter2)
		assert.NoError(t, mergedIter.Close())
		assert.True(t, closeCalled1)
		assert.True(t, closeCalled2)
	})

	t.Run("UnderlyingIteratorErrorsReturnedWithErr", func(t *testing.T) {
		expErr1 := rnd.Error()
		expErr2 := rnd.Error()
		mergedIter := iterkit.Merge[int](iterkit.Error[int](expErr1), iterkit.Error[int](expErr2))
		assert.NoError(t, mergedIter.Close())
		assert.ErrorIs(t, mergedIter.Err(), expErr1)
		assert.ErrorIs(t, mergedIter.Err(), expErr2)
	})
}
