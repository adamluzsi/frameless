package queries_test

import (
	"github.com/adamluzsi/frameless/queries"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/stretchr/testify/require"
)

// Pass By Value
func TestSetID_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	t.Parallel()

	err := queries.SetID(IDInFieldName{}, "Pass by Value")

	require.Error(t, err)
}

func TestSetID_PtrStructGivenButIDIsCannotBeIndentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	t.Parallel()

	err := queries.SetID(&UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec")

	require.Error(t, err)
}

func TestSetID_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDInFieldName{}

	err := queries.SetID(subject, "OK")

	require.Nil(t, err)
	require.Equal(t, "OK", subject.ID)
}

func TestSetID_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDInTagName{}

	err := queries.SetID(subject, "OK")

	require.Nil(t, err)
	require.Equal(t, "OK", subject.DI)
}

func TestSetID_InterfaceTypeGiven_IDSaved(t *testing.T) {
	t.Parallel()

	var subject frameless.Entity = &IDInFieldName{}
	require.Nil(t, queries.SetID(subject, "OK"))
	require.Equal(t, "OK", subject.(*IDInFieldName).ID)
}
