package flsql_test

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"iter"
	"testing"

	"go.llib.dev/frameless/pkg/flsql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit"

	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
	"go.llib.dev/testcase/random"
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

	userIDIter := flsql.MakeRowsIterator(userIDs, func(scanner flsql.Scanner) (mytype, error) {
		var value mytype
		if err := scanner.Scan(&value.asdf); err != nil {
			return mytype{}, err
		}
		return value, nil
	})

	for id, err := range userIDIter {
		if err != nil {
			panic(err)
		}
		_ = id
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

	var (
		rows   = let.Var[flsql.Rows](s, nil)
		mapper = let.Var(s, func(t *testcase.T) flsql.RowMapper[testType] {
			return func(s flsql.Scanner) (testType, error) {
				var v testType
				return v, s.Scan(&v.Text)
			}
		})
	)
	act := let.Act(func(t *testcase.T) iterkit.ErrSeq[testType] {
		return flsql.MakeRowsIterator(rows.Get(t), mapper.Get(t))
	})

	s.Context(`has no values`, func(s *testcase.Spec) {
		rows.Let(s, func(t *testcase.T) flsql.Rows {
			return NewSQLRowsStubFromIter(t, iterkit.Empty2[[]any, error]())
		})

		s.Then("it will be an empty iterator", func(t *testcase.T) {
			itr := act(t)
			vs, err := iterkit.CollectErr(itr)
			assert.NoError(t, err)
			assert.Empty(t, vs)
		})
	})

	s.Context(`has value(s)`, func(s *testcase.Spec) {
		stub := let.Var(s, func(t *testcase.T) *SQLRowsStub {
			return NewSQLRowsStubFromIter(t, iterkit.ToErrSeq(iterkit.Slice([][]any{[]any{`42`}})))
		})

		rows.Let(s, func(t *testcase.T) flsql.Rows {
			return stub.Get(t)
		})

		s.Then(`it will fetch the values`, func(t *testcase.T) {
			itr := act(t)

			vs, err := iterkit.CollectErr(itr)
			assert.NoError(t, err)
			assert.Equal(t, []testType{testType{Text: `42`}}, vs)
		})

		s.And(`error happen during scanning`, func(s *testcase.Spec) {
			expectedErr := let.Error(s)

			stub.Let(s, func(t *testcase.T) *SQLRowsStub {
				stb := stub.Super(t)
				stb.ScanErr = expectedErr.Get(t)
				return stb
			})

			s.Then(`it will be propagated during decode`, func(t *testcase.T) {
				_, err := iterkit.CollectErr(act(t))

				assert.ErrorIs(t, err, expectedErr.Get(t))
			})
		})

	})

	s.When(`close encounter error`, func(s *testcase.Spec) {
		expectedErr := let.Error(s)

		rows.Let(s, func(t *testcase.T) flsql.Rows {
			stub := NewSQLRowsStubFromIter(t, iterkit.ToErrSeq(iterkit.Empty[[]any]()))
			stub.CloseErr = expectedErr.Get(t)
			return stub
		})

		s.Then(`it will be propagated during iterator closing`, func(t *testcase.T) {
			_, err := iterkit.CollectErr(act(t))
			assert.ErrorIs(t, err, expectedErr.Get(t))
		})
	})

}

func NewSQLRowsStubFromIter(tb testing.TB, i iter.Seq2[[]any, error]) *SQLRowsStub {
	next, stop := iter.Pull2(i)
	tb.Cleanup(stop)
	return &SQLRowsStub{
		NextFunc: next,
		StopFunc: stop,
	}
}

type SQLRowsStub struct {
	NextFunc func() ([]any, error, bool)
	StopFunc func()
	CloseErr error
	ScanErr  error

	args []any
	err  error
}

func (s *SQLRowsStub) Next() bool {
	if s.NextFunc == nil {
		return false
	}
	v, err, ok := s.NextFunc()
	if err != nil {
		s.err = err
	}
	if ok {
		s.args = v
	}
	return ok
}

func (s *SQLRowsStub) Err() error {
	return s.err
}

func (s *SQLRowsStub) Close() error {
	if s.StopFunc != nil {
		s.StopFunc()
	}
	return s.CloseErr
}

func (s *SQLRowsStub) Scan(dest ...interface{}) error {
	if s.ScanErr != nil {
		return s.ScanErr
	}
	if len(s.args) != len(dest) {
		return fmt.Errorf("scan argument count mismatch")
	}
	for i, dst := range dest {
		if err := reflectkit.Link(s.args[i], dst); err != nil {
			return err
		}
	}
	return nil
}

func TestQueryMany(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		expQuery := fmt.Sprintf("SELECT id FROM table as %s WHERE x = ? AND y = ?", t.Random.StringNC(2, random.CharsetAlpha()))
		expArgs := []any{t.Random.Int(), t.Random.Int()}

		var c flsql.Queryable = flsql.QueryableAdapter{
			QueryFunc: func(ctx context.Context, query string, args ...any) (flsql.Rows, error) {
				assert.Equal(t, query, expQuery)
				assert.Equal(t, args, expArgs)
				itr := iterkit.ToErrSeq(iterkit.Slice([][]any{[]any{42}}))
				return NewSQLRowsStubFromIter(t, itr), nil
			},
		}

		type T struct {
			ID int
		}
		var m flsql.RowMapper[T] = func(s flsql.Scanner) (T, error) {
			var v T
			err := s.Scan(&v.ID)
			return v, err
		}

		res, err := flsql.QueryMany(c, t.Context(), m, expQuery, expArgs...)
		assert.NoError(t, err)
		vs, err := iterkit.CollectErr(res)
		assert.NoError(t, err)
		assert.ContainExactly(t, vs, []T{{ID: 42}})
	})

	s.Test("rainy", func(t *testcase.T) {
		expErr := t.Random.Error()

		var c flsql.Queryable = flsql.QueryableAdapter{
			QueryFunc: func(ctx context.Context, query string, args ...any) (flsql.Rows, error) {
				return &SQLRowsStub{}, expErr
			},
		}

		type T struct {
			ID int
		}
		var m flsql.RowMapper[T] = func(s flsql.Scanner) (T, error) {
			var v T
			err := s.Scan(&v.ID)
			return v, err
		}

		query := fmt.Sprintf("SELECT id FROM table as %s WHERE x = ? AND y = ?", t.Random.StringNC(2, random.CharsetAlpha()))
		args := []any{t.Random.Int(), t.Random.Int()}
		res, err := flsql.QueryMany(c, t.Context(), m, query, args...)
		assert.ErrorIs(t, err, expErr)
		assert.Nil(t, res)
	})
}
