package fixtures_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/fixtures"
)

func TestRandomInt(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Describe(`#RandomIntn`, func(s *testcase.Spec) {
		var subject = func(t *testcase.T) int {
			return fixtures.RandomIntn(t.I(`n`).(int))
		}

		s.Let(`n`, func(t *testcase.T) interface{} {
			return rand.Intn(42) + 42 // ensure it is not zero for the test
		})

		s.Test(`returns with random number excluding the received`, func(t *testcase.T) {
			out := subject(t)
			require.True(t, 0 <= out)
			require.True(t, out < t.I(`n`).(int))
		})
	})

	s.Describe(`#RandomIntByRange`, func(s *testcase.Spec) {
		var subject = func(t *testcase.T) int {
			return fixtures.RandomIntByRange(t.I(`numerical range`).([]int)...)
		}

		s.When(`numerical range includes one value`, func(s *testcase.Spec) {
			s.Let(`min`, func(t *testcase.T) interface{} {
				return fixtures.RandomIntn(42) + 1
			})

			s.Let(`numerical range`, func(t *testcase.T) interface{} {
				return []int{t.I(`min`).(int)}
			})

			s.Then(`it will return the exact number as the range only has one possible value`, func(t *testcase.T) {
				require.Equal(t, t.I(`min`).(int), subject(t))
			})
		})

		s.When(`numerical range includes a min and a max`, func(s *testcase.Spec) {
			s.Let(`min`, func(t *testcase.T) interface{} {
				return fixtures.RandomIntn(42)
			})

			s.Let(`max`, func(t *testcase.T) interface{} {
				// +1 in the end to ensure that `max` is bigger than `min`
				return fixtures.RandomIntn(42) + t.I(`min`).(int) + 1
			})

			s.Let(`numerical range`, func(t *testcase.T) interface{} {
				return []int{t.I(`min`).(int), t.I(`max`).(int)}
			})

			s.Then(`it will return a value between the range`, func(t *testcase.T) {
				out := subject(t)
				require.True(t, t.I(`min`).(int) <= out, `expected that from <= than out`)
				require.True(t, out < t.I(`max`).(int), `expected that out is < than max`)
			})

			s.And(`if the number list is not ascending sorted`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					nums := t.I(`numerical range`).([]int)
					sort.Sort(sort.Reverse(sort.IntSlice(nums)))
				})

				s.Then(`it will still return a value between the given number range min-max`, func(t *testcase.T) {
					out := subject(t)
					require.True(t, t.I(`min`).(int) <= out, `expected that from <= than out`)
					require.True(t, out < t.I(`max`).(int), `expected that out is < than max`)
				})
			})

			s.And(`the number list includes multiple values other than just min and maxes`, func(s *testcase.Spec) {
				s.Before(func(t *testcase.T) {
					t.Let(`max`, t.I(`max`).(int)+5)
					for i := t.I(`min`).(int); i < t.I(`max`).(int); i++ {
						t.Let(`numerical range`, append(t.I(`numerical range`).([]int), i))
					}
					require.True(t, len(t.I(`numerical range`).([]int)) > 2,
						`for the sake of the test, it is expected that there is more than 2 value here`)
				})

				s.Then(`it will still return a value between the given number range min-max`, func(t *testcase.T) {
					out := subject(t)
					require.True(t, t.I(`min`).(int) <= out, `expected that from <= than out`)
					require.True(t, out < t.I(`max`).(int), `expected that out is < than max`)
				})
			})
		})
	})
}
