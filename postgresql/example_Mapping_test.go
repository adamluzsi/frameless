package postgresql_test

import (
	"context"
	"time"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/postgresql"
)

func ExampleMapper() {
	type ExampleEntity struct {
		ID   int64
		Col1 int
		Col2 string
		Col3 bool
	}
	_ = postgresql.Mapper /* [ExampleEntity] */ {
		Table:   `"public"."entities"`,
		ID:      "entity_id",
		Columns: []string{"entity_id", "col1", "col2", "col3"},
		NewIDFn: func(ctx context.Context) (interface{}, error) {
			// a really bad way to make id,
			// but this is only an example
			return time.Now().UnixNano(), nil
		},
		ToArgsFn: func(ptr interface{}) ([]interface{}, error) {
			ent := ptr.(*ExampleEntity) // Go1.18 will solve this with generics
			return []interface{}{ent.ID, ent.Col1, ent.Col2, ent.Col3}, nil
		},
		MapFn: func(scanner iterators.SQLRowScanner, ptr interface{}) error {
			ent := ptr.(*ExampleEntity)
			return scanner.Scan(&ent.ID, &ent.Col1, &ent.Col2, &ent.Col3)
		},
	}
}
