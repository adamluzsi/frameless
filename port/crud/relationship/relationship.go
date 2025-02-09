// Package erm stands for Entity-Relationship Modeling
package relationship

import (
	"fmt"
	"reflect"
	"strings"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/crud/extid"
)

// BelongsTo is a HasMany relationship, where "Who" belongs to "ToWhom" through the "ToWhomID".
func BelongsTo[Who, ToWhom any, ToWhomID comparable](accessor extid.Accessor[Who, ToWhomID]) func() {
	var (
		WhoType    = reflectkit.TypeOf[Who]()
		ToWhomType = reflectkit.TypeOf[ToWhom]()
	)
	return register(ToWhomType, WhoType, func(r *_Record) {
		r.BelongsTo = accessor
	})
}

// ReferencesMany is a HasMany relationship, where A references many B by a slice of B IDs.
func ReferencesMany[A, B any, BID comparable](accessor extid.Accessor[A, []BID]) func() {
	var (
		AType   = reflectkit.TypeOf[A]()
		BType   = reflectkit.TypeOf[B]()
		BIDType = reflectkit.TypeOf[BID]()
	)

	sf, _, _ := extid.ExtractIdentifierField(reflect.New(BType).Elem())
	if sf.Type != BIDType {
		panic(fmt.Errorf("implementation error: %s doesn't have a %s id field", BType.String(), BIDType.String()))
	}

	return register(AType, BType, func(r *_Record) {
		r.ReferencesMany = accessor
	})
}

func register(WhoType, ToWhoType reflect.Type, conf func(r *_Record)) func() {
	var (
		key        = typeMatch{A: WhoType, B: ToWhoType}
		_, existed = registry.Lookup(key)
		current    = getRecord(key)
		prev       = current // pass by value copy
	)
	conf(&current)
	registry.Set(key, current)
	return func() {
		if existed {
			registry.Set(key, prev) // restore previous
		} else {
			registry.Delete(key)
		}
	}
}

var registry synckit.Map[typeMatch, _Record]

type typeMatch struct {
	A reflect.Type
	B reflect.Type
}

type _Record struct {
	A, B           reflect.Type
	BelongsTo      accessor // func(*B) *AID
	ReferencesMany accessor // func(*A) *[]BID
}

type accessor interface {
	ReflectLookup(rENT reflect.Value) (rID reflect.Value, ok bool)
	ReflectSet(ptrENT reflect.Value, id reflect.Value) error
}

func (r _Record) Empty() bool {
	return r.A == nil && r.B == nil && r.BelongsTo == nil && r.ReferencesMany == nil
}

func Related(a, b any) bool {
	var (
		rA = reflectkit.BaseValueOf(a)
		rB = reflectkit.BaseValueOf(b)
	)
	return checkRelationBetween(rA, rB) ||
		checkRelationBetween(rB, rA)
}

func checkRelationBetween(a, b reflect.Value) bool {
	r := getRecord(typeMatch{A: a.Type(), B: b.Type()})

	return belongsToCheck(r, a, b) ||
		referenceManyCheck(r, a, b)
}

func getRecord(key typeMatch) _Record {
	if key.A.Kind() != reflect.Struct || key.B.Kind() != reflect.Struct { // to avoid caching all possible pointless type variations
		return _Record{}
	}
	return registry.GetOrInit(key, func() _Record {
		return defaultRecord(key.A, key.B)
	})
}

func belongsToCheck(r _Record, rA, rB reflect.Value) (ok bool) {
	if r.BelongsTo == nil {
		return false
	}
	_, expAID, ok := extid.ExtractIdentifierField(rA)
	if !ok {
		return false
	}
	gotAID, ok := r.BelongsTo.ReflectLookup(rB)
	if !ok {
		return false
	}
	return reflectkit.Equal(expAID, gotAID)
}

func referenceManyCheck(r _Record, a, b reflect.Value) (ok bool) {
	if r.ReferencesMany == nil {
		return false
	}
	_, expBID, ok := extid.ExtractIdentifierField(b)
	if !ok {
		return false
	}
	bIDs, ok := r.ReferencesMany.ReflectLookup(a)
	if !ok {
		return false
	}
	for i := 0; i < bIDs.Len(); i++ {
		gotBID := bIDs.Index(i)

		if reflectkit.Equal(expBID, gotBID) {
			return true
		}
	}
	return false
}

func defaultRecord(a, b reflect.Type) _Record {
	var r = _Record{
		A: a,
		B: b,
	}

	if a.Kind() != reflect.Struct || b.Kind() != reflect.Struct {
		return r
	}

	aExtIDStructField, _, ok := extid.ExtractIdentifierField(reflect.New(a).Elem())
	if !ok {
		return r
	}

	bExtIDStructField, _, ok := extid.ExtractIdentifierField(reflect.New(b).Elem())
	if !ok {
		return r
	}

	{
		var (
			aIDType = aExtIDStructField.Type
			// aIDSliceType = reflect.SliceOf(aExtIDStructField.Type)
			isBuiltIn = isBuiltInType(aIDType)
		)

	scanB:
		for bFieldIndex := 0; bFieldIndex < b.NumField(); bFieldIndex++ {
			var bField = b.Field(bFieldIndex)

			if isBuiltIn { // if it is a primitive, we must ensure that the field prefix is there
				var bStructField = b.Field(bFieldIndex)
				if !strings.HasPrefix(bStructField.Name, a.Name()) {
					continue
				}
			}

			// check if "A" is in a 1:N relationship with "B" by "B" having "A.ID"
			if bField.Type == aIDType {
				bFieldIndexForAID := bFieldIndex
				r.BelongsTo = extid.ReflectAccessor(func(ptrB reflect.Value) (ptrAID reflect.Value) {
					return ptrB.Elem().Field(bFieldIndexForAID).Addr()
				})
				break scanB // TODO: check if support for more might be needed
			}
		}
	}

	{
		var (
			bIDType      = bExtIDStructField.Type
			bIDSliceType = reflect.SliceOf(bIDType)
			isBuiltIn    = isBuiltInType(bIDType)
		)

	scanA:
		for aFieldIndex := 0; aFieldIndex < a.NumField(); aFieldIndex++ {
			var aField = a.Field(aFieldIndex)

			if isBuiltIn { // if it is a primitive, we must ensure that the field prefix is there
				var aStructField = a.Field(aFieldIndex)
				if !strings.HasPrefix(aStructField.Name, b.Name()) {
					continue
				}
			}

			// check if "B" has 1:N relationship with "A" by an "A" ids field
			if aField.Type == bIDSliceType {
				aFieldIndexForBIDs := aFieldIndex
				r.ReferencesMany = extid.ReflectAccessor(func(ptrA reflect.Value) (ptrSliceOfBIDs reflect.Value) {
					return ptrA.Elem().Field(aFieldIndexForBIDs).Addr()
				})
				break scanA
			}
		}
	}

	return r
}

func isBuiltInType(t reflect.Type) bool {
	return t.PkgPath() == ""
}

func Associate(a, b any) error {
	var (
		rA = reflectkit.ToValue(a)
		rB = reflectkit.ToValue(b)
	)
	checkPointerGotTheRightImplementation(rA)
	checkPointerGotTheRightImplementation(rB)
	rA = reflectkit.BaseValue(rA)
	rB = reflectkit.BaseValue(rB)
	if err := associate(rA, rB); err != nil {
		return err
	}
	if err := associate(rB, rA); err != nil {
		return err
	}
	return nil
}

func checkPointerGotTheRightImplementation(ptr reflect.Value) {
	if ptr.Kind() != reflect.Pointer {
		panic(fmt.Errorf("%w: relationship.Associate works with entity pointers, but got %s", reflectkit.ErrTypeMismatch, ptr.Type().String()))
	}
}

func associate(a, b reflect.Value) error {
	r := getRecord(typeMatch{A: a.Type(), B: b.Type()})

	if r.BelongsTo != nil {
		_, aID, ok := extid.ExtractIdentifierField(a)
		if ok {
			if err := r.BelongsTo.ReflectSet(b.Addr(), aID); err != nil {
				return fmt.Errorf("error while setting BelongsTo relationship between %s and %s: %w", a.Type(), b.Type(), err)
			}
		}
	}

	if r.ReferencesMany != nil {
		bIDSF, bID, ok := extid.ExtractIdentifierField(b)
		if ok && !reflectkit.IsEmpty(bID) {
			bIDSliceType := reflect.SliceOf(bIDSF.Type)
			bIDs, _ := r.ReferencesMany.ReflectLookup(a)
			if bIDs.Type() != bIDSliceType {
				bIDs = reflect.MakeSlice(bIDSliceType, 0, 1)
			}
			bIDs = reflect.Append(bIDs, bID)
			if err := r.ReferencesMany.ReflectSet(a.Addr(), bIDs); err != nil {
				return err
			}
		}
	}

	return nil
}

func HasReference(from, to any) bool {
	var (
		rWho    = reflectkit.BaseValueOf(from)
		rToWhom = reflectkit.BaseValueOf(to)
	)
	if getRecord(typeMatch{B: rWho.Type(), A: rToWhom.Type()}).BelongsTo != nil {
		return true
	}
	if getRecord(typeMatch{A: rWho.Type(), B: rToWhom.Type()}).ReferencesMany != nil {
		return true
	}
	return false
}
