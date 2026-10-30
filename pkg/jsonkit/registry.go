package jsonkit

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"go.llib.dev/frameless/pkg/reflectkit"
)

var (
	_ = RegisterTypeID[int]("int", "integer")
	_ = RegisterTypeID[int8]("int8")
	_ = RegisterTypeID[int16]("int16")
	_ = RegisterTypeID[int32]("int32")
	_ = RegisterTypeID[int64]("int64")
	_ = RegisterTypeID[uint]("uint")
	_ = RegisterTypeID[uint8]("uint8")
	_ = RegisterTypeID[uint16]("uint16")
	_ = RegisterTypeID[uint32]("uint32")
	_ = RegisterTypeID[uint64]("uint64")
	_ = RegisterTypeID[uintptr]("uintptr")
	_ = RegisterTypeID[float32]("float32")
	_ = RegisterTypeID[float64]("float64")
	_ = RegisterTypeID[complex64]("complex64")
	_ = RegisterTypeID[complex128]("complex128")
	_ = RegisterTypeID[bool]("bool", "boolean")
	_ = RegisterTypeID[string]("string")
)

// RegisterTypeID registers T with the type ID id, and its aliases, on every Codec, see CodecRegisterTypeID.
// Like there, the returned function removes this registration, and only this one.
func RegisterTypeID[T any](id TypeID, aliases ...TypeID) func() {
	rType := reflectkit.TypeOf[T]()
	return typeIDRegistry.Register(rType, id, aliases...)
}

func LookupTypeID[T any]() (TypeID, bool) {
	rType := reflectkit.TypeOf[T]()
	return typeIDRegistry.TypeIDByType(rType)
}

var typeIDRegistry _TypeRegistry

// customCodecEntry holds the user-supplied Marshal/Unmarshal closures for a
// type that was registered with CodecRegister. The closures are stored as
// runtime values (interface{}-shaped) so the registry can hold entries for any
// type without the registry itself becoming generic. The Marshal/Unmarshal
// adapters in codec.go type-assert back to func(c *Codec, v T) ([]byte, error)
// / func(c *Codec, data []byte, p *T) error when invoking them.
type customCodecEntry struct {
	TypeID    TypeID
	Marshal   func(c *Codec, v any) ([]byte, error)
	Unmarshal func(c *Codec, data []byte, p any) error
}

type _TypeRegistry struct {
	mutex    sync.RWMutex
	init     sync.Once
	byType   map[reflect.Type]TypeID
	byTypeID map[TypeID]reflect.Type
	// typeIDHolds counts the registrations which hold the type ID of a type, made with Register or SetCustomCodec,
	// so the type ID is removed with the last of them, whichever order they are removed in.
	typeIDHolds map[reflect.Type]int
	byAlias     map[TypeID]TypeID
	// customCodec holds the custom codec in effect for a type: the last of its customCodecs.
	customCodec map[reflect.Type]*customCodecEntry
	// customCodecs holds the custom codecs registered for a type, in the order they were registered,
	// so removing one of them leaves the others in place.
	customCodecs map[reflect.Type][]*customCodecEntry
	// migration holds the retired type IDs registered with CodecRegisterMigration.
	// They resolve to a type during unmarshal, but never take part in marshal.
	migration map[TypeID]*migrationEntry
}

type migrationEntry struct {
	Type      reflect.Type
	Unmarshal func(c *Codec, data []byte, p any) error
}

func (r *_TypeRegistry) Init() {
	r.mutex.RLock()
	ok := r.byType != nil
	r.mutex.RUnlock()
	if ok {
		return
	}
	r.init.Do(func() {
		r.byType = make(map[reflect.Type]TypeID)
		r.byTypeID = make(map[TypeID]reflect.Type)
		r.typeIDHolds = make(map[reflect.Type]int)
		r.byAlias = make(map[TypeID]TypeID)
		r.customCodec = make(map[reflect.Type]*customCodecEntry)
		r.customCodecs = make(map[reflect.Type][]*customCodecEntry)
		r.migration = make(map[TypeID]*migrationEntry)
	})
}

// SetMigration registers a retired type ID whose data is migrated into entry.Type on unmarshal.
// It panics if the ID is already in use as a type ID, alias or migration in this registry.
func (r *_TypeRegistry) SetMigration(id TypeID, entry *migrationEntry) func() {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	entry.Type = base(entry.Type)
	_, isTypeID := r.byTypeID[id]
	_, isAlias := r.byAlias[id]
	_, isMigration := r.migration[id]
	if isTypeID || isAlias || isMigration {
		panic(fmt.Sprintf("Unable to register %q $type migration for %s, because the $type id is already registered", id, entry.Type.String()))
	}
	r.migration[id] = entry
	return sync.OnceFunc(func() {
		r.mutex.Lock()
		defer r.mutex.Unlock()
		delete(r.migration, id)
	})
}

// hasDecoder reports whether t has a custom codec or any migration in this registry.
func (r *_TypeRegistry) hasDecoder(t reflect.Type) bool {
	if r == nil {
		return false
	}
	r.Init()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if len(r.customCodec) == 0 && len(r.migration) == 0 {
		return false
	}
	t = base(t)
	if r.customCodec[t] != nil {
		return true
	}
	for _, entry := range r.migration {
		if entry.Type == t {
			return true
		}
	}
	return false
}

// lookupDecoder returns the unmarshal closure for a value of type t that arrived under the envTypeID $type.
// A migration registered for envTypeID and t wins over the type's custom codec.
// Nil-safe; returns nil when neither is registered.
func (r *_TypeRegistry) lookupDecoder(t reflect.Type, envTypeID TypeID) func(c *Codec, data []byte, p any) error {
	if r == nil {
		return nil
	}
	r.Init()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if envTypeID != "" {
		if entry, ok := r.migration[envTypeID]; ok && entry.Type == base(t) {
			return entry.Unmarshal
		}
	}
	if entry := r.customCodec[base(t)]; entry != nil {
		return entry.Unmarshal
	}
	return nil
}

// SetCustomCodec stores a per-registry custom Marshal/Unmarshal pair for the
// given concrete type and returns a function that removes it.
// Panics if the type was already registered with a different ID, mirroring the
// behaviour of Register so a misuse is loud rather than silent.
//
// The custom codec registered last is in effect.
// Removing one leaves the others in place, so the last of the remaining ones is in effect,
// and the type ID stays registered as long as a registration holds it, see holdTypeID.
func (r *_TypeRegistry) SetCustomCodec(dtoType reflect.Type, entry *customCodecEntry) func() {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	dtoType = base(dtoType)
	if existingID, ok := r.byType[dtoType]; ok && existingID != entry.TypeID {
		panic(fmt.Sprintf("Unable to register %q $type id for %s, because it is already registered with %s",
			entry.TypeID, dtoType.String(), existingID))
	}
	r.holdTypeID(dtoType, entry.TypeID)
	r.customCodecs[dtoType] = append(r.customCodecs[dtoType], entry)
	r.customCodec[dtoType] = entry
	return sync.OnceFunc(func() {
		r.mutex.Lock()
		defer r.mutex.Unlock()
		remaining := slices.DeleteFunc(r.customCodecs[dtoType], func(e *customCodecEntry) bool {
			return e == entry
		})
		if len(remaining) == 0 {
			delete(r.customCodecs, dtoType)
			delete(r.customCodec, dtoType)
		} else {
			r.customCodecs[dtoType] = remaining
			r.customCodec[dtoType] = remaining[len(remaining)-1]
		}
		r.releaseTypeID(dtoType, entry.TypeID)
	})
}

// holdTypeID registers id as the type ID of t for one more registration.
// The caller holds the lock, and made sure t has no other type ID.
func (r *_TypeRegistry) holdTypeID(t reflect.Type, id TypeID) {
	r.byType[t] = id
	r.byTypeID[id] = t
	r.typeIDHolds[t]++
}

// releaseTypeID drops the hold of a registration on the type ID of t,
// and removes the type ID with the last one, so no registration takes the type ID away from another.
// The caller holds the lock.
func (r *_TypeRegistry) releaseTypeID(t reflect.Type, id TypeID) {
	r.typeIDHolds[t]--
	if 0 < r.typeIDHolds[t] {
		return
	}
	delete(r.typeIDHolds, t)
	delete(r.byType, t)
	delete(r.byTypeID, id)
}

// hasCustomCodecs reports whether the registry has a custom codec, and with decoding,
// whether it has a custom codec or a migration, which read a value otherwise than encoding/json.
func (r *_TypeRegistry) hasCustomCodecs(decoding bool) bool {
	if r == nil {
		return false
	}
	r.Init()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return 0 < len(r.customCodec) || (decoding && 0 < len(r.migration))
}

// LookupCustomCodec returns the custom codec entry registered for the given
// type, or nil if none was registered in this registry. Nil-safe: a nil
// receiver returns nil so callers can invoke this unconditionally during
// marshal/unmarshal without a nil check at every site.
func (r *_TypeRegistry) LookupCustomCodec(t reflect.Type) *customCodecEntry {
	if r == nil {
		return nil
	}
	r.Init()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.customCodec == nil {
		return nil
	}
	return r.customCodec[base(t)]
}

// Register registers id as the type ID of dtoType, and links the aliases to it.
// When dtoType is registered with id already, like with SetCustomCodec, it only links the aliases.
// The returned function removes what this call registered, and only that, see holdTypeID.
func (r *_TypeRegistry) Register(dtoType reflect.Type, id TypeID, aliases ...TypeID) func() {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	dtoType = base(dtoType)
	gotID, isRegistered := r.typeIDByType(dtoType)
	if isRegistered && gotID != id {
		const format = "Unable to register %q $type id for %s, because it is already registered with %s"
		panic(fmt.Sprintf(format, id, dtoType.String(), gotID))
	}
	// the aliases are checked before anything is registered, so a misuse leaves the registry as it was.
	for _, alias := range aliases {
		if _, isAlias := r.byAlias[alias]; isAlias {
			const format = "Unable to register %q $type alias for %s"
			panic(fmt.Sprintf(format, alias, dtoType.String()))
		}
		if _, isTypeID := r.byTypeID[alias]; isTypeID || alias == id {
			const format = "Unable to register %q $type alias because it is an already registered $type id"
			panic(fmt.Sprintf(format, alias))
		}
	}
	r.holdTypeID(dtoType, id)
	for _, alias := range aliases {
		r.byAlias[alias] = id
	}
	return sync.OnceFunc(func() {
		r.mutex.Lock()
		defer r.mutex.Unlock()
		for _, alias := range aliases {
			delete(r.byAlias, alias)
		}
		r.releaseTypeID(dtoType, id)
	})
}

func (r *_TypeRegistry) TypeIDFor(v any) (TypeID, bool) {
	return r.TypeIDByType(reflect.TypeOf(v))
}

func (r *_TypeRegistry) TypeIDByType(typ reflect.Type) (TypeID, bool) {
	r.Init()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.typeIDByType(typ)
}

func (r *_TypeRegistry) typeIDByType(typ reflect.Type) (TypeID, bool) {
	if typ == nil {
		return "", false
	}
	if r.byType == nil {
		return *new(TypeID), false
	}
	id, ok := r.byType[base(typ)]
	return id, ok
}

func (r *_TypeRegistry) TypeByID(id TypeID) (reflect.Type, bool) {
	r.Init()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r.byTypeID == nil {
		return nil, false
	}
	rType, ok := r.byTypeID[id]
	if ok {
		return rType, true
	}
	if ogID, knownAlias := r.byAlias[id]; knownAlias {
		rType, ok = r.byTypeID[ogID]
		return rType, ok
	}
	if entry, knownMigration := r.migration[id]; knownMigration {
		return entry.Type, true
	}
	return rType, ok
}

// lookupTypeIDForValue looks up the typeID for a value during MARSHAL.
// Marshal must be scoped to the codec's own registry plus the package-level
// global registry; cross-codec registrations visible to other codecs must
// not bleed into marshal, otherwise a type registered on one codec would
// silently marshal from another. A nil codec registry means "no
// codec-specific registrations".
func lookupTypeIDForValue(codecReg *_TypeRegistry, value any) (TypeID, bool) {
	if codecReg != nil {
		if id, ok := codecReg.TypeIDFor(value); ok {
			return id, true
		}
	}
	return typeIDRegistry.TypeIDFor(value)
}

// lookupTypeIDForHeldValue returns the type ID which value is written with, when an interface holds it:
// the type ID its type is registered with, or else the one derived for its type, see derivedTypeID.
func lookupTypeIDForHeldValue(codecReg *_TypeRegistry, value any) (TypeID, bool) {
	if id, ok := lookupTypeIDForValue(codecReg, value); ok {
		return id, true
	}
	if value == nil {
		return "", false
	}
	// Like a registered type, which is looked up without its pointers, a *[]T is written as a []T.
	return derivedTypeID(codecReg, base(reflect.TypeOf(value)))
}

// lookupTypeIDForType is the type-only counterpart of lookupTypeIDForValue.
// Like lookupTypeIDForValue, it is used during MARSHAL and therefore does
// not consult the shared cross-codec registry.
// It only finds the types which are registered themselves, so no type ID is derived for a type, see derivedTypeID.
func lookupTypeIDForType(codecReg *_TypeRegistry, typ reflect.Type) (TypeID, bool) {
	if codecReg != nil {
		if id, ok := codecReg.TypeIDByType(typ); ok {
			return id, true
		}
	}
	return typeIDRegistry.TypeIDByType(typ)
}

// lookupTypeByIDForCodec looks up a reflect.Type by typeID during UNMARSHAL.
// Unlike marshal, unmarshal consults the shared cross-codec registry so that
// data marshaled by one codec can be reconstructed by another codec that
// received it via e.g. an HTTP request.
//
// It finds the type of a value, so it doesn't find an interface type, even if it is registered,
// as no value is of an interface type; its type ID only names it as the element type of a slice or a map.
// A type ID which is not registered, but derived, like "[]item" for a []Item, is read back into its type, see derivedTypeID.
func lookupTypeByIDForCodec(codecReg *_TypeRegistry, id TypeID) (reflect.Type, bool) {
	rType, ok := lookupTypeByID(codecReg, id, 0)
	if !ok || rType.Kind() == reflect.Interface {
		return nil, false
	}
	return rType, true
}

// lookupTypeByID looks up the type of the type ID id, either registered, or derived at the given depth, see derivedTypeID.
func lookupTypeByID(codecReg *_TypeRegistry, id TypeID, depth int) (reflect.Type, bool) {
	if rType, ok := lookupRegisteredTypeByID(codecReg, id); ok {
		return rType, true
	}
	return derivedType(codecReg, id, depth)
}

func lookupRegisteredTypeByID(codecReg *_TypeRegistry, id TypeID) (reflect.Type, bool) {
	if codecReg != nil {
		if rType, ok := codecReg.TypeByID(id); ok {
			return rType, true
		}
	}
	return typeIDRegistry.TypeByID(id)
}

// maxDerivedTypeIDDepth is how many slices and maps deep a type ID is derived for a type, see derivedTypeID.
// It bounds the types which unmarshal builds for the type IDs it reads,
// as each type built with reflect stays in the memory of the process.
const maxDerivedTypeIDDepth = 8

var stringType = reflect.TypeFor[string]()

// derivedTypeID derives the type ID of a slice or a map type which is not registered itself,
// from the type IDs of its key and element types, the way Go spells the type:
// the type ID of a []E is "[]" + the type ID of E,
// and the type ID of a map[string]E is "map[" + the type ID of string + "]" + the type ID of E.
//
// The type ID of E is the one it is registered with, or the one derived for it,
// up to maxDerivedTypeIDDepth slices and maps deep.
// An interface type can be registered too, which names the interface as the element type, like with a []Greeter.
// Only an unnamed slice or map type has a derived type ID, as a named type, like a `type Items []Item`, is a type of its own.
//
// A slice or a map of the empty interface has none, even if the empty interface is registered,
// so a []any or a map[string]any is written as plain JSON, which unmarshal reads back as the very same type.
// A slice or a map of pointers has none either, as the type ID of a pointer is the one of the type it points to.
// So a type ID is not used, when unmarshal would read it back as another type,
// like the type which is registered with it, or the type a pointer points to.
//
// An array type has none, as its type ID would hold its length, which is not bounded like the depth is,
// so any JSON could make unmarshal build new types, which stay in memory, and allocate an array of any size.
func derivedTypeID(codecReg *_TypeRegistry, typ reflect.Type) (TypeID, bool) {
	id, ok := deriveTypeID(codecReg, typ, 0)
	if !ok {
		return "", false
	}
	if rType, ok := lookupTypeByIDForCodec(codecReg, id); !ok || rType != typ {
		return "", false
	}
	return id, true
}

func deriveTypeID(codecReg *_TypeRegistry, typ reflect.Type, depth int) (TypeID, bool) {
	if typ.Name() != "" || maxDerivedTypeIDDepth <= depth {
		return "", false
	}
	switch typ.Kind() {
	case reflect.Slice:
		elemID, ok := elemTypeID(codecReg, typ.Elem(), depth+1)
		if !ok {
			return "", false
		}
		return "[]" + elemID, true

	case reflect.Map:
		if typ.Key() != stringType {
			return "", false
		}
		keyID, ok := lookupTypeIDForType(codecReg, stringType)
		if !ok {
			return "", false
		}
		elemID, ok := elemTypeID(codecReg, typ.Elem(), depth+1)
		if !ok {
			return "", false
		}
		return "map[" + keyID + "]" + elemID, true

	default:
		return "", false
	}
}

// elemTypeID returns the type ID of typ, the element type of a slice or a map, see derivedTypeID.
func elemTypeID(codecReg *_TypeRegistry, typ reflect.Type, depth int) (TypeID, bool) {
	if id, ok := lookupTypeIDForType(codecReg, typ); ok {
		return id, true
	}
	return deriveTypeID(codecReg, typ, depth)
}

// derivedType reads the type ID id, which derivedTypeID derived, back into its type, at the given depth.
func derivedType(codecReg *_TypeRegistry, id TypeID, depth int) (reflect.Type, bool) {
	if maxDerivedTypeIDDepth <= depth {
		return nil, false
	}
	if elemID, ok := strings.CutPrefix(string(id), "[]"); ok {
		elem, ok := elemTypeByID(codecReg, TypeID(elemID), depth+1)
		if !ok {
			return nil, false
		}
		return reflect.SliceOf(elem), true
	}
	if rest, ok := strings.CutPrefix(string(id), "map["); ok {
		keyID, elemID, ok := strings.Cut(rest, "]")
		if !ok {
			return nil, false
		}
		if key, ok := lookupRegisteredTypeByID(codecReg, TypeID(keyID)); !ok || key != stringType {
			return nil, false
		}
		elem, ok := elemTypeByID(codecReg, TypeID(elemID), depth+1)
		if !ok {
			return nil, false
		}
		return reflect.MapOf(stringType, elem), true
	}
	return nil, false
}

// elemTypeByID reads the type ID of the element type of a slice or a map back into its type, see elemTypeID.
func elemTypeByID(codecReg *_TypeRegistry, id TypeID, depth int) (reflect.Type, bool) {
	elem, ok := lookupTypeByID(codecReg, id, depth)
	if !ok || isEmptyInterface(elem) {
		return nil, false
	}
	return elem, true
}

func isEmptyInterface(typ reflect.Type) bool {
	return typ.Kind() == reflect.Interface && typ.NumMethod() == 0
}

func base(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}
