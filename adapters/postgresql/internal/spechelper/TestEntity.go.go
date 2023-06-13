package spechelper

import (
	"context"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

type TestEntity struct {
	ID  string `ext:"ID"`
	Foo string
	Bar string
	Baz string
}

func MakeTestEntityFunc(tb testing.TB) func() TestEntity {
	return func() TestEntity {
		te := tb.(*testcase.T).Random.Make(TestEntity{}).(TestEntity)
		te.ID = ""
		return te
	}
}

type TestEntityDTO struct {
	ID  string `ext:"ID" json:"id"`
	Foo string `json:"foo"`
	Bar string `json:"bar"`
	Baz string `json:"baz"`
}

type TestEntityJSONMapping struct{}

func (n TestEntityJSONMapping) ToDTO(ent TestEntity) (TestEntityDTO, error) {
	return TestEntityDTO{ID: ent.ID, Foo: ent.Foo, Bar: ent.Bar, Baz: ent.Baz}, nil
}

func (n TestEntityJSONMapping) ToEnt(dto TestEntityDTO) (TestEntity, error) {
	return TestEntity{ID: dto.ID, Foo: dto.Foo, Bar: dto.Bar, Baz: dto.Baz}, nil
}

func TestEntityMapping() postgresql.Mapper[TestEntity, string] {
	var counter int
	return postgresql.Mapper[TestEntity, string]{
		Table: "test_entities",
		ID:    "id",
		NewIDFn: func(ctx context.Context) (string, error) {
			counter++
			rnd := random.New(random.CryptoSeed{})
			rndstr := rnd.StringNWithCharset(8, "ABCDEFGHIJKLMNOPQRSTUVWXYZ")
			return fmt.Sprintf("%d-%s", counter, rndstr), nil
		},
		Columns: []string{`id`, `foo`, `bar`, `baz`},
		ToArgsFn: func(ent *TestEntity) ([]interface{}, error) {
			return []interface{}{ent.ID, ent.Foo, ent.Bar, ent.Baz}, nil
		},
		MapFn: func(s iterators.SQLRowScanner) (TestEntity, error) {
			var ent TestEntity
			return ent, s.Scan(&ent.ID, &ent.Foo, &ent.Bar, &ent.Baz)
		},
	}
}

func MigrateTestEntity(tb testing.TB, cm postgresql.Connection) {
	ctx := context.Background()
	_, err := cm.ExecContext(ctx, testMigrateDOWN)
	assert.Nil(tb, err)
	_, err = cm.ExecContext(ctx, testMigrateUP)
	assert.Nil(tb, err)

	tb.Cleanup(func() {
		_, err := cm.ExecContext(ctx, testMigrateDOWN)
		assert.Nil(tb, err)
	})
}

const testMigrateUP = `
CREATE TABLE "test_entities" (
    id	TEXT	NOT	NULL	PRIMARY KEY,
	foo	TEXT	NOT	NULL,
	bar	TEXT	NOT	NULL,
	baz	TEXT	NOT	NULL
);
`

const testMigrateDOWN = `
DROP TABLE IF EXISTS "test_entities";
`
