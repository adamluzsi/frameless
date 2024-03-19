package extid_test

import (
	"go.llib.dev/frameless/ports/crud/extid/internal/testhelper"
	"testing"

	"go.llib.dev/frameless/ports/crud/extid"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

func TestID_E2E(t *testing.T) {
	ptr := &testhelper.IDAsInterface{}

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

	id, ok := extid.Lookup[string](testhelper.IDByIDField{ID: "ok"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("ok", id)
}

func TestLookup_withAnyType_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[any](testhelper.IDByIDField{ID: "ok"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(any("ok"), id)
}

func TestLookup_PointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[string](&testhelper.IDByIDField{ID: "ok"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("ok", id)
}

func TestLookup_PointerOfPointerIDGivenByFieldName_IDReturned(t *testing.T) {
	t.Parallel()

	var ptr1 *testhelper.IDByIDField
	var ptr2 **testhelper.IDByIDField

	ptr1 = &testhelper.IDByIDField{ID: "ok"}
	ptr2 = &ptr1

	id, ok := extid.Lookup[string](ptr2)
	assert.Must(t).True(ok)
	assert.Must(t).Equal("ok", id)
}

func TestLookup_IDGivenByUppercaseTag_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[string](testhelper.IDByUppercaseTag{DI: "KO"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("KO", id)
}

func TestLookup_IDGivenByLowercaseTag_IDReturned(t *testing.T) {
	t.Parallel()

	expected := random.New(random.CryptoSeed{}).String()
	id, ok := extid.Lookup[string](testhelper.IDByLowercaseTag{DI: expected})
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

	id, ok := extid.Lookup[string](&testhelper.IDByUppercaseTag{DI: "KO"})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("KO", id)
}

func TestLookup_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[any](testhelper.UnidentifiableID{UserID: "ok"})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_InterfaceTypeWithValue_IDReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[any](&testhelper.IDAsInterface{ID: `foo`})
	assert.Must(t).True(ok)
	assert.Must(t).Equal("foo", id)
}

func TestLookup_InterfaceTypeWithNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[any](&testhelper.IDAsInterface{})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_InterfaceTypeWithPointerTypeThatHasNoValueNilAsValue_NotFoundReturned(t *testing.T) {
	t.Parallel()

	var idVal *string
	id, ok := extid.Lookup[any](&testhelper.IDAsInterface{ID: idVal})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_PointerTypeThatIsNotInitialized_NotFoundReturned(t *testing.T) {
	t.Parallel()

	id, ok := extid.Lookup[*string](&testhelper.IDAsPointer{})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_PointerTypeWithValue_ValueReturned(t *testing.T) {
	t.Parallel()

	idVal := `foo`
	id, ok := extid.Lookup[*string](&testhelper.IDAsPointer{ID: &idVal})
	assert.Must(t).True(ok)
	assert.Must(t).Equal(&idVal, id)
}

func TestLookup_IDFieldWithZeroValueFound_NotOkReturned(t *testing.T) {
	t.Parallel()

	_, ok := extid.Lookup[string](testhelper.IDByIDField{ID: ""})
	assert.Must(t).False(ok, "zero value should be not OK")
}

// ------------------------------------------------------------------------------------------------------------------ //

func TestSet_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	t.Parallel()

	assert.Must(t).Error(extid.Set(testhelper.IDByIDField{}, "Set doesn't work with pass by value"))
}

func TestSet_PtrStructGivenButIDIsCannotBeIdentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	t.Parallel()

	assert.Must(t).NotNil(extid.Set(&testhelper.UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec"))
}

func TestSet_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &testhelper.IDByIDField{}
	assert.Must(t).Nil(extid.Set(subject, "OK"))
	assert.Must(t).Equal("OK", subject.ID)
}

func TestSet_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	t.Parallel()

	subject := &testhelper.IDByUppercaseTag{}
	assert.Must(t).Nil(extid.Set(subject, "OK"))
	assert.Must(t).Equal("OK", subject.DI)
}

func TestSet_InterfaceTypeGiven_IDSaved(t *testing.T) {
	t.Parallel()

	var subject interface{} = &testhelper.IDByIDField{}
	assert.Must(t).Nil(extid.Set(subject, "OK"))
	assert.Must(t).Equal("OK", subject.(*testhelper.IDByIDField).ID)
}

//--------------------------------------------------------------------------------------------------------------------//

type TypeWithCustomIDSet struct {
	Identification string
}

var _ = extid.RegisterType[TypeWithCustomIDSet, string](
	func(ent TypeWithCustomIDSet) string {
		return ent.Identification
	},
	func(ent *TypeWithCustomIDSet, id string) {
		ent.Identification = id
	},
)

func TestRegisterType(t *testing.T) {
	var ent TypeWithCustomIDSet
	id := random.New(random.CryptoSeed{}).String()
	gotID, ok := extid.Lookup[string](ent)
	assert.True(t, ok)
	assert.Empty(t, gotID)
	assert.NoError(t, extid.Set(&ent, id))
	assert.Equal(t, id, ent.Identification)
	gotID, ok = extid.Lookup[string](ent)
	assert.True(t, ok)
	assert.Equal(t, ent.Identification, id)
}
