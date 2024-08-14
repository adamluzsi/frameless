package iterators_test

import (
	"testing"

	"go.llib.dev/frameless/port/iterators"
	"go.llib.dev/testcase"
)

func TestFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	type FN func() (value string, more bool, err error)
	var (
		fn  = testcase.Let[FN](s, nil)
		cbs = testcase.LetValue[[]iterators.CallbackOption](s, nil)
	)
	act := testcase.Let(s, func(t *testcase.T) iterators.Iterator[string] {
		return iterators.Func[string](fn.Get(t), cbs.Get(t)...)
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
			vs, err := iterators.Collect[string](act.Get(t))
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
		cbs.Let(s, func(t *testcase.T) []iterators.CallbackOption {
			return []iterators.CallbackOption{
				iterators.OnClose(func() error {
					closed.Set(t, true)
					return nil
				}),
			}
		})

		s.Test("then value collected without an issue", func(t *testcase.T) {
			vs, err := iterators.Collect[string](act.Get(t))
			t.Must.Nil(err)
			t.Must.Empty(vs)
			t.Must.True(closed.Get(t))
		})
	})
}
