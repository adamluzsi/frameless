package iterators_test

//go:generate mockgen -destination SQLRows_mocks_test.go -source SQLRows.go -package iterators_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/testcase"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
)

func ExampleNewSQLRows(ctx context.Context, db *sql.DB) error {
	userIDs, err := db.QueryContext(ctx, `SELECT id FROM users`)

	if err != nil {
		return err
	}

	type mytype struct {
		asdf string
	}

	iter := iterators.NewSQLRows(userIDs, iterators.SQLRowMapperFunc(func(scanner iterators.SQLRowScanner, entity interface{}) error {
		value := entity.(*mytype)
		return scanner.Scan(&value.asdf)
	}))

	defer iter.Close()

	for iter.Next() {
		var t mytype
		if err := iter.Decode(&t); err != nil {
			return err
		}
		fmt.Println(t)
	}

	return iter.Err()
}

func TestSQLRows(t *testing.T) {
	s := testcase.NewSpec(t)
	subject := func(t *testcase.T) iterators.Interface {
		return iterators.NewSQLRows(t.I(`rows`).(iterators.SQLRows),
			t.I(`mapper`).(iterators.SQLRowMapper))
	}

	type testType struct{ Text string }

	s.Let(`rows.mock.ctrl`, func(t *testcase.T) interface{} { return gomock.NewController(t.TB) })
	s.After(func(t *testcase.T) { t.I(`rows.mock.ctrl`).(*gomock.Controller).Finish() })

	s.Let(`mapper`, func(t *testcase.T) interface{} {
		return iterators.SQLRowMapperFunc(func(s iterators.SQLRowScanner, e interface{}) error {
			ptr := e.(*testType)
			return s.Scan(&ptr.Text)
		})
	})

	s.When(`rows`, func(s *testcase.Spec) {
		s.Context(`has no values`, func(s *testcase.Spec) {
			s.Let(`rows`, func(t *testcase.T) interface{} {
				mock := NewMockSQLRows(t.I(`rows.mock.ctrl`).(*gomock.Controller))
				mock.EXPECT().Next().Return(false).AnyTimes()
				mock.EXPECT().Err().Return(nil).AnyTimes()
				mock.EXPECT().Close().Return(nil).AnyTimes()
				return mock
			})

			s.Then(`it will false to next`, func(t *testcase.T) {
				iter := subject(t)
				defer iter.Close()
				require.False(t, iter.Next())
			})

			s.Then(`it will result in no error`, func(t *testcase.T) {
				iter := subject(t)
				defer iter.Close()
				require.False(t, iter.Next())
				require.Nil(t, iter.Err())
			})

			s.Then(`it will be closeable`, func(t *testcase.T) {
				iter := subject(t)
				require.Nil(t, iter.Close())
			})
		})

		s.Context(`has value(s)`, func(s *testcase.Spec) {
			s.Let(`rows`, func(t *testcase.T) interface{} {
				mock := NewMockSQLRows(t.I(`rows.mock.ctrl`).(*gomock.Controller))

				value := &testType{Text: `42`}

				mock.EXPECT().Next().DoAndReturn(func() bool {
					return value != nil
				}).AnyTimes()

				mock.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...interface{}) error {
					require.Equal(t, 1, len(dest))
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

				require.True(t, iter.Next())
				require.Nil(t, iter.Decode(&value))
				require.Equal(t, testType{Text: `42`}, value)
				require.False(t, iter.Next())
				require.Nil(t, iter.Err())
				require.Nil(t, iter.Close())
			})

			s.And(`error happen during scanning`, func(s *testcase.Spec) {
				s.Let(`rows`, func(t *testcase.T) interface{} {
					mock := NewMockSQLRows(t.I(`rows.mock.ctrl`).(*gomock.Controller))
					mock.EXPECT().Next().Return(true)
					mock.EXPECT().Err().Return(nil)
					mock.EXPECT().Close().Return(nil)
					mock.EXPECT().Scan(gomock.Any()).Return(errors.New(`boom`))
					return mock
				})

				s.Then(`it will be propagated during decode`, func(t *testcase.T) {
					iter := subject(t)
					defer iter.Close()
					require.True(t, iter.Next())
					var value testType
					err := iter.Decode(&value)
					require.Error(t, err)
					require.Equal(t, `boom`, err.Error())
					require.Nil(t, iter.Err())
				})
			})

		})
	})

	s.When(`close encounter error`, func(s *testcase.Spec) {
		s.Let(`rows`, func(t *testcase.T) interface{} {
			mock := NewMockSQLRows(t.I(`rows.mock.ctrl`).(*gomock.Controller))
			mock.EXPECT().Close().Return(errors.New(`boom`))
			return mock
		})

		s.Then(`it will be propagated during iterator closing`, func(t *testcase.T) {
			iter := subject(t)
			err := iter.Close()
			require.Error(t, err)
			require.Equal(t, `boom`, err.Error())
		})
	})

}
