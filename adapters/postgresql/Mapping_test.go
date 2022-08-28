package postgresql_test

import "github.com/adamluzsi/frameless/adapters/postgresql"

var _ postgresql.Mapping[int] = postgresql.Mapper[int, int]{}
