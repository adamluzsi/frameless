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

		QueryID: func(id int) (flsql.QueryArgs, error) {
			return flsql.QueryArgs{"id": id}, nil
		},

		ToArgs: func(e Entity) (flsql.QueryArgs, error) {
			return flsql.QueryArgs{
				`id`:    e.ID,
				`value`: e.Value,
			}, nil
		},

		ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[Entity]) {
			return []flsql.ColumnName{"id", "value"},
				func(v *Entity, s flsql.Scanner) error {
					return s.Scan(&v.ID, &v.Value)
				}
		},

		ID: func(e *Entity) *int {
			return &e.ID
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
