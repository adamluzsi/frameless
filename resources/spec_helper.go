package resources

import (
	"fmt"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func extIDFieldRequired(s *testcase.Spec, entityType interface{}) {
	entityTypeName := reflects.FullyQualifiedName(entityType)
	desc := fmt.Sprintf(`An ext:ID field is given in %s`, entityTypeName)
	s.Test(desc, func(t *testcase.T) {
		_, hasExtID := LookupID(reflects.New(entityType))
		require.True(t, hasExtID, frameless.ErrIDRequired.Error())
	})
}

func name(e frameless.Entity) string {
	return reflects.BaseTypeOf(e).Name()
}
