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

var typeIDRegistry _TypeIDRegistry

type _TypeIDRegistry struct {
	mutex    sync.RWMutex
	init     sync.Once
	byType   map[reflect.Type]TypeID
	byTypeID map[TypeID]reflect.Type
	byAlias  map[TypeID]TypeID
}

func (r *_TypeIDRegistry) Init() {
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
	})
}

func (r *_TypeIDRegistry) Register(dtoType reflect.Type, id TypeID, aliases ...TypeID) func() {
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
	return func() { typeIDRegistry.UnregisterType(dtoType, id, aliases...) }
}

func (r *_TypeIDRegistry) UnregisterType(rType reflect.Type, id TypeID, aliases ...TypeID) {
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

func (r *_TypeIDRegistry) TypeIDFor(v any) (TypeID, bool) {
	return r.TypeIDByType(reflect.TypeOf(v))
}

func (r *_TypeIDRegistry) TypeIDByType(typ reflect.Type) (TypeID, bool) {
	r.Init()
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.typeIDByType(typ)
}

func (r *_TypeIDRegistry) typeIDByType(typ reflect.Type) (TypeID, bool) {
	if typ == nil {
		return "", false
	}
	if r.byType == nil {
		return *new(TypeID), false
	}
	id, ok := r.byType[base(typ)]
	return id, ok
}

func (r *_TypeIDRegistry) TypeByID(id TypeID) (reflect.Type, bool) {
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

func base(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}
