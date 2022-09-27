package iterators_test

import (
	iterators2 "github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestFunc(t *testing.T) {
	s := testcase.NewSpec(t)

	type FN func() (value string, more bool, err error)
	var (
		fn = testcase.Let[FN](s, nil)
	)
	subject := testcase.Let(s, func(t *testcase.T) *iterators2.FuncIter[string] {
		return iterators2.Func[string](fn.Get(t))
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
			vs, err := iterators2.Collect[string](subject.Get(t))
			t.Must.Nil(err)
			t.Must.Equal(values.Get(t), vs)
		})
	})

	s.When("func yields an error", func(s *testcase.Spec) {
		expectedErr := testcase.Let(s, func(t *testcase.T) error {
			return t.Random.Error()
		})

		fn.Let(s, func(t *testcase.T) FN {
			return func() (string, bool, error) {
				return "", t.Random.Bool(), expectedErr.Get(t)
			}
		})

		s.Test("then no value is fetched and error is returned with .Err()", func(t *testcase.T) {
			iter := subject.Get(t)
			t.Must.False(iter.Next())
			t.Must.ErrorIs(expectedErr.Get(t), iter.Err())
		})
	})
}
