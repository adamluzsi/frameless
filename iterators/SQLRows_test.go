package iterators_test

//go:generate mockgen -destination SQLRows_mocks_test.go -source SQLRows.go -package iterators_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/golang/mock/gomock"
)

func ExampleNewSQLRows(ctx context.Context, db *sql.DB) error {
	userIDs, err := db.QueryContext(ctx, `SELECT id FROM users`)

	if err != nil {
		return err
	}

	type mytype struct {
		asdf string
	}

	iter := iterators.NewSQLRows[mytype](userIDs, iterators.SQLRowMapperFunc[mytype](func(scanner iterators.SQLRowScanner) (mytype, error) {
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
	return iter.Err()
}

func TestSQLRows(t *testing.T) {
	type testType struct{ Text string }

	s := testcase.NewSpec(t)

	rows := testcase.Var[iterators.SQLRows]{ID: "iterators.SQLRows"}
	mapper := testcase.Var[iterators.SQLRowMapper[testType]]{ID: "iterators.SQLRowMapper"}
	subject := func(t *testcase.T) frameless.Iterator[testType] {
		return iterators.NewSQLRows(rows.Get(t), mapper.Get(t))
	}

	rowsmockctrl := testcase.Let(s, func(t *testcase.T) *gomock.Controller {
		c := gomock.NewController(t.TB)
		t.Defer(c.Finish)
		return c
	})

	mapper.Let(s, func(t *testcase.T) iterators.SQLRowMapper[testType] {
		return iterators.SQLRowMapperFunc[testType](func(s iterators.SQLRowScanner) (testType, error) {
			var v testType
			return v, s.Scan(&v.Text)
		})
	})

	s.When(`rows`, func(s *testcase.Spec) {
		s.Context(`has no values`, func(s *testcase.Spec) {
			rows.Let(s, func(t *testcase.T) iterators.SQLRows {
				mock := NewMockSQLRows(rowsmockctrl.Get(t))
				mock.EXPECT().Next().Return(false).AnyTimes()
				mock.EXPECT().Err().Return(nil).AnyTimes()
				mock.EXPECT().Close().Return(nil).AnyTimes()
				return mock
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
			rows.Let(s, func(t *testcase.T) iterators.SQLRows {
				mock := NewMockSQLRows(rowsmockctrl.Get(t))

				value := &testType{Text: `42`}

				mock.EXPECT().Next().DoAndReturn(func() bool {
					return value != nil
				}).AnyTimes()

				mock.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...interface{}) error {
					assert.Must(t).Equal(1, len(dest))
					*(dest[0].(*string)) = value.Text
					value = nil
					return nil
				})

				mock.EXPECT().Err().Return(nil)
				mock.EXPECT().Close().Return(nil)
				return mock
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
				rows.Let(s, func(t *testcase.T) iterators.SQLRows {
					mock := NewMockSQLRows(rowsmockctrl.Get(t))
					mock.EXPECT().Next().Return(true)
					mock.EXPECT().Err().Return(nil).AnyTimes()
					mock.EXPECT().Close().Return(nil)
					mock.EXPECT().Scan(gomock.Any()).Return(expectedErr)
					return mock
				})

				s.Then(`it will be propagated during decode`, func(t *testcase.T) {
					iter := subject(t)
					defer iter.Close()
					assert.Must(t).False(iter.Next())
					assert.Must(t).Equal(expectedErr, iter.Err())
				})
			})

		})
	})

	s.When(`close encounter error`, func(s *testcase.Spec) {
		rows.Let(s, func(t *testcase.T) iterators.SQLRows {
			mock := NewMockSQLRows(rowsmockctrl.Get(t))
			mock.EXPECT().Close().Return(errors.New(`boom`))
			return mock
		})

		s.Then(`it will be propagated during iterator closing`, func(t *testcase.T) {
			iter := subject(t)
			err := iter.Close()
			assert.Must(t).NotNil(err)
			assert.Must(t).Equal(`boom`, err.Error())
		})
	})

}
