package iterateover_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterators/iterateover"
	"github.com/adamluzsi/frameless/iterators/iterateover/sqlrows"
)

var _ sqlrows.Rows = &sql.Rows{}

func TestSQLRows_IteratorClosed_RowsReceivedIt(t *testing.T) {
	t.Parallel()

	rows := NewRows([][]string{}, nil)
	matcher := func(scan sqlrows.Scan, destination interface{}) error { return nil }
	iterator := iterateover.SQLRows(rows, matcher)

	require.Nil(t, iterator.Close())
	require.True(t, rows.IsClosed)

}

func TestSQLRows_ErrorHappenInRows_IteratorReflectIt(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("KaBuM")

	rows := NewRows([][]string{}, expectedErr)
	matcher := func(scan sqlrows.Scan, destination interface{}) error { return nil }
	iterator := iterateover.SQLRows(rows, matcher)

	require.Equal(t, expectedErr, iterator.Err())

}

func TestSQLRows_DecoderGiven_IteratorUseItForScanning(t *testing.T) {
	t.Parallel()

	rows := NewRows(
		[][]string{
			[]string{"a", "b", "c"},
			[]string{"d", "e", "f"},
		},
		nil,
	)

	matcher := func(scan sqlrows.Scan, destination interface{}) error {
		o, ok := destination.(*SQLRowsSubject)

		if !ok {
			panic("this only can works with *SQLRowsSubject")
		}

		return scan(&o.C1, &o.C2, &o.C3)
	}

	iterator := iterateover.SQLRows(rows, matcher)
	defer iterator.Close()

	results := make([]*SQLRowsSubject, 0)
	for iterator.Next() {
		o := &SQLRowsSubject{}

		if err := iterator.Decode(o); err != nil {
			t.Fatal(err)
		}

		results = append(results, o)
	}

	require.Nil(t, iterator.Err())
	require.Equal(t, 2, len(results))
	require.Equal(t, &SQLRowsSubject{C1: "a", C2: "b", C3: "c"}, results[0])
	require.Equal(t, &SQLRowsSubject{C1: "d", C2: "e", C3: "f"}, results[1])
}
