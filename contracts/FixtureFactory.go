package contracts

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/extid"

	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type FixtureFactory interface {
	// Create create a newEntity struct instance based on the received input struct type.
	// Create also populate the struct field with dummy values.
	// It is expected that the newly created fixture will have no content for extID field.
	//Create(testing.TB, context.Context, any) any
	Create(T interface{}) (ptr interface{})
	// Context able to provide the specs with a Context object for a certain entity Type.
	Context() (ctx context.Context)
}

type FixtureFactorySpec struct {
	Type interface{}
	FixtureFactory
}

func (spec FixtureFactorySpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	s.Describe(`.Create`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) interface{} {
			return spec.FixtureFactory.Create(spec.Type)
		}

		s.Then(`each created fixture value is uniq`, func(t *testcase.T) {
			var results []interface{}

			for i := 0; i < 42; i++ {
				result := subject(t)
				require.NotContains(t, results, result)
				results = append(results, result)
			}
		})

		s.When(`when struct has Resource external ID`, func(s *testcase.Spec) {
			if _, _, hasExtIDField := extid.LookupStructField(spec.Type); !hasExtIDField {
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

	s.Describe(`.Context`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) context.Context {
			return spec.FixtureFactory.Context()
		}

		s.Then(`it will return a Context`, func(t *testcase.T) {
			require.NotNil(t, subject(t))
		})

		s.Then(`the Context expected to be not cancelled`, func(t *testcase.T) {
			require.Nil(t, subject(t).Err())
		})
	})
}

func (spec FixtureFactorySpec) Benchmark(b *testing.B) {
	b.Skip()
}