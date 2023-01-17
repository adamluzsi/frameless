package postgresql_test

import "github.com/adamluzsi/frameless/adapters/postgresql"

var _ postgresql.Mapping[any, int] = postgresql.Mapper[any, int]{}
