package postgresql_test

import (
	"context"
	"os"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/flsql"
)

func ExampleRepository() {
	type Entity struct {
		ID    int `ext:"ID"`
		Value string
	}

	mapping := flsql.Mapping[Entity, int]{
		TableName: "entities",

		ToID: func(id int) (map[flsql.ColumnName]any, error) {
			return map[flsql.ColumnName]any{"id": id}, nil
		},

		ToArgs: func(e Entity) (map[flsql.ColumnName]any, error) {
			return map[flsql.ColumnName]any{
				`id`:    e.ID,
				`value`: e.Value,
			}, nil
		},

		ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[Entity]) {
			return []flsql.ColumnName{"id", "value"},
				func(v *Entity, scan flsql.ScanFunc) error {
					return scan(&v.ID, &v.Value)
				}
		},

		GetID: func(e Entity) int {
			return e.ID
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
