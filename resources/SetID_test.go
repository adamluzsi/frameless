package resources_test

import (
	"testing"

	"github.com/adamluzsi/frameless/resources"

	"github.com/stretchr/testify/require"
)

// Pass By Value
func TestSetID_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	t.Parallel()

	err := resources.SetID(IDInFieldName{}, "Pass by Value")

	require.Error(t, err)
}

func TestSetID_PtrStructGivenButIDIsCannotBeIndentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	t.Parallel()

	err := resources.SetID(&UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec")

	require.Error(t, err)
}

func TestSetID_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDInFieldName{}

	err := resources.SetID(subject, "OK")

	require.Nil(t, err)
	require.Equal(t, "OK", subject.ID)
}

func TestSetID_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDInTagName{}

	err := resources.SetID(subject, "OK")

	require.Nil(t, err)
	require.Equal(t, "OK", subject.DI)
}

func TestSetID_InterfaceTypeGiven_IDSaved(t *testing.T) {
	t.Parallel()

	var subject interface{} = &IDInFieldName{}
	require.Nil(t, resources.SetID(subject, "OK"))
	require.Equal(t, "OK", subject.(*IDInFieldName).ID)
}
