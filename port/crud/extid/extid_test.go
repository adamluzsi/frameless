package extid_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud/extid/internal/testhelper"
	"go.llib.dev/frameless/port/migration"
	"go.llib.dev/frameless/spechelper/testent"

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
	b.Run("ExtractIdentifierField", func(b *testing.B) {
		b.Run("id by ID field", func(b *testing.B) {
			extid.ExtractIdentifierField(IDByField{})
			vs := random.Slice(b.N, func() IDByField {
				return IDByField{ID: rnd.String()}
			})
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				extid.ExtractIdentifierField(vs[i])
			}
		})
		b.Run("id by tag", func(b *testing.B) {
			extid.ExtractIdentifierField(IDByTag{})
			vs := random.Slice(b.N, func() IDByTag {
				return IDByTag{IDD: rnd.String()}
			})
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
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

			for i := 0; i < b.N; i++ {
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

			for i := 0; i < b.N; i++ {
				v := vs[i]
				accessor.Set(&v, v.ID)
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
		assert.False(t, ok)
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

func TestLookup_UnidentifiableIDGiven_NotFoundReturnedAsBoolean(t *testing.T) {
	id, ok := extid.Lookup[any](testhelper.UnidentifiableID{UserID: "ok"})
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
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_PointerTypeThatIsNotInitialized_NotFoundReturned(t *testing.T) {
	id, ok := extid.Lookup[*string](&testhelper.IDAsPointer{})
	assert.Must(t).False(ok)
	assert.Must(t).Nil(id)
}

func TestLookup_PointerTypeWithValue_ValueReturned(t *testing.T) {
	idVal := `foo`
	id, ok := extid.Lookup[*string](&testhelper.IDAsPointer{ID: &idVal})
	assert.True(t, ok)
	assert.Equal(t, &idVal, id)
}

func TestLookup_IDFieldWithZeroValueFound_NotOkReturned(t *testing.T) {
	_, ok := extid.Lookup[string](testhelper.IDByIDField{ID: ""})
	assert.Must(t).False(ok, "zero value should be not OK")
}

// ------------------------------------------------------------------------------------------------------------------ //

func TestSet_NonPtrStructGiven_ErrorWarnsAboutNonPtrObject(t *testing.T) {
	assert.Must(t).Error(extid.Set(testhelper.IDByIDField{}, "Set doesn't work with pass by value"))
}

func TestSet_PtrStructGivenButIDIsCannotBeIdentified_ErrorWarnsAboutMissingIDFieldOrTagName(t *testing.T) {
	assert.Must(t).NotNil(extid.Set(&testhelper.UnidentifiableID{}, "Cannot be passed because the missing ID Field or Tag spec"))
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
	assert.False(t, ok)
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
		assert.False(t, found)
		assert.Empty(t, id)
	})

	t.Run("function returns non-zero value", func(t *testing.T) {
		fn := extid.Accessor[ENT, ID](func(v *ENT) *ID { return &v.ID })
		id, found := fn.Lookup(ENT{ID: "24"})
		assert.True(t, found)
		assert.Equal(t, id, "24")

		id, found = fn.Lookup(ENT{ID: ""})
		assert.False(t, found)
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

	t.Run("", func(t *testing.T) {

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
		assert.Must(t).False(ok)
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
