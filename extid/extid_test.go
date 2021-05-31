package extid_test

import (
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/fixtures"

	"github.com/adamluzsi/frameless/extid"
	"github.com/stretchr/testify/require"
)

func TestID_E2E(t *testing.T) {
	ptr := &IDAsInterface{}

	_, ok := extid.Lookup(ptr)
	require.False(t, ok)

	idVal := 42
	require.Nil(t, extid.Set(ptr, idVal))

	id, ok := extid.Lookup(ptr)
	require.True(t, ok)
	require.Equal(t, idVal, id)
}

func TestLookup_IDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(IDByIDField{ID: "ok"})
	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookup_PointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(&IDByIDField{ID: "ok"})
	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookup_PointerOfPointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	var ptr1 *IDByIDField
	var ptr2 **IDByIDField

	ptr1 = &IDByIDField{"ok"}
	ptr2 = &ptr1

	id, ok := extid.Lookup(ptr2)
	require.True(t, ok)
	require.Equal(t, "ok", id)
}

func TestLookup_IDGivenByUppercaseTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(IDByUppercaseTag{DI: "KO"})
	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookup_IDGivenByLowercaseTag_IDReturned(t *testing.T) {
	t.Parallel()

	expected := fixtures.Random.String()
	id, ok := extid.Lookup(IDByLowercaseTag{DI: expected})
	require.True(t, ok)
	require.Equal(t, expected, id)
}

func TestLookup_IDGivenByTagButIDFieldAlsoPresentForOtherPurposes_IDReturnedByTag(t *testing.T) {
	t.Parallel()

	type IDByTagNameNextToIDField struct {
		ID string
		DI string `ext:"ID"`
	}

	id, ok := extid.Lookup(IDByTagNameNextToIDField{DI: "KO", ID: "OK"})
	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookup_PointerIDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(&IDByUppercaseTag{DI: "KO"})
	require.True(t, ok)
	require.Equal(t, "KO", id)
}

func TestLookup_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(UnidentifiableID{UserID: "ok"})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookup_InterfaceTypeWithValue_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(&IDAsInterface{ID: `foo`})
	require.True(t, ok)
	require.Equal(t, "foo", id)
}

func TestLookup_InterfaceTypeWithNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(&IDAsInterface{})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookup_InterfaceTypeWithPointerTypeThatHasNoValueNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	var idVal *string
	id, ok := extid.Lookup(&IDAsInterface{ID: idVal})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookup_PointerTypeThatIsNotInitialized_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup(&IDAsPointer{})
	require.False(t, ok)
	require.Nil(t, id)
}

func TestLookup_PointerTypeWithValue_ValueReturned(t *testing.T) {
	t.Parallel()

	idVal := `foo`
	id, ok := extid.Lookup(&IDAsPointer{ID: &idVal})
	require.True(t, ok)
	require.Equal(t, &idVal, id)
}

// ------------------------------------------------------------------------------------------------------------------ //

func TestSet_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	t.Parallel()

	require.Error(t, extid.Set(IDByIDField{}, "Pass by Value"))
}

func TestSet_PtrStructGivenButIDIsCannotBeIndentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	t.Parallel()

	require.Error(t, extid.Set(&UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec"))
}

func TestSet_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDByIDField{}
	require.Nil(t, extid.Set(subject, "OK"))
	require.Equal(t, "OK", subject.ID)
}

func TestSet_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDByUppercaseTag{}
	require.Nil(t, extid.Set(subject, "OK"))
	require.Equal(t, "OK", subject.DI)
}

func TestSet_InterfaceTypeGiven_IDSaved(t *testing.T) {
	t.Parallel()

	var subject interface{} = &IDByIDField{}
	require.Nil(t, extid.Set(subject, "OK"))
	require.Equal(t, "OK", subject.(*IDByIDField).ID)
}

//--------------------------------------------------------------------------------------------------------------------//

func TestLookupStructField(t *testing.T) {
	var (
		field reflect.StructField
		value reflect.Value
		ok    bool
	)

	field, value, ok = extid.LookupStructField(IDByIDField{ID: `42`})
	require.True(t, ok)
	require.Equal(t, `ID`, field.Name)
	require.Equal(t, `42`, value.Interface())

	field, value, ok = extid.LookupStructField(IDByUppercaseTag{DI: `42`})
	require.True(t, ok)
	require.Equal(t, `DI`, field.Name)
	require.Equal(t, `42`, value.Interface())

	field, value, ok = extid.LookupStructField(IDByLowercaseTag{DI: `42`})
	require.True(t, ok)
	require.Equal(t, `DI`, field.Name)
	require.Equal(t, `42`, value.Interface())

	field, value, ok = extid.LookupStructField(IDAsInterface{ID: 42})
	require.True(t, ok)
	require.Equal(t, `ID`, field.Name)
	require.Equal(t, 42, value.Interface())

	idValue := `42`
	field, value, ok = extid.LookupStructField(IDAsPointer{ID: &idValue})
	require.True(t, ok)
	require.Equal(t, `ID`, field.Name)
	require.Equal(t, &idValue, value.Interface())

	field, value, ok = extid.LookupStructField(UnidentifiableID{})
	require.False(t, ok)
}

//--------------------------------------------------------------------------------------------------------------------//

type IDByIDField struct {
	ID string
}

type IDByUppercaseTag struct {
	DI string `ext:"ID"`
}

type IDByLowercaseTag struct {
	DI string `ext:"id"`
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
