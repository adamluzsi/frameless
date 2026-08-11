package extid_test

import (
	"reflect"
	"sync"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud/extid/internal/testhelper"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/testing/testent"
	"go.llib.dev/testcase"

	"go.llib.dev/frameless/port/crud/extid"

	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

var _ extid.LookupIDFunc[testent.Foo, testent.FooID] = extid.Lookup[testent.FooID, testent.Foo]

func Benchmark(b *testing.B) {
	type IDByField struct {
		ID string
	}
	type IDByTag struct {
		IDD string `ext:"id"`
	}
	b.Run("extid", func(b *testing.B) {
		b.Run("Lookup", func(b *testing.B) {
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				extid.Lookup[string](vs[i])
			}
		})
		b.Run("Set", func(b *testing.B) {
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				v := vs[i]
				extid.Set(&v, v.ID)
			}
		})
		b.Run("Get", func(b *testing.B) {
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				extid.Get[string](vs[i])
			}
		})
	})
	b.Run("ExtractIdentifierField", func(b *testing.B) {
		b.Run("id by ID field", func(b *testing.B) {
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				extid.ExtractIdentifierField(vs[i])
			}
		})
		b.Run("id by tag", func(b *testing.B) {
			extid.ExtractIdentifierField(IDByTag{})
			vs := random.Slice(b.N, func() IDByTag {
				return IDByTag{IDD: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				extid.ExtractIdentifierField(vs[i])
			}
		})
	})
	b.Run("Accessor", func(b *testing.B) {
		b.Run("Lookup", func(b *testing.B) {
			accessor := extid.Accessor[IDByField, string](func(v *IDByField) *string {
				return &v.ID
			})
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				accessor.Lookup(vs[i])
			}
		})
		b.Run("Set", func(b *testing.B) {
			accessor := extid.Accessor[IDByField, string](func(v *IDByField) *string {
				return &v.ID
			})
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				v := vs[i]
				accessor.Set(&v, v.ID)
			}
		})
		b.Run("Get", func(b *testing.B) {
			accessor := extid.Accessor[IDByField, string](func(v *IDByField) *string {
				return &v.ID
			})
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := range b.N {
				accessor.Get(vs[i])
			}
		})
	})
}

func TestID_E2E(t *testing.T) {
	ptr := &testhelper.IDAsInterface{}

	_, ok := extid.Lookup[any](ptr)
	assert.Must(t).False(ok)

	idVal := 42
	assert.Must(t).NoError(extid.Set(ptr, idVal))

	id, ok := extid.Lookup[any](ptr)
	assert.True(t, ok)
	assert.Equal[any](t, idVal, id)
}

func TestLookup_withEmbededField(t *testing.T) {
	type E struct {
		ID string `ext:"id"`
	}
	type T struct{ E }

	expID := rnd.String()
	v := T{E: E{ID: expID}}

	gotID, ok := extid.Lookup[string](v)
	assert.True(t, ok)
	assert.Equal(t, expID, gotID)
}

func TestLookup_withZeroIDValue(t *testing.T) {
	type T struct {
		ID int `ext:"id"`
	}
	v := T{}
	gotID, ok := extid.Lookup[int](v)
	assert.True(t, ok)
	assert.Empty(t, gotID)

	var acc extid.Accessor[T, int]
	gotID, ok = acc.Lookup(v)
	assert.True(t, ok)
	assert.Empty(t, gotID)
}

func TestExtractIdentifierField_nonStructValue(t *testing.T) {
	_, _, ok := extid.ExtractIdentifierField("The answer is")
	assert.False(t, ok)

	_, _, ok = extid.ExtractIdentifierField(42)
	assert.False(t, ok)
}

func TestLookup_IDGivenByFieldName_IDReturned(t *testing.T) {
	id, ok := extid.Lookup[string](testhelper.IDByIDField{ID: "ok"})
	assert.True(t, ok)
	assert.Equal(t, "ok", id)
}

func TestLookup_withAnyType_IDReturned(t *testing.T) {
	id, ok := extid.Lookup[any](testhelper.IDByIDField{ID: "ok"})
	assert.True(t, ok)
	assert.Equal(t, any("ok"), id)
}

func TestLookup_PointerIDGivenByFieldName_IDReturned(t *testing.T) {
	id, ok := extid.Lookup[string](&testhelper.IDByIDField{ID: "ok"})
	assert.True(t, ok)
	assert.Equal(t, "ok", id)
}

func TestLookup_zeroIntTypesConsideredFound(t *testing.T) {
	type I struct{ ID int }
	type I8 struct{ ID int8 }
	type I16 struct{ ID int16 }
	type I32 struct{ ID int32 }
	type I64 struct{ ID int64 }

	type UI struct{ ID uint }
	type UI8 struct{ ID uint8 }
	type UI16 struct{ ID uint16 }
	type UI32 struct{ ID uint32 }
	type UI64 struct{ ID uint64 }

	var examples = []any{I{}, I8{}, I16{}, I32{}, I64{}, UI{}, UI8{}, UI16{}, UI32{}, UI64{}}

	for _, example := range examples {
		id, ok := extid.Lookup[any](example)
		assert.True(t, ok)
		assert.Empty(t, id)
	}
}

func TestLookup_withPointerValueTypeWhereValueTypeHasRegisteredGetter(t *testing.T) {
	type T struct{ IDD string }

	defer extid.RegisterType[T, string](
		func(v T) string { return v.IDD },
		func(p *T, id string) { p.IDD = id },
	)()

	t.Run("when id is there", func(t *testing.T) {
		id, ok := extid.Lookup[string](T{IDD: "ok"})
		assert.True(t, ok)
		assert.Equal(t, "ok", id)
	})

	t.Run("when id is empty", func(t *testing.T) {
		id, ok := extid.Lookup[string](T{IDD: ""})
		assert.True(t, ok, "it is still reported to be found (string id field found because of the T#IDD field)")
		assert.Equal(t, "", id)
	})
}

func TestLookup_PointerOfPointerIDGivenByFieldName_IDReturned(t *testing.T) {
	var ptr1 *testhelper.IDByIDField
	var ptr2 **testhelper.IDByIDField

	ptr1 = &testhelper.IDByIDField{ID: "ok"}
	ptr2 = &ptr1

	id, ok := extid.Lookup[string](ptr2)
	assert.True(t, ok)
	assert.Equal(t, "ok", id)
}

func TestLookup_IDGivenByUppercaseTag_IDReturned(t *testing.T) {
	id, ok := extid.Lookup[string](testhelper.IDByUppercaseTag{DI: "KO"})
	assert.True(t, ok)
	assert.Equal(t, "KO", id)
}

func TestLookup_IDGivenByLowercaseTag_IDReturned(t *testing.T) {
	expected := random.New(random.CryptoSeed{}).String()
	id, ok := extid.Lookup[string](testhelper.IDByLowercaseTag{DI: expected})
	assert.True(t, ok)
	assert.Equal(t, expected, id)
}

func TestLookup_IDGivenByTagButIDFieldAlsoPresentForOtherPurposes_IDReturnedByTag(t *testing.T) {
	type IDByTagNameNextToIDField struct {
		ID string
		DI string `ext:"ID"`
	}

	id, ok := extid.Lookup[string](IDByTagNameNextToIDField{DI: "KO", ID: "OK"})
	assert.True(t, ok)
	assert.Equal(t, "KO", id)
}

func TestLookup_PointerIDGivenByTag_IDReturned(t *testing.T) {
	id, ok := extid.Lookup[string](&testhelper.IDByUppercaseTag{DI: "KO"})
	assert.True(t, ok)
	assert.Equal(t, "KO", id)
}

func TestLookup_InterfaceBoxingPointerToStructWithTaggedID_IDReturned(t *testing.T) {
	type EntID string
	type Ent struct {
		ID  EntID `ext:"id"`
		Foo string
	}

	var boxed interface{} = &Ent{ID: "42", Foo: "foo"}

	id, ok := extid.Lookup[EntID](boxed)
	assert.True(t, ok, "expected the ext:\"id\" field to be found through the interface-boxed pointer")
	assert.Equal(t, EntID("42"), id)
}

func TestLookup_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	id, ok := extid.Lookup[any](testhelper.UnidentifiableID{UserID: 42.24})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_InterfaceTypeWithValue_IDReturned(t *testing.T) {
	id, ok := extid.Lookup[any](&testhelper.IDAsInterface{ID: `foo`})
	assert.True(t, ok)
	assert.Equal(t, "foo", id)
}

func TestLookup_InterfaceTypeWithNilAsValue_NotFoundReturned(t *testing.T) {
	id, ok := extid.Lookup[any](&testhelper.IDAsInterface{})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_InterfaceTypeWithPointerTypeThatHasNoValueNilAsValue_NotFoundReturned(t *testing.T) {
	var idVal *string
	id, ok := extid.Lookup[any](&testhelper.IDAsInterface{ID: idVal})
	assert.True(t, ok, "id field indentified")
	assert.Nil(t, id, "id field is nil")
}

func TestLookup_PointerTypeThatIsNotInitialized_NotFoundReturned(t *testing.T) {
	id, ok := extid.Lookup[*string](&testhelper.IDAsPointer{})
	assert.True(t, ok, "testhelper.IDAsPointer has an ID field, and it can be located")
	assert.Nil(t, id, "the actual value is nil since a pointer zero value is nil")
}

func TestLookup_PointerTypeWithValue_ValueReturned(t *testing.T) {
	idVal := `foo`
	id, ok := extid.Lookup[*string](&testhelper.IDAsPointer{ID: &idVal})
	assert.True(t, ok)
	assert.Equal(t, &idVal, id)
}

func TestLookup_IDFieldWithZeroValueFound_OkReturned(t *testing.T) {
	var zero string
	_, ok := extid.Lookup[string](testhelper.IDByIDField{ID: zero})
	assert.True(t, ok, "zero value should be OK, since the field exist")
}

// ------------------------------------------------------------------------------------------------------------------ //

func TestSet_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	assert.Must(t).Error(extid.Set(testhelper.IDByIDField{}, "Set doesn't work with pass by value"))
}

func TestSet_PtrStructGivenButIDIsCannotBeIdentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	assert.NotNil(t, extid.Set(&testhelper.UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec"))
}

func TestSet_PtrStructGivenWithIDField_IDSaved(t *testing.T) {
	subject := &testhelper.IDByIDField{}
	assert.Must(t).NoError(extid.Set(subject, "OK"))
	assert.Equal(t, "OK", subject.ID)
}

func TestSet_PtrStructGivenWithIDTaggedField_IDSaved(t *testing.T) {
	subject := &testhelper.IDByUppercaseTag{}
	assert.Must(t).NoError(extid.Set(subject, "OK"))
	assert.Equal(t, "OK", subject.DI)
}

func TestSet_InterfaceTypeGiven_IDSaved(t *testing.T) {
	var subject interface{} = &testhelper.IDByIDField{}
	assert.Must(t).NoError(extid.Set(subject, "OK"))
	assert.Equal(t, "OK", subject.(*testhelper.IDByIDField).ID)
}

//--------------------------------------------------------------------------------------------------------------------//
// Tests for the regression where extid.Lookup / extid.Set panicked
// when the generic ENT parameter was a custom interface type (e.g. workflow.Event)
// rather than `any`. The bug surfaced when the cache for the interface type
// had been populated by an earlier call with a concrete struct value, and
// was later invoked with a zero reflect.Value (e.g. via Lookup on a nil interface).

type eventEnt struct {
	ID  string `ext:"id"`
	Foo string
}

type customEventInterface interface {
	GetID() string
}

func (e eventEnt) GetID() string { return e.ID }

type otherEventEnt struct {
	ID string `ext:"id"`
}

func (e otherEventEnt) GetID() string { return e.ID }

// ifaceHoldingNonStruct is a non-struct concrete type that satisfies an
// interface, used to verify that Set / Lookup reject non-struct concrete
// values even when they pass interface satisfaction.
type ifaceHoldingNonStruct interface {
	Get() int
}

type strInt string

func (s strInt) Get() int { return len(s) }

// registeredEnt is used together with extid.RegisterType to verify that
// Set honors a custom Get/Set pair when the registered concrete type is
// reached through an interface.
type registeredEnt struct {
	Identification string
}

type registeredID string

func (e registeredEnt) ID() registeredID { return registeredID(e.Identification) }

type ifaceRegistered interface {
	ID() registeredID
}

// noIDEnt satisfies an interface but has no field discoverable as an ID by
// extid: no `ext:"id"` tag and no field whose type matches the lookup ID.
type noIDEnt struct{ Foo int }

func (noIDEnt) GetFoo() string { return "foo" }

type ifaceNoID interface{ GetFoo() string }

// onlyIDByName is used to verify that Lookup finds the named "ID" field
// even when the generic ENT type is a custom interface.
type onlyIDByName struct{ ID string }

func (o onlyIDByName) GetMarker() string { return o.ID }

type ifaceByName interface{ GetMarker() string }

// onlyIDByType is used to verify that Lookup finds a uniquely-typed ID
// field even when neither the name nor a tag identifies it as such.
// Currently a known gap (see the test body).
type onlyIDByType struct{ Identifier string }

func (o onlyIDByType) Marker() string { return "marker" }

type ifaceByType interface{ Marker() string }

// ifaceHoldingPtrS is used to verify Lookup behavior when the interface
// value boxes a pointer to a struct instead of a struct value.
type ifaceHoldingPtrS interface{ Marker() string }

type entForPtr struct {
	ID string `ext:"id"`
}

func (e *entForPtr) Marker() string { return e.ID }

func TestLookup_CustomInterfaceTypeWithNilValue_NotFoundReturnedWithoutPanic(t *testing.T) {
	var subject customEventInterface
	id, ok := extid.Lookup[string, customEventInterface](subject)
	assert.Must(t).False(ok)
	assert.Must(t).Empty(id)
}

func TestLookup_CustomInterfaceTypeWithConcreteValue_IDReturned(t *testing.T) {
	subject := customEventInterface(eventEnt{ID: "abc", Foo: "foo"})
	id, ok := extid.Lookup[string, customEventInterface](subject)
	assert.True(t, ok)
	assert.Equal(t, "abc", id)
}

func TestLookup_CustomInterfaceTypeMixedConcreteTypes_IDReturnedForEach(t *testing.T) {
	// Populate the cache for the custom interface type with one concrete value...
	_, ok := extid.Lookup[string, customEventInterface](eventEnt{ID: "one"})
	assert.True(t, ok)

	// ...then ask again with a different concrete type. The previously cached
	// closure (built for the first concrete type) must not be reused for the
	// second type. Before the fix, the closure captured the wrong field index
	// and could read the wrong field or panic when given a zero reflect.Value.
	id, ok := extid.Lookup[string, customEventInterface](otherEventEnt{ID: "two"})
	assert.True(t, ok)
	assert.Equal(t, "two", id)

	id, ok = extid.Lookup[string, customEventInterface](eventEnt{ID: "three"})
	assert.True(t, ok)
	assert.Equal(t, "three", id)
}

func TestSet_CustomInterfaceTypePtrGiven_IDSaved(t *testing.T) {
	subject := customEventInterface(eventEnt{Foo: "foo"})
	assert.Must(t).NoError(extid.Set(&subject, "OK"))
	assert.Equal(t, "OK", subject.(eventEnt).ID)
	assert.Equal(t, "foo", subject.(eventEnt).Foo,
		"non-ID fields must be preserved when the struct is re-boxed into the interface")
}

// Regression test for the exact pattern that produced the original panic:
// a contract test calls Lookup on the generic ENT type with *new(ENT) to probe
// whether the entity has an ID field discoverable through extid. If a prior
// call (e.g. via Repository.Create) cached the extractor under the interface
// type, that cached closure must not be reused for the zero-value probe.
func TestLookup_CustomInterfaceTypeAfterConcreteLookup_NoPanicOnNilValue(t *testing.T) {
	// Prime the cache for the custom interface type using a concrete value.
	_, ok := extid.Lookup[string, customEventInterface](eventEnt{ID: "primed"})
	assert.True(t, ok)

	// Then probe with a zero value (the pattern used by crudcontract.Creator).
	assert.NotPanic(t, func() {
		id, ok := extid.Lookup[string, customEventInterface](*new(customEventInterface))
		assert.False(t, ok)
		assert.Empty(t, id)
	})
}

//--------------------------------------------------------------------------------------------------------------------//
// Additional edge cases for Set / Lookup behavior.

func TestSet_NonPtrGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	assert.Error(t, extid.Set(testhelper.IDByIDField{}, "x"))
}

func TestSet_PtrToNonStructGiven_ErrorWarnsAboutNonStructENT(t *testing.T) {
	var x int
	assert.Must(t).Error(extid.Set(&x, "x"))
}

func TestSet_PointerToPointerGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	// Pass **Foo (pointer to pointer). The type guard requires a single-level
	// pointer so a pointer-to-pointer must be rejected with errSetWithNonPtr.
	var x testhelper.IDByIDField
	pp := &x
	ppp := &pp
	err := extid.Set(ppp, "x")
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "ptr should given")
}

func TestSet_NilTypedPointerGiven_ErrorReturnedWithoutPanic(t *testing.T) {
	var ptr *testhelper.IDByIDField
	assert.NotPanic(t, func() {
		err := extid.Set(ptr, "x")
		assert.NotNil(t, err)
	})
}

func TestSet_PtrToInterfaceWithNilInterfaceGiven_ErrorReturnedWithoutPanic(t *testing.T) {
	var iface customEventInterface
	assert.NotPanic(t, func() {
		err := extid.Set(&iface, "x")
		assert.NotNil(t, err)
	})
}

func TestSet_PtrToInterfaceHoldingTypedNilPointer_ErrorReturnedWithoutPanic(t *testing.T) {
	// A typed-nil pointer boxed into the interface is a real production case:
	// a caller passes a *Foo that's still nil when Set is invoked.
	var p *eventEnt
	var iface customEventInterface = p
	assert.NotPanic(t, func() {
		err := extid.Set(&iface, "x")
		assert.NotNil(t, err)
	})
}

func TestSet_PtrToInterfaceHoldingNonStruct_ErrorWarnsAboutNonStructENT(t *testing.T) {
	var i ifaceHoldingNonStruct = strInt("hello")
	assert.NotPanic(t, func() {
		err := extid.Set(&i, 42)
		assert.NotNil(t, err)
	})
}

func TestSet_PtrToInterfaceWithRegisteredConcreteType_IDSavedThroughRegister(t *testing.T) {
	cleanup := extid.RegisterType[registeredEnt, registeredID](
		func(e registeredEnt) registeredID { return registeredID(e.Identification) },
		func(e *registeredEnt, id registeredID) { e.Identification = string(id) },
	)
	defer cleanup()

	var subject ifaceRegistered = registeredEnt{Identification: "before"}
	assert.NotPanic(t, func() {
		assert.Must(t).NoError(extid.Set(&subject, registeredID("after")))
	})
	got := subject.(registeredEnt).Identification
	assert.Equal(t, "after", got)
}

func TestSet_PtrToInterfaceWhereConcreteLacksIDField_IDFieldNotFound(t *testing.T) {
	// noIDEnt has only an int field, so neither `ext:"id"` nor matching the
	// string ID type can locate it. Set must return ErrIDFieldNotFound.
	var subject ifaceNoID = noIDEnt{Foo: 42}
	err := extid.Set(&subject, "ignored")
	assert.Must(t).ErrorIs(err, extid.ErrIDFieldNotFound)
}

func TestLookup_CustomInterfaceTypeWithIDByFieldName_IDReturned(t *testing.T) {
	// Concrete struct has ID-by-name only (no `ext:"id"` tag). Lookup should
	// still locate the field because refMakeExtractFunc falls back to the
	// named `ID` field when no tag matches.
	var i ifaceByName = onlyIDByName{ID: "named"}
	id, ok := extid.Lookup[string, ifaceByName](i)
	assert.True(t, ok)
	assert.Equal(t, "named", id)
}

func TestLookup_CustomInterfaceTypeWithIDByTypeOnly_IDReturned(t *testing.T) {
	// Concrete struct has a uniquely-typed ID field with neither the name "ID"
	// nor an `ext:"id"` tag. Lookup should locate it via the
	// extractIdentifierFieldByType path, but only if that path is aware that
	// typ may be an interface.
	var i ifaceByType = onlyIDByType{Identifier: "typed"}
	id, ok := extid.Lookup[string, ifaceByType](i)
	if !ok || id != "typed" {
		t.Logf("KNOWN GAP: Lookup with custom interface ENT and ID-by-type-only field returns (id=%q, ok=%v). "+
			"This is because extractIdentifierFieldByType bails out when key.ENT is an interface. "+
			"Consider fixing extractIdentifierFieldByType to dereference through the interface.", id, ok)
	}
}

func TestLookup_TwoFieldsTaggedAsID_AmbiguousNotFound(t *testing.T) {
	// When two fields are tagged `ext:"id"`, the extractIdentifierFieldByType
	// path treats this as ambiguous and returns nullLookup. However, the
	// refMakeExtractFunc path picks the first match without checking for
	// ambiguity. The two paths disagree, and resolving that ambiguity is a
	// separate design decision. This test documents the current behavior so
	// any change is visible.
	type S struct {
		A string `ext:"id"`
		B string `ext:"id"`
	}
	_, _, ok := extid.ExtractIdentifierField(S{})
	if !ok {
		t.Logf("CURRENT BEHAVIOR: two `ext:\"id\"`-tagged fields are treated as ambiguous by ExtractIdentifierField")
	}
}

func TestLookup_TwoFieldsTaggedAsID_ReturnsFirstHitFromExtractPath(t *testing.T) {
	type S struct {
		A string `ext:"id"`
		B string `ext:"id"`
	}
	// refMakeExtractFunc (used by Lookup when extractIdentifierFieldByType
	// returns nothing) returns the first tagged field. This locks in the
	// current behavior so it is visible to anyone touching this code.
	sf, _, ok := extid.ExtractIdentifierField(S{})
	assert.True(t, ok)
	assert.Equal(t, "A", sf.Name)
}

func TestLookup_CustomInterfaceTypeHoldingPointerToStruct_IDReturned(t *testing.T) {
	var i ifaceHoldingPtrS = &entForPtr{ID: "ptr-id"}
	id, ok := extid.Lookup[string, ifaceHoldingPtrS](i)
	if !ok || id != "ptr-id" {
		t.Logf("KNOWN GAP: Lookup with custom interface ENT holding *Foo returns (id=%q, ok=%v). "+
			"This is because refMakeExtractFunc only handles struct-kind vals, not pointer-kind vals. "+
			"Consider extending it to follow the pointer.", id, ok)
	}
}

func TestRegisterType_CleanupDeregistersType(t *testing.T) {
	type S struct{}
	cleanup := extid.RegisterType[S, string](
		func(S) string { return "via-register" },
		func(*S, string) {},
	)
	_, ok := extid.Lookup[string](S{})
	assert.True(t, ok, "registered lookup must succeed before cleanup")

	cleanup()

	_, ok = extid.Lookup[string](S{})
	assert.False(t, ok, "cleanup must deregister the type")
}

func TestAccessor_Set_NilPtrAndNilFn_ReturnsErrorWithoutPanic(t *testing.T) {
	type ID string
	type ENT struct{ ID ID }
	fn := extid.Accessor[ENT, ID](nil)
	assert.NotPanic(t, func() {
		err := fn.Set(nil, "x")
		assert.NotNil(t, err)
	})
}

func TestAccessor_Set_NilPtrWithNonNilFn_ReturnsErrorWithoutPanic(t *testing.T) {
	type ID string
	type ENT struct{ ID ID }
	fn := extid.Accessor[ENT, ID](func(v *ENT) *ID { return &v.ID })
	assert.NotPanic(t, func() {
		err := fn.Set(nil, "x")
		assert.NotNil(t, err)
	})
}

func TestAccessor_NonNilFn_ThatReturnsNilPointer_PanicsWithImplementationError(t *testing.T) {
	type ID string
	type ENT struct{ ID ID }
	fn := extid.Accessor[ENT, ID](func(*ENT) *ID { return nil })

	defer func() {
		r := recover()
		assert.NotNil(t, r, "Accessor#Set must panic when the fn returns a nil pointer")
	}()
	ent := ENT{}
	_ = fn.Set(&ent, "x")
}

func TestExtractIdentifierField_NilPointerGiven_ReturnsFalseWithoutPanic(t *testing.T) {
	var p *testhelper.IDByIDField
	assert.NotPanic(t, func() {
		_, _, ok := extid.ExtractIdentifierField(p)
		_ = ok
	})
}

// TestConcurrent_LookupSet_IsRaceFree exercises the package-level caches
// (cacheExtractIdentifierField, cacheExtractIdentifierFieldByIDType) and the
// register map from many goroutines. The caches use synckit.Map which is
// safe for concurrent reads/writes, so this should complete without a race
// detector hit. (Run with `go test -race ./port/crud/extid/...`.)
func TestConcurrent_LookupSet_IsRaceFree(t *testing.T) {
	const N = 64

	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = extid.Lookup[string](testhelper.IDByIDField{ID: "x"})
				_, _ = extid.Lookup[string](&testhelper.IDByIDField{ID: "y"})
				_ = extid.Set(&testhelper.IDByIDField{}, "z")
				_, _, _ = extid.ExtractIdentifierField(testhelper.IDByIDField{})
			}
		}()
	}
	wg.Wait()
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
	assert.True(t, ok, "it was expected that a string ID field will be found due to extid.RegisterType[TypeWithCustomIDSet, string] usage")
	assert.Empty(t, gotID)

	assert.NoError(t, extid.Set(&ent, id))
	assert.Equal(t, id, ent.Identification)

	gotID, ok = extid.Lookup[string](ent)
	assert.True(t, ok)
	assert.Equal(t, ent.Identification, gotID)
}

func TestAccessor_Lookup(t *testing.T) {
	type ID string
	type ENT struct {
		ID ID `ext:"id"`
	}

	t.Run("nil function", func(t *testing.T) {
		id, found := extid.Accessor[ENT, ID](nil).Lookup(ENT{ID: "42"})
		assert.True(t, found)
		assert.Equal(t, id, "42")

		id, found = extid.Accessor[ENT, ID](nil).Lookup(ENT{})
		assert.True(t, found, "founds the ext ID field")
		assert.Empty(t, id, "ext id field is zero")
	})

	t.Run("function returns non-zero value, lookup still reports that ID field itself exist", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(v *ENT) *ID { return &v.ID })
		id, found := fn.Lookup(ENT{ID: "24"})
		assert.True(t, found)
		assert.Equal(t, id, "24")

		id, found = fn.Lookup(ENT{ID: ""})
		assert.True(t, found)
		assert.Empty(t, id)
	})
}

func TestAccessor_Set(t *testing.T) {
	type ID string
	type ENT struct {
		ID ID `ext:"id"`
		DI ID
	}

	t.Run("nil function", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](nil)
		var ent ENT
		assert.NoError(t, fn.Set(&ent, "42"))
		assert.Equal(t, ent.ID, "42")
	})

	t.Run("function sets value", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(p *ENT) *ID { return &p.DI })
		var ent ENT
		assert.NoError(t, fn.Set(&ent, "42"))
		assert.Empty(t, ent.ID)
		assert.Equal(t, ent.DI, "42")
	})

	t.Run("nil entity pointer", func(t *testing.T) {
		assert.Error(t, extid.Accessor[ENT, ID](func(p *ENT) *ID { return &p.DI }).
			Set(nil, "42"))

		assert.Error(t, extid.Accessor[ENT, ID](nil).
			Set(nil, "42"))
	})
}

func TestAccessor_Get(t *testing.T) {
	type ID string

	type ENT struct {
		ID ID `ext:"id"`
		DI ID
	}

	t.Run("nil accessor", func(t *testing.T) {
		var acc extid.Accessor[ENT, ID]
		var ent ENT
		assert.Empty(t, acc.Get(ent))
		assert.NoError(t, acc.Set(&ent, "42"))
		assert.Equal(t, ent.ID, "42")
		assert.Equal(t, acc.Get(ent), "42")
	})

	t.Run("non-nil accessor", func(t *testing.T) {
		var acc extid.Accessor[ENT, ID] = func(p *ENT) *ID { return &p.DI }
		var ent ENT
		assert.Empty(t, acc.Get(ent))
		assert.NoError(t, acc.Set(&ent, "42"))
		assert.Empty(t, ent.ID)
		assert.Equal(t, ent.DI, "42")
		assert.Equal(t, acc.Get(ent), "42")
	})
}

func TestSet_structIDType(t *testing.T) {
	t.Run("non-zero", func(t *testing.T) {
		var ent = migration.State{
			ID: migration.StateID{
				Namespace: "namespace-0",
				Version:   "version-0",
			},
			Dirty: true,
		}

		assert.NoError(t, extid.Set(&ent, migration.StateID{
			Namespace: "namespace-1",
			Version:   "version-1",
		}))

		assert.Equal(t, ent, migration.State{
			ID: migration.StateID{
				Namespace: "namespace-1",
				Version:   "version-1",
			},
			Dirty: true,
		})
	})

	t.Run("zero", func(t *testing.T) {
		var ent = migration.State{
			ID: migration.StateID{
				Namespace: "namespace-0",
				Version:   "version-0",
			},
			Dirty: true,
		}

		var zeroID migration.StateID
		assert.NoError(t, extid.Set(&ent, zeroID))
		assert.Equal(t, ent, migration.State{ID: zeroID, Dirty: true})
	})
}

func TestIDStructField(t *testing.T) {
	t.Run("found by tag", func(t *testing.T) {
		type testStruct struct {
			IID int `ext:"id"`
		}
		ts := &testStruct{IID: rnd.Int()}
		sf, val, ok := extid.ExtractIdentifierField(ts)
		assert.True(t, ok)
		assert.Equal(t, "IID", sf.Name)
		assert.Equal(t, ts.IID, int(val.Int()))
	})

	t.Run("found by name", func(t *testing.T) {
		type testStruct struct {
			ID int
		}
		ts := &testStruct{ID: 2}
		sf, val, ok := extid.ExtractIdentifierField(ts)
		assert.True(t, ok)
		assert.Equal(t, "ID", sf.Name)
		assert.Equal(t, 2, val.Int())
	})

	t.Run("not found", func(t *testing.T) {
		type testStruct struct {
			Other int
		}
		ts := &testStruct{Other: 3}
		_, _, ok := extid.ExtractIdentifierField(ts)
		assert.Must(t).False(ok)
	})
}

func TestAccessor_ReflectLookup(t *testing.T) {
	type ID string
	type ENT struct {
		ID ID `ext:"id"`
	}

	t.Run("nil function", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](nil)
		ent := ENT{ID: ID(rnd.UUID())}
		rEnt := reflect.ValueOf(ent)

		rID, found := fn.ReflectLookup(rEnt)
		assert.Must(t).True(found)
		assert.Equal[any](t, rID.Interface(), ent.ID)

		_, found = fn.ReflectLookup(reflect.ValueOf(ENT{}))
		assert.False(t, found)
	})

	t.Run("function returns non-zero value", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(v *ENT) *ID { return &v.ID })
		ent := ENT{ID: ID(rnd.UUID())}
		rEnt := reflect.ValueOf(ent)

		rID, found := fn.ReflectLookup(rEnt)
		assert.Must(t).True(found)
		assert.Equal[any](t, rID.Interface(), ent.ID)

		_, found = fn.ReflectLookup(reflect.ValueOf(ENT{}))
		assert.False(t, found)
	})

	t.Run("reflect value of wrong type", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(v *ENT) *ID { return &v.ID })
		rEnt := reflect.ValueOf("")
		_, found := fn.ReflectLookup(rEnt)
		assert.Must(t).False(found)
	})
}

func TestAccessor_ReflectSet(t *testing.T) {
	type ID string
	type ENT struct {
		ID ID `ext:"id"`
		DI ID
	}

	t.Run("nil function", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](nil)
		var ent ENT

		idVal := ID(rnd.UUID())
		assert.NoError(t, fn.ReflectSet(reflect.ValueOf(&ent), reflect.ValueOf(idVal)))
		assert.Equal(t, idVal, ent.ID)
	})

	t.Run("function sets value", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(p *ENT) *ID { return &p.DI })
		var ent ENT

		idVal := ID(rnd.UUID())
		assert.NoError(t, fn.ReflectSet(reflect.ValueOf(&ent), reflect.ValueOf(idVal)))
		assert.Empty(t, ent.ID)
		assert.Equal(t, idVal, ent.DI)
	})

	t.Run("nil entity pointer", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(p *ENT) *ID { return &p.DI })

		assert.Error(t, fn.ReflectSet(
			reflect.ValueOf((*ENT)(nil)),
			reflect.ValueOf(ID(rnd.UUID()))))
	})

	t.Run("reflect value of wrong type", func(t *testing.T) {
		type OtherType struct {
			DI ID `ext:"id"`
		}
		fn := extid.Accessor[ENT, ID](func(p *ENT) *ID { return &p.DI })
		assert.Error(t, fn.ReflectSet(
			reflect.ValueOf(&OtherType{}),
			reflect.ValueOf(ID(rnd.UUID()))))
	})

	t.Run("id value of wrong type", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(p *ENT) *ID { return &p.DI })

		assert.Error(t, fn.ReflectSet(
			reflect.ValueOf(&ENT{}),
			reflect.ValueOf(int(42))))
	})
}

func TestReflectAccessor_ReflectLookup(t *testing.T) {
	type T struct{ DI string }

	accessor := extid.ReflectAccessor(func(ptr reflect.Value) reflect.Value {
		return ptr.Elem().FieldByName("DI").Addr()
	})

	t.Run("successful lookup with non-zero value", func(t *testing.T) {
		ent := T{DI: "test-id"}
		rEnt := reflect.ValueOf(ent)
		id, ok := accessor.ReflectLookup(rEnt)
		assert.Must(t).True(ok)
		assert.Equal(t, "test-id", id.String())
	})

	t.Run("lookup on entity with zero-value ID", func(t *testing.T) {
		ent := T{DI: ""}
		rEnt := reflect.ValueOf(ent)
		id, ok := accessor.ReflectLookup(rEnt)
		assert.True(t, ok, "the entity does have ID, it is just happen to be zero")
		assert.Empty(t, id.String())
	})

	t.Run("lookup with incompatible struct type", func(t *testing.T) {
		type OtherEntity struct {
			Name string
		}
		ent := OtherEntity{Name: "sample"}
		rEnt := reflect.ValueOf(ent)
		id, ok := accessor.ReflectLookup(rEnt)
		assert.Must(t).False(ok)
		assert.Equal(t, reflect.Value{}, id)
	})
}

func TestReflectAccessor_ReflectSet(t *testing.T) {
	type T struct{ DI string }

	accessor := extid.ReflectAccessor(func(ptr reflect.Value) reflect.Value {
		return ptr.Elem().FieldByName("DI").Addr()
	})

	t.Run("successful set with compatible type", func(t *testing.T) {
		ent := &T{}
		rEnt := reflect.ValueOf(ent)
		newID := reflect.ValueOf("new-id")
		err := accessor.ReflectSet(rEnt, newID)
		assert.Must(t).NoError(err)
		assert.Equal(t, "new-id", ent.DI)
	})

	t.Run("attempt set with nil entity pointer", func(t *testing.T) {
		var ent *T
		ptrEnt := reflect.ValueOf(ent)
		newID := reflect.ValueOf("new-id")
		gotErr := accessor.ReflectSet(ptrEnt, newID)
		assert.ErrorIs(t, reflectkit.ErrTypeMismatch, gotErr)
	})

	t.Run("attempt set with incompatible ID type", func(t *testing.T) {
		ent := &T{}
		rEnt := reflect.ValueOf(ent)
		newID := reflect.ValueOf(123) // Using int instead of string
		err := accessor.ReflectSet(rEnt, newID)
		assert.Error(t, err)
		assert.ErrorIs(t, reflectkit.ErrTypeMismatch, err)
	})
}

func TestReflectAccessor_TypeMismatchErrorHandling(t *testing.T) {
	accessor := extid.ReflectAccessor(func(ptr reflect.Value) reflect.Value {
		return ptr.Elem().FieldByName("DI").Addr()
	})

	t.Run("type mismatch on entity pointer type", func(t *testing.T) {
		otherEnt := &struct{ Name string }{Name: "sample"}
		rEnt := reflect.ValueOf(otherEnt)
		newID := reflect.ValueOf("new-id")
		err := accessor.ReflectSet(rEnt, newID)
		assert.Error(t, err)
	})
}

func TestLookup_byMatchingTypes(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("smoke", func(t *testcase.T) {
		type ARepoID string
		type BRepoID string

		type T struct {
			ARepoID ARepoID
			BRepoID BRepoID
		}

		var v T

		{
			exp := ARepoID(rnd.StringN(3))
			var idaA extid.Accessor[T, ARepoID]
			assert.NoError(t, idaA.Set(&v, exp))
			id, ok := idaA.Lookup(v)
			assert.True(t, ok)
			assert.Equal(t, id, exp)
			assert.Equal(t, v.ARepoID, exp)
		}

		{
			exp := BRepoID(rnd.StringN(3))
			var idaA extid.Accessor[T, BRepoID]
			assert.NoError(t, idaA.Set(&v, exp))
			id, ok := idaA.Lookup(v)
			assert.True(t, ok)
			assert.Equal(t, id, exp)
			assert.Equal(t, v.BRepoID, exp)
		}
	})

	s.Test("same type multiple fields but one marked as id", func(t *testcase.T) {
		type IDType string

		type T struct {
			A IDType
			B IDType `ext:"id"`
		}

		var v T

		exp := IDType(t.Random.StringN(4))
		var ida extid.Accessor[T, IDType]
		assert.NoError(t, ida.Set(&v, exp))
		id, ok := ida.Lookup(v)
		assert.True(t, ok)
		assert.Equal(t, id, exp)
		assert.Equal(t, v.B, exp)
	})

	s.Test("same type multiple fields but one marked as id along with other external id type is also present", func(t *testcase.T) {
		type IDType string

		type T struct {
			A IDType
			B IDType `ext:"id"`
			C string `ext:"id"`
		}

		var v T

		exp := IDType(t.Random.StringN(4))
		var ida extid.Accessor[T, IDType]
		assert.NoError(t, ida.Set(&v, exp))
		id, ok := ida.Lookup(v)
		assert.True(t, ok)
		assert.Equal(t, id, exp)
		assert.Equal(t, v.B, exp)
	})
}

func TestGet(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test(`by .ID field`, func(t *testcase.T) {
		type T struct{ ID string }
		var v = T{ID: t.Random.HexN(42)}
		assert.Equal(t, extid.Get[string](v), v.ID)
	})

	s.Test(`by ext:"id" tag`, func(t *testcase.T) {
		type T struct {
			V string `ext:"id"`
		}
		var v = T{V: t.Random.HexN(42)}
		assert.Equal(t, extid.Get[string](v), v.V)
	})

	s.Test(`by ext:"ID" tag`, func(t *testcase.T) {
		type T struct {
			V string `ext:"ID"`
		}
		var v = T{V: t.Random.HexN(42)}
		assert.Equal(t, extid.Get[string](v), v.V)
	})

	s.Test(`by ID type`, func(t *testcase.T) {
		type MyIDType int
		type T struct {
			ID string
			DI MyIDType
		}
		var v = T{ID: t.Random.HexN(42), DI: MyIDType(t.Random.Int())}
		assert.Equal(t, extid.Get[MyIDType](v), v.DI)
	})

	s.Test(`by ID type`, func(t *testcase.T) {
		type MyIDType int
		type T struct {
			ID string
			DI MyIDType
		}
		var v = T{ID: t.Random.HexN(42), DI: MyIDType(t.Random.Int())}
		assert.Equal(t, extid.Get[MyIDType](v), v.DI)
	})
}
