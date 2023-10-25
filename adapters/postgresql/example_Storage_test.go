package postgresql_test

import (
	"context"
	"math/rand"
	"os"

	"go.llib.dev/frameless/adapters/postgresql"
	"go.llib.dev/frameless/ports/iterators"
)

func ExampleRepository() {
	type Entity struct {
		ID    int `ext:"ID"`
		Value string
	}

	mapping := postgresql.Mapping[Entity, int]{
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

	cm, err := postgresql.Connect(os.Getenv("DATABASE_URL"))
	if err != nil {
		panic(err)
	}
	defer cm.Close()

	repo := postgresql.Repository[Entity, int]{
		Connection: cm,
		Mapping:    mapping,
	}

	_ = repo
}
