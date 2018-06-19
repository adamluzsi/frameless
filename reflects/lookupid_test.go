package reflects_test

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

func TestLookupID_IDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := reflects.LookupID(IDInFieldName{"ok"})

	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_PointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := reflects.LookupID(&IDInFieldName{"ok"})

	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_IDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := reflects.LookupID(IDInTagName{"KO"})

	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookupID_PointerIDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := reflects.LookupID(&IDInTagName{"KO"})

	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookupID_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	t.Parallel()

	id, ok := reflects.LookupID(UnidentifiableID{"ok"})

	require.False(t, ok)
	require.Equal(t, "", id)
}
