package postgresql_test

import "go.llib.dev/frameless/adapters/postgresql"

var _ postgresql.Mapping[any, int] = postgresql.Mapper[any, int]{}
