package flsql_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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

func TestJSON(t *testing.T) {
	t.Run("marshal and unmarshal", func(t *testing.T) {
		type testStruct struct {
			Foo string
		}
		ptr := &testStruct{Foo: "bar"}
		dto := flsql.JSON(ptr)

		val, err := dto.Value()
		assert.NoError(t, err)
		data, ok := val.([]byte)
		assert.True(t, ok, "expeted to receive []byte from .Value")
		assert.True(t, json.Valid(data))

		var actual testStruct
		assert.NoError(t, flsql.JSON(&actual).Scan(data))
		assert.Equal(t, ptr.Foo, actual.Foo)
	})

	t.Run("unmarshal invalid type", func(t *testing.T) {
		var val string
		dto := flsql.JSON(&val)
		err := dto.Scan(123)
		assert.Error(t, err)
	})

	t.Run("nil pointer", func(t *testing.T) {
		val, err := flsql.JSON[int](nil).Value()
		assert.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("pointer to a nil value", func(t *testing.T) {
		type Fooer interface{ Foo() }
		var in Fooer
		dto := flsql.JSON(&in)
		val, err := dto.Value()
		assert.NoError(t, err)
		data, ok := val.([]byte)
		assert.True(t, ok)
		assert.Equal(t, []byte("null"), data)
	})
}

func TestTimestamp(t *testing.T) {
	layout := "2006-01-02 15:04:05"
	tz := time.UTC

	t.Run("scan and value", func(t *testing.T) {
		var timestamp time.Time
		dto := flsql.Timestamp(&timestamp, layout, tz)
		val := []byte("2022-07-25 14:30:00")

		assert.NoError(t, dto.Scan(val))
		assert.Equal(t, "2022-07-25 14:30:00", timestamp.Format(layout))

		outVal, err := dto.Value()
		assert.NoError(t, err)
		outStr, ok := outVal.(string)
		assert.True(t, ok)
		assert.Equal(t, val, []byte(outStr))
	})

	t.Run("scan invalid layout", func(t *testing.T) {
		assert.Error(t, flsql.Timestamp(&time.Time{}, "invalid-layout", tz).Scan("2022-07-25 14:30:00"))
	})

	t.Run("value on invalid layout", func(t *testing.T) {
		_, err := flsql.Timestamp(&time.Time{}, "invalid-layout", tz).Value()
		assert.Error(t, err)
	})

	t.Run("scan unsupported type", func(t *testing.T) {
		assert.Error(t, flsql.Timestamp(&time.Time{}, layout, tz).Scan(123))
	})

	t.Run("value with nil pointer", func(t *testing.T) {
		val, err := flsql.Timestamp((*time.Time)(nil), layout, tz).Value()
		assert.NoError(t, err)
		assert.Nil(t, val)
	})

	t.Run("scan with nil value", func(t *testing.T) {
		var timestamp time.Time
		dto := flsql.Timestamp(&timestamp, layout, tz)
		err := dto.Scan(nil)
		assert.NoError(t, err)
		assert.True(t, timestamp.IsZero())
	})
}