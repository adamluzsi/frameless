package externalresources_test

import (
	"github.com/adamluzsi/frameless/externalresources"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupID_IDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := externalresources.LookupID(IDInFieldName{"ok"})

	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_PointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := externalresources.LookupID(&IDInFieldName{"ok"})

	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_PointerOfPointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	var ptr1 *IDInFieldName
	var ptr2 **IDInFieldName

	ptr1 = &IDInFieldName{"ok"}
	ptr2 = &ptr1

	id, ok := externalresources.LookupID(ptr2)

	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_IDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := externalresources.LookupID(IDInTagName{"KO"})

	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookupID_PointerIDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := externalresources.LookupID(&IDInTagName{"KO"})

	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookupID_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	t.Parallel()

	id, ok := externalresources.LookupID(UnidentifiableID{"ok"})

	require.False(t, ok)
	require.Equal(t, "", id)
}
