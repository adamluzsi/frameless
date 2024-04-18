package iterators_test

import (
	"context"
	"encoding/json"
	"go.llib.dev/frameless/pkg/dtos"
	"go.llib.dev/frameless/ports/iterators"
	. "go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"
)

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

		vs, err := dtos.Map[[]Foo](ctx, values)
		if err != nil {
			return nil, false, err
		}
		probablyHasNextPage := len(vs) == limit
		return vs, probablyHasNextPage, nil
	}

	foos := iterators.Paginate(ctx, fetchMoreFoo)
	_ = foos // foos can be called like any iterator,
	// and under the hood, the fetchMoreFoo function will be used dynamically,
	// to retrieve more values when the previously called values are already used up.
}

func TestPaginate(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		ctx  = let.Context(s)
		more = testcase.Let[func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error)](s, nil)
	)
	act := func(t *testcase.T) iterators.Iterator[Foo] {
		return iterators.Paginate(ctx.Get(t), more.Get(t))
	}

	s.When("more function returns no more values", func(s *testcase.Spec) {
		more.Let(s, func(t *testcase.T) func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
			return func(ctx context.Context, offset int) (values []Foo, hasMore bool, _ error) {
				return nil, false, nil
			}
		})

		s.Then("iteration finishes and we get the empty result", func(t *testcase.T) {
			vs, err := iterators.Collect(act(t))
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
			vs, err := iterators.Collect(act(t))
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
				vs, err := iterators.Collect(act(t))
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
			vs, err := iterators.Collect(act(t))
			assert.NoError(t, err)
			assert.Equal(t, vs, values.Get(t))
		})

		s.Test("if the iterator closed early on values are not retrieved any further", func(t *testcase.T) {
			iter := act(t)
			assert.NoError(t, iter.Close())

			vs, err := iterators.Collect(iter)
			assert.NoError(t, err)
			assert.Empty(t, vs)
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
			_, err := iterators.Collect(act(t))
			assert.ErrorIs(t, expErr.Get(t), err)
		})
	})
}
