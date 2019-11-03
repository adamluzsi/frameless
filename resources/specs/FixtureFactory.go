package specs

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type FixtureFactory interface {
	// Create create a newEntityBasedOn struct instance based on the received input struct type.
	// Create also populate the struct field with dummy values.
	// It is expected that the newly created fixture will have no content for extID field.
	Create(EntityType interface{}) (StructPTR interface{})
	// Context able to provide the specs with a context object for a certain entity Type.
	Context() (ctx context.Context)
}

type FixtureFactorySpec struct {
	Type interface{}
	FixtureFactory
}

func (spec FixtureFactorySpec) Test(t *testing.T) {
	s := testcase.NewSpec(t)
	s.Parallel()

	s.Describe(`Create`, func(s *testcase.Spec) {
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
			if _, hasExtID := resources.LookupID(spec.Type); !hasExtID {
				return
			}

			s.Then(`it should leave it empty without any value for the fixtures`, func(t *testcase.T) {
				fixture := subject(t)
				extID, has := resources.LookupID(fixture)
				require.True(t, has)
				require.Empty(t, extID)
			})
		})
	})

	s.Describe(`Context`, func(s *testcase.Spec) {
		subject := func(t *testcase.T) context.Context {
			return spec.FixtureFactory.Context()
		}

		s.Then(`it will return a context`, func(t *testcase.T) {
			require.NotNil(t, subject(t))
		})

		s.Then(`the context expected to be not cancelled`, func(t *testcase.T) {
			require.Nil(t, subject(t).Err())
		})
	})
}
