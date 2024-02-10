package jsondto

import (
	"fmt"
	"go.llib.dev/frameless/pkg/reflectkit"
	"reflect"
	"sync"
)

func Register[T any](id TypeID) func() {
	rType := reflectkit.TypeOf[T]()
	return typeIDRegistry.Register(rType, id)
}

var typeIDRegistry _TypeIDRegistry

type _TypeIDRegistry struct {
	mutex         sync.RWMutex
	_TypeIDByType map[reflect.Type]TypeID
	_TypeByTypeID map[TypeID]reflect.Type
}

func (r *_TypeIDRegistry) Init() {
	r.mutex.RLock()
	ok := r._TypeIDByType != nil && r._TypeByTypeID != nil
	r.mutex.RUnlock()
	if ok {
		return
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	if r._TypeIDByType != nil {
		return
	}
	r._TypeIDByType = make(map[reflect.Type]TypeID)
	r._TypeByTypeID = make(map[TypeID]reflect.Type)
}

func (r *_TypeIDRegistry) Register(dtoType reflect.Type, id TypeID) func() {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	dtoType = base(dtoType)
	gotID, ok := r.typeIDByType(dtoType)
	if ok {
		const unableToRegisterFormat = "Unable to register %s __type id for %s, because it is already registered with %s"
		panic(fmt.Sprintf(unableToRegisterFormat, id, dtoType.String(), gotID))
	}
	r._TypeIDByType[dtoType] = id
	r._TypeByTypeID[id] = dtoType
	return func() { typeIDRegistry.UnregisterType(dtoType, id) }
}

func (r *_TypeIDRegistry) UnregisterType(rType reflect.Type, id TypeID) {
	r.Init()
	r.mutex.Lock()
	defer r.mutex.Unlock()
	rType = base(rType)
	delete(r._TypeIDByType, rType)
	delete(r._TypeByTypeID, id)
}

func (r *_TypeIDRegistry) TypeIDFor(v any) (TypeID, bool) {
	return r.TypeIDByType(reflect.TypeOf(v))
}

func (r *_TypeIDRegistry) TypeIDByType(typ reflect.Type) (TypeID, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return r.typeIDByType(typ)
}

func (r *_TypeIDRegistry) typeIDByType(typ reflect.Type) (TypeID, bool) {
	if typ == nil {
		return "", false
	}
	if r._TypeIDByType == nil {
		return *new(TypeID), false
	}
	id, ok := r._TypeIDByType[base(typ)]
	return id, ok
}

func (r *_TypeIDRegistry) TypeByID(id TypeID) (reflect.Type, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	if r._TypeByTypeID == nil {
		return nil, false
	}
	rType, ok := r._TypeByTypeID[id]
	return rType, ok
}

func base(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}
