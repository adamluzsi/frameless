package specs

import (
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/resources"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

type resource interface {
	resources.Saver
	resources.Finder
	resources.FinderAll
	resources.Updater
	resources.Deleter
	resources.Truncater
}

func extIDFieldRequired(s *testcase.Spec, entityType interface{}) {
	entityTypeName := reflects.FullyQualifiedName(entityType)
	desc := fmt.Sprintf(`An ext:ID field is given in %s`, entityTypeName)
	s.Test(desc, func(t *testcase.T) {
		_, hasExtID := resources.LookupID(reflects.New(entityType))
		require.True(t, hasExtID, frameless.ErrIDRequired.Error())
	})
}

func createEntities(count int, f FixtureFactory, T interface{}) []interface{} {
	var es []interface{}
	for i := 0; i < count; i++ {
		es = append(es, f.Create(T))
	}
	return es
}

func saveEntities(tb testing.TB, s resources.Saver, f FixtureFactory, es ...interface{}) []string {
	var ids []string
	for _, e := range es {
		require.Nil(tb, s.Save(f.Context(), e))
		id, _ := resources.LookupID(e)
		ids = append(ids, id)
	}
	return ids
}

func cleanup(tb testing.TB, t resources.Truncater, f FixtureFactory, T interface{}) {
	require.Nil(tb, t.Truncate(f.Context(), T))
}
