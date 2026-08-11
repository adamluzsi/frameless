package jsonkit

import (
	"fmt"
	"reflect"
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
	mutex       sync.RWMutex
	init        sync.Once
	byType      map[reflect.Type]TypeID
	byTypeID    map[TypeID]reflect.Type
	byAlias     map[TypeID]TypeID
	customCodec map[reflect.Type]*customCodecEntry
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
		r.byAlias = make(map[TypeID]TypeID)
		r.customCodec = make(map[reflect.Type]*customCodecEntry)
	})
}

// SetCustomCodec stores a per-registry custom Marshal/Unmarshal pair for the
// given concrete type and returns a function that restores the previous entry.
// Panics if the type was already registered with a different ID, mirroring the
// behaviour of Register so a misuse is loud rather than silent.
func (r *_TypeRegistry) SetCustomCodec(dtoType reflect.Type, entry *customCodecEntry) func() {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	dtoType = base(dtoType)
	if existingID, ok := r.byType[dtoType]; ok && existingID != entry.TypeID {
		panic(fmt.Sprintf("Unable to register %q @type id for %s, because it is already registered with %s",
			entry.TypeID, dtoType.String(), existingID))
	}
	prev := r.customCodec[dtoType]
	r.customCodec[dtoType] = entry
	r.byType[dtoType] = entry.TypeID
	r.byTypeID[entry.TypeID] = dtoType
	// Mark this type as having a custom codec on at least one codec in the
	// process. unmarshalPlan uses this marker to decide whether to render
	// the field as json.RawMessage so the runtime copyFn can choose between
	// invoking the custom closure and reporting a clear error when the
	// current codec does not have a registration for this type.
	addCustomCodecMarker(dtoType)
	return func() {
		r.mutex.Lock()
		defer r.mutex.Unlock()
		if prev == nil {
			delete(r.customCodec, dtoType)
			delete(r.byType, dtoType)
			delete(r.byTypeID, entry.TypeID)
			removeCustomCodecMarker(dtoType)
			return
		}
		r.customCodec[dtoType] = prev
		r.byType[dtoType] = prev.TypeID
		r.byTypeID[prev.TypeID] = dtoType
		addCustomCodecMarker(dtoType)
	}
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

func (r *_TypeRegistry) Register(dtoType reflect.Type, id TypeID, aliases ...TypeID) func() {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	dtoType = base(dtoType)
	gotID, ok := r.typeIDByType(dtoType)
	if ok {
		const format = "Unable to register %q @type id for %s, because it is already registered with %s"
		panic(fmt.Sprintf(format, id, dtoType.String(), gotID))
	}
	r.byType[dtoType] = id
	r.byTypeID[id] = dtoType
	for _, alias := range aliases {
		if _, isRegistered := r.byAlias[alias]; isRegistered {
			const format = "Unable to register %q @type alias for %s"
			panic(fmt.Sprintf(format, alias, dtoType.String()))
		}
		if _, isRegistered := r.byTypeID[alias]; isRegistered {
			const format = "Unable to register %q @type alias because it is an already registered @type id"
			panic(fmt.Sprintf(format, alias))
		}
		r.byAlias[alias] = id
	}
	return func() { r.UnregisterType(dtoType, id, aliases...) }
}

func (r *_TypeRegistry) UnregisterType(rType reflect.Type, id TypeID, aliases ...TypeID) {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	rType = base(rType)
	delete(r.byType, rType)
	delete(r.byTypeID, id)
	for _, alias := range aliases {
		delete(r.byAlias, alias)
	}
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

// lookupTypeIDForType is the type-only counterpart of lookupTypeIDForValue.
// Like lookupTypeIDForValue, it is used during MARSHAL and therefore does
// not consult the shared cross-codec registry.
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
func lookupTypeByIDForCodec(codecReg *_TypeRegistry, id TypeID) (reflect.Type, bool) {
	if codecReg != nil {
		if rType, ok := codecReg.TypeByID(id); ok {
			return rType, true
		}
	}
	return typeIDRegistry.TypeByID(id)
}

func base(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

// customCodecTypes is a process-wide set of reflect.Types that have been
// registered through CodecRegister on at least one codec in the process.
// unmarshalPlan consults this set to decide whether a struct field whose
// concrete type might be codec-registered should be rendered as
// json.RawMessage in the DTO, so the runtime copyFn can invoke the custom
// Unmarshal closure when the current codec has the entry.
//
// The set only needs to track "could this type have a custom codec?"; the
// runtime copyFn still consults the codec's own registry to decide whether
// to invoke the closure. The set's sole purpose is to make the unmarshal
// plan structure accommodate both shapes.
var (
	customCodecMarkerMu sync.RWMutex
	customCodecMarker   = map[reflect.Type]struct{}{}
)

func addCustomCodecMarker(t reflect.Type) {
	customCodecMarkerMu.Lock()
	defer customCodecMarkerMu.Unlock()
	customCodecMarker[base(t)] = struct{}{}
	// Invalidate any cached unmarshal plan that might embed the now
	// custom-codec-aware type as a concrete struct field. The plan
	// embeds the DTO shape, which depends on whether the type is
	// custom-codec-aware (RawMessage field vs the concrete type). Plans
	// built before CodecRegister was called would otherwise route the
	// field through the default decode path and silently produce zero
	// values for any concrete-type field whose wire was encoded with
	// a custom Marshal closure.
	invalidateUnmarshalPlanCache()
}

func removeCustomCodecMarker(t reflect.Type) {
	customCodecMarkerMu.Lock()
	defer customCodecMarkerMu.Unlock()
	delete(customCodecMarker, base(t))
}

func hasCustomCodecMarker(t reflect.Type) bool {
	customCodecMarkerMu.RLock()
	defer customCodecMarkerMu.RUnlock()
	_, ok := customCodecMarker[base(t)]
	return ok
}
