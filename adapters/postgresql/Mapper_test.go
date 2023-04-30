package postgresql_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless/adapters/postgresql"
	"github.com/adamluzsi/testcase/random"

	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"github.com/adamluzsi/frameless/ports/iterators"
	"github.com/adamluzsi/testcase/assert"
)

func TestMapper_Map(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	type X struct {
		Foo int
	}

	m := postgresql.Mapper[X, int]{MapFn: func(s iterators.SQLRowScanner) (X, error) {
		var x X
		return x, s.Scan(&x.Foo)
	}}

	t.Run(`happy-path`, func(t *testing.T) {
		expectedInt := rnd.Int()
		scanner := FakeSQLRowScanner{ScanFunc: func(i ...interface{}) error {
			return reflectkit.Link(expectedInt, i[0])
		}}

		x, err := m.Map(scanner)
		assert.Nil(t, err)
		assert.Equal(t, expectedInt, x.Foo)
	})

	t.Run(`rainy-path`, func(t *testing.T) {
		var expectedErr = errors.New(`boom`)
		scanner := FakeSQLRowScanner{ScanFunc: func(i ...interface{}) error {
			return expectedErr
		}}

		_, err := m.Map(scanner)
		assert.Equal(t, expectedErr, err)
	})
}

type FakeSQLRowScanner struct {
	ScanFunc func(...interface{}) error
}

func (scanner FakeSQLRowScanner) Scan(i ...interface{}) error {
	return scanner.ScanFunc(i...)
}

func TestMapper_ToArgs(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	type X struct {
		Foo int64
	}

	t.Run(`happy-path`, func(t *testing.T) {
		m := postgresql.Mapper[X, int64]{ToArgsFn: func(ptr *X) ([]interface{}, error) {
			return []interface{}{sql.NullInt64{Int64: ptr.Foo, Valid: true}}, nil
		}}

		x := X{Foo: int64(rnd.Int())}

		args, err := m.ToArgsFn(&x)
		assert.Nil(t, err)

		assert.Equal(t, []interface{}{sql.NullInt64{
			Int64: x.Foo,
			Valid: true,
		}}, args)
	})

	t.Run(`rainy-path`, func(t *testing.T) {
		expectedErr := errors.New(`boom`)
		m := postgresql.Mapper[X, int64]{ToArgsFn: func(ptr *X) ([]interface{}, error) {
			return nil, expectedErr
		}}

		_, err := m.ToArgsFn(&X{Foo: int64(rnd.Int())})
		assert.Equal(t, expectedErr, err)
	})
}

func TestMapper_NewID(t *testing.T) {
	rnd := random.New(random.CryptoSeed{})
	t.Run(`happy-path`, func(t *testing.T) {
		expectedID := rnd.Int()
		m := postgresql.Mapper[any, int]{NewIDFn: func(ctx context.Context) (int, error) {
			return expectedID, nil
		}}

		actualID, err := m.NewID(context.Background())
		assert.NoError(t, err)

		assert.Equal(t, expectedID, actualID)
	})

	t.Run(`rainy-path`, func(t *testing.T) {
		expectedErr := errors.New(`boom`)
		m := postgresql.Mapper[any, any]{NewIDFn: func(ctx context.Context) (any, error) {
			return nil, expectedErr
		}}

		_, err := m.NewID(context.Background())
		assert.Equal(t, expectedErr, err)
	})
}
