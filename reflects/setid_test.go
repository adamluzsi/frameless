package reflects_test

import (
	"testing"

	"github.com/adamluzsi/frameless/reflects"
	"github.com/stretchr/testify/require"
)

// Pass By Value
func TestSetID_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	t.Parallel()

	err := reflects.SetID(IDInFieldName{}, "Pass by Value")

	require.Error(t, err)
}

func TestSetID_PtrStructGivenButIDIsCannotBeIndentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	t.Parallel()

	err := reflects.SetID(&UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec")

	require.Error(t, err)
}

func TestSetID_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDInFieldName{}

	err := reflects.SetID(subject, "OK")

	require.Nil(t, err)
	require.Equal(t, "OK", subject.ID)
}

func TestSetID_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDInTagName{}

	err := reflects.SetID(subject, "OK")

	require.Nil(t, err)
	require.Equal(t, "OK", subject.DI)
}
