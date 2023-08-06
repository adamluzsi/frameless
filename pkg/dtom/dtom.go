package dtom

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"reflect"
)

/* MAPPING */

func MapList[X, Y any](vs []X, mapFn func(v X) Y) []Y {
	var out []Y
	for _, v := range vs {
		out = append(out, mapFn(v))
	}
	return out
}

func MapVal[T any](r *Registry, dto any) T {
	switch dto := dto.(type) {
	case Struct:
		v, err := r.ToEntity(dto)
		if err != nil {
			panic(fmt.Errorf("%w: %s", Err, err.Error()))
		}
		ent, ok := v.(T)
		if !ok {
			panic(fmt.Errorf("%w: %s, %T", Err, "invalid type received", ent))
		}
		return ent
	default:
		panic(fmt.Errorf("%w: %s, %T", Err, "unrecognised DTO value", dto))
	}
}

func MapDTO[T any](r *Registry, ent T) any {
	dto, ok, err := r.ToDataTransferObject(ent)
	if err != nil {
		panic(fmt.Errorf("%w: %s", Err, err.Error()))
	}
	if ok {
		return dto
	}
	refVal := reflect.ValueOf(ent)
	switch refVal.Kind() {
	case reflect.Slice:
		var out []any
		for i, l := 0, refVal.Len(); i < l; i++ {
			out = append(out, MapDTO[any](r, refVal.Index(i)))
		}
		return out
	default:
		panic(fmt.Errorf("%w: DTO mapping not found", Err))
	}
}

type Registry struct{ structs []mapper }

type mapper interface {
	ToEntity(dto any) (ent any, err error)
	ToDataTransferObject(ent any) (dto any, err error)

	CheckEntity(ent any) bool
	CheckDataTransferObject(dto any) bool
}

/* STRUCT */

func RegisterStruct[Entity any](r *Registry, m StructMapping[Entity]) struct{} {
	r.structs = append(r.structs, m)
	return struct{}{}
}

type Struct map[string]any

func (dto Struct) List(key string) []Struct {
	v, ok := dto.Lookup(key)
	if !ok {
		panic(ErrKeyNotFound(key))
	}
	vs, ok := v.([]Struct)
	if !ok {
		panic(ErrKeyNotFound(key))
	}
	return vs
}

func (dto Struct) Object(key string) Struct {
	v, ok := dto.LookupObject(key)
	if !ok {
		panic(ErrKeyNotFound(key))
	}
	return v
}

func (dto Struct) LookupObject(key string) (Struct, bool) {
	v, ok := dto.Lookup(key)
	if !ok {
		return nil, false
	}
	if v, ok := v.(Struct); ok {
		return v, true
	}
	if v, ok := v.(map[string]any); ok {
		return v, true
	}
	return nil, false
}

func (dto Struct) Lookup(key string) (any, bool) {
	if dto == nil {
		return nil, false
	}
	v, ok := dto[key]
	return v, ok
}

func (r *Registry) ToEntity(dto Struct) (any, error) {
	for _, m := range r.structs {
		if m.CheckDataTransferObject(dto) {
			return m.ToEntity(dto)
		}
	}
	return nil, fmt.Errorf("unexpected data transfer object")
}

func (r *Registry) ToDataTransferObject(ent any) (Struct, bool, error) {
	for _, m := range r.structs {
		if m.CheckEntity(ent) {
			dto, err := m.ToDataTransferObject(ent)
			return dto, true, err
		}
	}
	return nil, false, nil
}

type StructMapping[Entity any] struct {
	Check func(str Struct) bool
	ToEnt func(str Struct) (Entity, error)
	ToDTO func(ent Entity) (Struct, error)
}

func (m StructMapping[Entity]) ToEntity(dto Struct) (_ any, rErr error) {
	defer mappingRecover(&rErr)
	ent, err := m.ToEnt(dto)
	return ent, err
}

func (m StructMapping[Entity]) ToDataTransferObject(iEnt any) (_ Struct, rErr error) {
	defer mappingRecover(&rErr)
	ent, ok := iEnt.(Entity)
	if !ok {
		return nil, fmt.Errorf("expected %T but got %T", *new(Entity), iEnt)
	}
	return m.ToDTO(ent)
}

func (m StructMapping[Entity]) CheckEntity(ent any) bool {
	_, ok := ent.(Entity)
	return ok
}

func (m StructMapping[Entity]) CheckDataTransferObject(dto Struct) bool {
	return m.Check(dto)
}

const Err errorkit.Error = "DATA_TRANSFER_OBJECT_MAPPING_ERROR"

func ErrKeyNotFound(key string) error { return fmt.Errorf("%w: %s key not found", Err, key) }

func mappingRecover(returnErr *error) {
	r := recover()
	if r == nil {
		return
	}
	if err, ok := r.(error); ok {
		*returnErr = err
	} else {
		*returnErr = fmt.Errorf("%v", r)
	}
}
