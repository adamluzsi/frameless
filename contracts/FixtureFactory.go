package contracts

import (
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type FixtureFactory struct {
	T              interface{}
	FixtureFactory func(tb testing.TB) frameless.FixtureFactory
}

func (c FixtureFactory) String() string {
	return "FixtureFactory"
}

func (c FixtureFactory) Test(t *testing.T) { c.Spec(testcase.NewSpec(t)) }

func (c FixtureFactory) Benchmark(b *testing.B) { b.Skip() }

func (c FixtureFactory) Spec(s *testcase.Spec) {
	s.Parallel()
	factoryLet(s, c.FixtureFactory)

	s.Describe(`.Create`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) interface{} {
			return factoryGet(t).Fixture(c.T, nil)
		}

		s.Then(`each created fixture value is uniq`, func(t *testcase.T) {
			var results []interface{}

			for i := 0; i < 42; i++ {
				result := subject(t)
				require.NotContains(t, results, result)
				results = append(results, result)
			}
		})

		s.When(`struct has Resource external ID`, func(s *testcase.Spec) {
			if _, _, hasExtIDField := extid.LookupStructField(c.T); !hasExtIDField {
				return
			}

			s.Then(`it should leave it empty without any value for the fixtures`, func(t *testcase.T) {
				fixture := subject(t)
				extID, has := extid.Lookup(fixture)
				require.False(t, has)
				require.Empty(t, extID)
			})
		})
	})
}
