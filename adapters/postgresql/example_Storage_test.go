package postgresql_test

import (
	"context"
	"math/rand"
	"os"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/ports/iterators"
)

func ExampleRepository() {
	type Entity struct {
		ID    int `ext:"ID"`
		Value string
	}

	mapping := postgresql.Mapper[Entity, int]{
		Table:   "entities",
		ID:      "id",
		Columns: []string{`id`, `value`},
		NewIDFn: func(ctx context.Context) (int, error) {
			// only example, don't do this in production code.
			return rand.Int(), nil
		},
		ToArgsFn: func(ent *Entity) ([]interface{}, error) {
			return []interface{}{ent.ID, ent.Value}, nil
		},
		MapFn: func(s iterators.SQLRowScanner) (Entity, error) {
			var ent Entity
			return ent, s.Scan(&ent.ID, &ent.Value)
		},
	}

	stg := postgresql.NewRepositoryByDSN[Entity, int](mapping, os.Getenv("DATABASE_URL"))
	defer stg.Close()
}
