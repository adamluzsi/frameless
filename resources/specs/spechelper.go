package specs

import (
	"fmt"

	"github.com/adamluzsi/frameless/resources"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
)

func extIDFieldRequired(s *testcase.Spec, entityType interface{}) {
	entityTypeName := reflects.FullyQualifiedName(entityType)
	desc := fmt.Sprintf(`An ext:ID field is given in %s`, entityTypeName)
	s.Test(desc, func(t *testcase.T) {
		_, hasExtID := resources.LookupID(reflects.New(entityType))
		require.True(t, hasExtID, frameless.ErrIDRequired.Error())
	})
}
