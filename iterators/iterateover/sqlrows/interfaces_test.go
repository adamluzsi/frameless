package sqlrows_test

import (
	"database/sql"
	"github.com/adamluzsi/frameless/iterators/iterateover/sqlrows"
)

var _ sqlrows.Rows = &sql.Rows{}
