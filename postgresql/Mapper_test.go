package postgresql_test

import (
	"context"
	"database/sql"
	"errors"
	"github.com/adamluzsi/frameless/postgresql"
	"testing"

	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

func TestMapper_Map(t *testing.T) {
	type X struct {
		Foo int
	}

	m := postgresql.Mapper{MapFn: func(s iterators.SQLRowScanner, ptr interface{}) error {
		x := ptr.(*X)
		return s.Scan(&x.Foo)
	}}

	t.Run(`happy-path`, func(t *testing.T) {
		expectedInt := fixtures.Random.Int()
		scanner := FakeSQLRowScanner{ScanFunc: func(i ...interface{}) error {
			return reflects.Link(expectedInt, i[0])
		}}

		var x X
		require.Nil(t, m.Map(scanner, &x))
		require.Equal(t, expectedInt, x.Foo)
	})

	t.Run(`rainy-path`, func(t *testing.T) {
		var expectedErr = errors.New(`boom`)
		scanner := FakeSQLRowScanner{ScanFunc: func(i ...interface{}) error {
			return expectedErr
		}}

		require.Equal(t, expectedErr, m.Map(scanner, &X{}))
	})
}

type FakeSQLRowScanner struct {
	ScanFunc func(...interface{}) error
}

func (scanner FakeSQLRowScanner) Scan(i ...interface{}) error {
	return scanner.ScanFunc(i...)
}

func TestMapper_ToArgs(t *testing.T) {
	type X struct {
		Foo int64
	}

	t.Run(`happy-path`, func(t *testing.T) {
		m := postgresql.Mapper{ToArgsFn: func(ptr interface{}) ([]interface{}, error) {
			x := ptr.(*X)
			return []interface{}{sql.NullInt64{Int64: x.Foo, Valid: true}}, nil
		}}

		x := X{Foo: int64(fixtures.Random.Int())}

		args, err := m.ToArgsFn(&x)
		require.Nil(t, err)

		require.Equal(t, []interface{}{sql.NullInt64{
			Int64: x.Foo,
			Valid: true,
		}}, args)
	})

	t.Run(`rainy-path`, func(t *testing.T) {
		expectedErr := errors.New(`boom`)
		m := postgresql.Mapper{ToArgsFn: func(ptr interface{}) ([]interface{}, error) {
			return nil, expectedErr
		}}

		_, err := m.ToArgsFn(&X{Foo: int64(fixtures.Random.Int())})
		require.Equal(t, expectedErr, err)
	})
}

func TestMapper_NewID(t *testing.T) {
	t.Run(`happy-path`, func(t *testing.T) {
		expectedID := fixtures.Random.Int()
		m := postgresql.Mapper{NewIDFn: func(ctx context.Context) (interface{}, error) {
			return expectedID, nil
		}}

		actualID, err := m.NewID(context.Background())
		require.NoError(t, err)

		require.Equal(t, expectedID, actualID)
	})

	t.Run(`rainy-path`, func(t *testing.T) {
		expectedErr := errors.New(`boom`)
		m := postgresql.Mapper{NewIDFn: func(ctx context.Context) (interface{}, error) {
			return nil, expectedErr
		}}

		_, err := m.NewID(context.Background())
		require.Equal(t, expectedErr, err)
	})
}
