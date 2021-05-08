package resources_test

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/resources"

	"github.com/stretchr/testify/require"
)

func TestID_E2E(t *testing.T) {
	ptr := &IDAsInterface{}

	_, ok := resources.LookupID(ptr)
	require.False(t, ok)

	idVal := 42
	require.Nil(t, resources.SetID(ptr, idVal))

	id, ok := resources.LookupID(ptr)
	require.True(t, ok)
	require.Equal(t, idVal, id)
}

func TestLookupID_IDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(IDByIDField{"ok"})
	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_PointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(&IDByIDField{"ok"})
	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_PointerOfPointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	var ptr1 *IDByIDField
	var ptr2 **IDByIDField

	ptr1 = &IDByIDField{"ok"}
	ptr2 = &ptr1

	id, ok := resources.LookupID(ptr2)
	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookupID_IDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(IDByTag{"KO"})
	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookupID_IDGivenByTagButIDFieldAlsoPresentForOtherPurposes_IDReturnedByTag(t *testing.T) {
	t.Parallel()

	type IDByTagNameNextToIDField struct {
		ID string
		DI string `ext:"ID"`
	}

	id, ok := resources.LookupID(IDByTagNameNextToIDField{DI: "KO", ID: "OK"})
	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookupID_PointerIDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(&IDByTag{"KO"})
	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookupID_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(UnidentifiableID{"ok"})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookupID_InterfaceTypeWithValue_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(&IDAsInterface{ID: `foo`})
	require.True(t, ok)
	require.Equal(t, "foo", id)
}

func TestLookupID_InterfaceTypeWithNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(&IDAsInterface{})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookupID_InterfaceTypeWithPointerTypeThatHasNoValueNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	var idVal *string
	id, ok := resources.LookupID(&IDAsInterface{ID: idVal})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookupID_PointerTypeThatIsNotInitialized_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := resources.LookupID(&IDAsPointer{})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookupID_PointerTypeWithValue_ValueReturned(t *testing.T) {
	t.Parallel()

	idVal := `foo`
	id, ok := resources.LookupID(&IDAsPointer{ID: &idVal})
	require.True(t, ok)
	require.Equal(t, &idVal, id)
}

// ------------------------------------------------------------------------------------------------------------------ //

func TestSetID_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	t.Parallel()

	require.Error(t, resources.SetID(IDByIDField{}, "Pass by Value"))
}

func TestSetID_PtrStructGivenButIDIsCannotBeIndentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	t.Parallel()

	require.Error(t, resources.SetID(&UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec"))
}

func TestSetID_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDByIDField{}
	require.Nil(t, resources.SetID(subject, "OK"))
	require.Equal(t, "OK", subject.ID)
}

func TestSetID_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDByTag{}
	require.Nil(t, resources.SetID(subject, "OK"))
	require.Equal(t, "OK", subject.DI)
}

func TestSetID_InterfaceTypeGiven_IDSaved(t *testing.T) {
	t.Parallel()

	var subject interface{} = &IDByIDField{}
	require.Nil(t, resources.SetID(subject, "OK"))
	require.Equal(t, "OK", subject.(*IDByIDField).ID)
}

func TestLookupIDStructField(t *testing.T) {
	var (
		field reflect.StructField
		value reflect.Value
		ok    bool
	)

	field, value, ok = resources.LookupIDStructField(IDByIDField{ID: `42`})
	require.True(t, ok)
	require.Equal(t, `ID`, field.Name)
	require.Equal(t, `42`, value.Interface())

	field, value, ok = resources.LookupIDStructField(IDByTag{DI: `42`})
	require.True(t, ok)
	require.Equal(t, `DI`, field.Name)
	require.Equal(t, `42`, value.Interface())

	field, value, ok = resources.LookupIDStructField(IDAsInterface{ID: 42})
	require.True(t, ok)
	require.Equal(t, `ID`, field.Name)
	require.Equal(t, 42, value.Interface())

	idValue := `42`
	field, value, ok = resources.LookupIDStructField(IDAsPointer{ID: &idValue})
	require.True(t, ok)
	require.Equal(t, `ID`, field.Name)
	require.Equal(t, &idValue, value.Interface())

	field, value, ok = resources.LookupIDStructField(UnidentifiableID{})
	require.False(t, ok)
}

//--------------------------------------------------------------------------------------------------------------------//

type IDByIDField struct {
	ID string
}

type IDByTag struct {
	DI string `ext:"ID"`
}

type IDAsInterface struct {
	ID interface{} `ext:"ID"`
}

type IDAsPointer struct {
	ID *string `ext:"ID"`
}

type UnidentifiableID struct {
	UserID string
}
