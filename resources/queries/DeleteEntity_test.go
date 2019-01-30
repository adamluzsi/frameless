package queries_test

import (
	"github.com/adamluzsi/frameless/resources"
	"github.com/adamluzsi/frameless/resources/queries"
)

var _ resources.Query = queries.DeleteEntity{}
