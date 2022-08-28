package extid_test

import (
	"github.com/adamluzsi/frameless/ports/crud/extid"
	"reflect"
	"testing"

	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

func TestID_E2E(t *testing.T) {
	ptr := &IDAsInterface{}

	_, ok := extid.Lookup[any](ptr)
	assert.Must(t).False(ok)

	idVal := 42
	assert.Must(t).Nil(extid.Set(ptr, idVal))

	id, ok := extid.Lookup[any](ptr)
	assert.Must(t).True(ok)
	assert.Must(t).Equal(idVal, id)
}

func TestLookup_IDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[string](IDByIDField{ID: "ok"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("ok", id)
}

func TestLookup_PointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[string](&IDByIDField{ID: "ok"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("ok", id)
}

func TestLookup_PointerOfPointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	var ptr1 *IDByIDField
	var ptr2 **IDByIDField

	ptr1 = &IDByIDField{"ok"}
	ptr2 = &ptr1

	id, ok := extid.Lookup[string](ptr2)
	assert.Must(t).True(ok)
	assert.Must(t).Equal("ok", id)
}

func TestLookup_IDGivenByUppercaseTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[string](IDByUppercaseTag{DI: "KO"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("KO", id)
}

func TestLookup_IDGivenByLowercaseTag_IDReturned(t *testing.T) {
	t.Parallel()

	expected := random.New(random.CryptoSeed{}).String()
	id, ok := extid.Lookup[string](IDByLowercaseTag{DI: expected})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(expected, id)
}

func TestLookup_IDGivenByTagButIDFieldAlsoPresentForOtherPurposes_IDReturnedByTag(t *testing.T) {
	t.Parallel()

	type IDByTagNameNextToIDField struct {
		ID string
		DI string `ext:"ID"`
	}

	id, ok := extid.Lookup[string](IDByTagNameNextToIDField{DI: "KO", ID: "OK"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("KO", id)
}

func TestLookup_PointerIDGivenByTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[string](&IDByUppercaseTag{DI: "KO"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("KO", id)
}

func TestLookup_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[any](UnidentifiableID{UserID: "ok"})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_InterfaceTypeWithValue_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[any](&IDAsInterface{ID: `foo`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("foo", id)
}

func TestLookup_InterfaceTypeWithNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[any](&IDAsInterface{})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_InterfaceTypeWithPointerTypeThatHasNoValueNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	var idVal *string
	id, ok := extid.Lookup[any](&IDAsInterface{ID: idVal})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_PointerTypeThatIsNotInitialized_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[*string](&IDAsPointer{})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_PointerTypeWithValue_ValueReturned(t *testing.T) {
	t.Parallel()

	idVal := `foo`
	id, ok := extid.Lookup[*string](&IDAsPointer{ID: &idVal})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(&idVal, id)
}

func TestLookup_IDFieldWithZeroValueFound_NotOkReturned(t *testing.T) {
	t.Parallel()

	_, ok := extid.Lookup[string](IDByIDField{ID: ""})
	assert.Must(t).False(ok, "zero value should be not OK")
}

// ------------------------------------------------------------------------------------------------------------------ //

func TestSet_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	t.Parallel()

	assert.Must(t).NotNil(extid.Set(IDByIDField{}, "Pass by Value"))
}

func TestSet_PtrStructGivenButIDIsCannotBeIdentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	t.Parallel()

	assert.Must(t).NotNil(extid.Set(&UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec"))
}

func TestSet_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDByIDField{}
	assert.Must(t).Nil(extid.Set(subject, "OK"))
	assert.Must(t).Equal("OK", subject.ID)
}

func TestSet_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &IDByUppercaseTag{}
	assert.Must(t).Nil(extid.Set(subject, "OK"))
	assert.Must(t).Equal("OK", subject.DI)
}

func TestSet_InterfaceTypeGiven_IDSaved(t *testing.T) {
	t.Parallel()

	var subject interface{} = &IDByIDField{}
	assert.Must(t).Nil(extid.Set(subject, "OK"))
	assert.Must(t).Equal("OK", subject.(*IDByIDField).ID)
}

//--------------------------------------------------------------------------------------------------------------------//

func TestLookupStructField(t *testing.T) {
	var (
		field reflect.StructField
		value reflect.Value
		ok    bool
	)

	field, value, ok = extid.LookupStructField(IDByIDField{ID: `42`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`ID`, field.Name)
	assert.Must(t).Equal(`42`, value.Interface())

	field, value, ok = extid.LookupStructField(IDByUppercaseTag{DI: `42`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`DI`, field.Name)
	assert.Must(t).Equal(`42`, value.Interface())

	field, value, ok = extid.LookupStructField(IDByLowercaseTag{DI: `42`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`DI`, field.Name)
	assert.Must(t).Equal(`42`, value.Interface())

	field, value, ok = extid.LookupStructField(IDAsInterface{ID: 42})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`ID`, field.Name)
	assert.Must(t).Equal(42, value.Interface())

	idValue := `42`
	field, value, ok = extid.LookupStructField(IDAsPointer{ID: &idValue})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(`ID`, field.Name)
	assert.Must(t).Equal(&idValue, value.Interface())

	field, value, ok = extid.LookupStructField(UnidentifiableID{})
	assert.Must(t).False(ok)
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
