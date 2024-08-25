package flsql_test

import (
	"context"
	"errors"
	"testing"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/spechelper/testent"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func TestMakeErrSQLRow(t *testing.T) {
	expErr := rnd.Error()
	row := flsql.MakeErrSQLRow(expErr)
	assert.NotNil(t, row)
	assert.ErrorIs(t, expErr, row.Scan())
	assert.ErrorIs(t, expErr, row.Err())
}

func ExampleMapping() {
	type ExampleEntity struct {
		ID   int64
		Col1 int
		Col2 string
		Col3 bool
	}
	_ = flsql.Mapping[ExampleEntity, int64]{
		TableName: `"public"."entities"`,

		ToID: func(id int64) (map[flsql.ColumnName]any, error) {
			return map[flsql.ColumnName]any{"entity_id": id}, nil
		},

		ToArgs: func(ee ExampleEntity) (map[flsql.ColumnName]any, error) {
			return map[flsql.ColumnName]any{
				"entity_id": ee.ID,
				"col1":      ee.Col1,
				"col2":      ee.Col2,
				"col3":      ee.Col3,
			}, nil
		},

		ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[ExampleEntity]) {
			return []flsql.ColumnName{"entity_id", "col1", "col2", "col3"},
				func(ent *ExampleEntity, scan flsql.ScanFunc) error {
					return scan(&ent.ID, &ent.Col1, &ent.Col2, &ent.Col3)
				}
		},
	}
}

func TestMapper_ToQuery(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	type X struct{ Foo int }

	ctx := context.Background()

	m := flsql.Mapping[X, int]{ToQuery: func(ctx context.Context) ([]flsql.ColumnName, flsql.MapScan[X]) {
		return []flsql.ColumnName{"foo"}, func(v *X, scan flsql.ScanFunc) error {
			return scan(&v.Foo)
		}
	}}

	t.Run(`happy-path`, func(t *testing.T) {
		expectedInt := rnd.Int()
		scanner := FakeSQLRowScanner{ScanFunc: func(i ...interface{}) error {
			return reflectkit.Link(expectedInt, i[0])
		}}

		_, mscan := m.ToQuery(ctx)
		x, err := mscan.Map(scanner)
		assert.Nil(t, err)
		assert.Equal(t, expectedInt, x.Foo)
	})

	t.Run(`rainy-path`, func(t *testing.T) {
		var expectedErr = errors.New(`boom`)
		scanner := FakeSQLRowScanner{ScanFunc: func(i ...interface{}) error {
			return expectedErr
		}}

		_, mscan := m.ToQuery(ctx)
		_, err := mscan.Map(scanner)
		assert.Equal(t, expectedErr, err)
	})
}

type FakeSQLRowScanner struct {
	ScanFunc func(...interface{}) error
}

func (scanner FakeSQLRowScanner) Scan(i ...interface{}) error {
	return scanner.ScanFunc(i...)
}

func TestMapper_CreatePrepare(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run(`provided`, func(t *testing.T) {
		expID := testent.FooID(rnd.String())
		expErr := rnd.Error()

		m := flsql.Mapping[testent.Foo, testent.FooID]{
			CreatePrepare: func(ctx context.Context, a *testent.Foo) error {
				a.ID = expID
				return expErr
			},
		}

		var ent testent.Foo
		assert.ErrorIs(t, expErr, m.OnCreate(context.Background(), &ent))
		assert.Equal(t, expID, ent.ID)
	})

	t.Run(`absent`, func(t *testing.T) {
		m := flsql.Mapping[testent.Foo, testent.FooID]{}
		var ent testent.Foo
		assert.NoError(t, m.OnCreate(context.Background(), &ent))
	})
}
