package iterate_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/adamluzsi/frameless/iterate/oversqlrows"

	"github.com/adamluzsi/frameless/iterate"
)

var _ oversqlrows.Rows = &sql.Rows{}

func TestOverSQLRows_IteratorClosed_RowsReceivedIt(t *testing.T) {
	t.Parallel()

	rows := NewRows([][]string{}, nil)
	matcher := func(scan oversqlrows.Scan, destination interface{}) error { return nil }
	iterator := iterate.OverSQLRows(rows, matcher)

	require.Nil(t, iterator.Close())
	require.True(t, rows.IsClosed)

}

func TestOverSQLRows_ErrorHappenInRows_IteratorReflectIt(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("KaBuM")

	rows := NewRows([][]string{}, expectedErr)
	matcher := func(scan oversqlrows.Scan, destination interface{}) error { return nil }
	iterator := iterate.OverSQLRows(rows, matcher)

	require.Equal(t, expectedErr, iterator.Err())

}

func TestOverSQLRows_DecoderGiven_IteratorUseItForScanning(t *testing.T) {
	t.Parallel()

	rows := NewRows(
		[][]string{
			[]string{"a", "b", "c"},
			[]string{"d", "e", "f"},
		},
		nil,
	)

	matcher := func(scan oversqlrows.Scan, destination interface{}) error {
		o, ok := destination.(*OverSQLRowsSubject)

		if !ok {
			panic("this only can works with *OverSQLRowsSubject")
		}

		return scan(&o.C1, &o.C2, &o.C3)
	}

	iterator := iterate.OverSQLRows(rows, matcher)
	defer iterator.Close()

	results := make([]*OverSQLRowsSubject, 0)
	for iterator.Next() {
		o := &OverSQLRowsSubject{}

		if err := iterator.Decode(o); err != nil {
			t.Fatal(err)
		}

		results = append(results, o)
	}

	require.Nil(t, iterator.Err())
	require.Equal(t, 2, len(results))
	require.Equal(t, &OverSQLRowsSubject{C1: "a", C2: "b", C3: "c"}, results[0])
	require.Equal(t, &OverSQLRowsSubject{C1: "d", C2: "e", C3: "f"}, results[1])
}
