package postgresql_test

import (
	"database/sql"
	"github.com/adamluzsi/frameless/resources/postgresql"
)

var (
	_ postgresql.SQLClient = &sql.DB{}
	_ postgresql.SQLClient = &sql.Tx{}
)
