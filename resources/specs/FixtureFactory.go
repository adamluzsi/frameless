package specs

import (
	"github.com/stretchr/testify/require"
	"testing"
	"github.com/adamluzsi/testcase"
)

type FixtureFactory interface {
	// Create create a new struct instance based on the received input struct type.
	// Create also populate the struct field with dummy values.
	// It is expected that the newly created fixture will have no content for extID field.
	Create(EntityType interface{}) (StructPTR interface{})
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

		s.When(`when struct has resource external ID`, func(s *testcase.Spec) {
			if _, hasExtID := LookupID(spec.Type); !hasExtID {
				return
			}

			s.Then(`it should leave it empty without any value for the fixtures`, func(t *testcase.T) {
				fixture := subject(t)
				extID, has := LookupID(fixture)
				require.True(t, has)
				require.Empty(t, extID)
			})
		})
	})
}
