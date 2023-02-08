package iterators_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/iterators"

	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

func ExampleSQLRows() {
	var (
		ctx context.Context
		db  *sql.DB
	)
	userIDs, err := db.QueryContext(ctx, `SELECT id FROM users`)

	if err != nil {
		panic(err)
	}

	type mytype struct {
		asdf string
	}

	iter := iterators.SQLRows[mytype](userIDs, iterators.SQLRowMapperFunc[mytype](func(scanner iterators.SQLRowScanner) (mytype, error) {
		var value mytype
		if err := scanner.Scan(&value.asdf); err != nil {
			return mytype{}, err
		}
		return value, nil
	}))

	defer iter.Close()
	for iter.Next() {
		v := iter.Value()
		_ = v
	}
	if err := iter.Err(); err != nil {
		panic(err)
	}
}

type SQLRows interface {
	io.Closer
	Next() bool
	Err() error
	Scan(dest ...interface{}) error
}

func TestSQLRows(t *testing.T) {
	type testType struct{ Text string }

	s := testcase.NewSpec(t)

	rows := testcase.Var[SQLRows]{ID: "iterators.SQLRows"}
	mapper := testcase.Var[iterators.SQLRowMapper[testType]]{ID: "iterators.SQLRowMapper"}
	subject := func(t *testcase.T) iterators.Iterator[testType] {
		return iterators.SQLRows(rows.Get(t), mapper.Get(t))
	}
	mapper.Let(s, func(t *testcase.T) iterators.SQLRowMapper[testType] {
		return iterators.SQLRowMapperFunc[testType](func(s iterators.SQLRowScanner) (testType, error) {
			var v testType
			return v, s.Scan(&v.Text)
		})
	})

	s.When(`rows`, func(s *testcase.Spec) {
		s.Context(`has no values`, func(s *testcase.Spec) {
			rows.Let(s, func(t *testcase.T) SQLRows {
				return &SQLRowsStub{
					Iterator: iterators.Empty[[]any](),
				}
			})

			s.Then(`it will false to next`, func(t *testcase.T) {
				iter := subject(t)
				defer iter.Close()
				assert.Must(t).False(iter.Next())
			})

			s.Then(`it will result in no error`, func(t *testcase.T) {
				iter := subject(t)
				defer iter.Close()
				assert.Must(t).False(iter.Next())
				assert.Must(t).Nil(iter.Err())
			})

			s.Then(`it will be closeable`, func(t *testcase.T) {
				iter := subject(t)
				assert.Must(t).Nil(iter.Close())
			})
		})

		s.Context(`has value(s)`, func(s *testcase.Spec) {
			rows.Let(s, func(t *testcase.T) SQLRows {
				return &SQLRowsStub{
					Iterator: iterators.Slice([][]any{[]any{`42`}}),
				}
			})

			s.Then(`it will decode values into the passed ptr`, func(t *testcase.T) {
				iter := subject(t)

				var value testType

				assert.Must(t).True(iter.Next())
				value = iter.Value()
				assert.Must(t).Equal(testType{Text: `42`}, value)
				assert.Must(t).False(iter.Next())
				assert.Must(t).Nil(iter.Err())
				assert.Must(t).Nil(iter.Close())
			})

			s.And(`error happen during scanning`, func(s *testcase.Spec) {
				expectedErr := errors.New(`boom`)
				rows.Let(s, func(t *testcase.T) SQLRows {
					return &SQLRowsStub{
						Iterator: iterators.Slice[[]any]([][]any{{`42`}}),
						ScanErr:  expectedErr,
					}
				})

				s.Then(`it will be propagated during decode`, func(t *testcase.T) {
					iter := subject(t)
					defer iter.Close()
					t.Must.False(iter.Next())
					t.Must.ErrorIs(expectedErr, iter.Err())
				})
			})

		})
	})

	s.When(`close encounter error`, func(s *testcase.Spec) {
		expectedErr := errors.New(`boom`)
		rows.Let(s, func(t *testcase.T) SQLRows {
			return &SQLRowsStub{
				Iterator: iterators.Empty[[]any](),
				CloseErr: expectedErr,
			}
		})

		s.Then(`it will be propagated during iterator closing`, func(t *testcase.T) {
			t.Must.ErrorIs(expectedErr, subject(t).Close())
		})
	})

}

type SQLRowsStub struct {
	iterators.Iterator[[]any]
	CloseErr error
	ScanErr  error
}

func (s *SQLRowsStub) Close() error {
	return s.CloseErr
}

func (s *SQLRowsStub) Scan(dest ...interface{}) error {
	if s.ScanErr != nil {
		return s.ScanErr
	}
	args := s.Iterator.Value()
	if len(args) != len(dest) {
		return fmt.Errorf("scan argument count mismatch")
	}
	for i, dst := range dest {
		if err := reflects.Link(args[i], dst); err != nil {
			return err
		}
	}
	return nil
}
