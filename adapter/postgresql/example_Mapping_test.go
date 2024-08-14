package postgresql_test

import (
	"context"
	"time"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/port/iterators"
)

func ExampleMapping() {
	type ExampleEntity struct {
		ID   int64
		Col1 int
		Col2 string
		Col3 bool
	}
	_ = postgresql.Mapping[ExampleEntity, int64]{
		Table:   `"public"."entities"`,
		ID:      "entity_id",
		Columns: []string{"entity_id", "col1", "col2", "col3"},
		NewIDFn: func(ctx context.Context) (int64, error) {
			// a really bad way to make id,
			// but this is only an example
			return time.Now().UnixNano(), nil
		},
		ToArgsFn: func(ent *ExampleEntity) ([]interface{}, error) {
			return []interface{}{ent.ID, ent.Col1, ent.Col2, ent.Col3}, nil
		},
		MapFn: func(scanner iterators.SQLRowScanner) (ExampleEntity, error) {
			var ent ExampleEntity
			return ent, scanner.Scan(&ent.ID, &ent.Col1, &ent.Col2, &ent.Col3)
		},
	}
}
