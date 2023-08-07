package dtom

import (
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
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

func MapValue[T any](r *Registry, dto any) (T, error) {
	switch dto := dto.(type) {
	case Struct:
		var val T
		var mapper structMapper
		for _, m := range r.structs {
			if m.CheckDataTransferObject(dto) {
				mapper = m
				break
			}
		}
		if mapper == nil {
			return val, wrapErr(fmt.Errorf("unexpected data transfer object"))
		}
		v, err := mapper.ToEntity(dto)
		if err != nil {
			return val, wrapErr(err)
		}
		val, ok := v.(T)
		if !ok {
			return val, wrapErr(fmt.Errorf("%s, %T", "invalid type received", val))
		}
		return val, nil

	default:
		var typ = reflect.TypeOf((*T)(nil)).Elem()

		if !isPrimitiveKind(typ) {
			return *new(T), wrapErr(fmt.Errorf("%s, expected %T but got %T",
				"unrecognised DTO value", *new(T), dto))
		}

		val, ok := reflectkit.Cast[T](dto)
		if !ok {
			return *new(T), wrapErr(fmt.Errorf(
				"failed to cast value from %T to %s", dto, typ.String()))
		}

		return val, nil
	}
}

func MapDTO(r *Registry, ent any) (any, error) {
	dto, ok, err := r.toDataTransferObject(ent)
	if err != nil {
		return nil, wrapErr(err)
	}
	if ok {
		return dto, nil
	}
	refVal := reflect.ValueOf(ent)
	switch refVal.Kind() {
	case reflect.Slice:
		var out []any
		for i, l := 0, refVal.Len(); i < l; i++ {
			elem, err := MapDTO(r, refVal.Index(i).Interface())
			if err != nil {
				return nil, err
			}
			out = append(out, elem)
		}
		return out, nil

	//Array, Chan, Func, Interface, Map,
	//Pointer, Slice,
	//, Struct, UnsafePointer,

	default:
		if isPrimitiveKind(refVal.Type()) {
			return ent, nil
		}
		return nil, wrapErr(fmt.Errorf("DTO mapping not found for %T", ent))
	}
}

func isPrimitiveKind(typ reflect.Type) bool {
	switch typ.Kind() {
	case reflect.String, reflect.Bool, reflect.Uintptr,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128:
		return true
	default:
		return false
	}
}

type Registry struct {
	structs    []structMapper
	interfaces []interfaceMapping
}

type structMapper interface {
	ToEntity(dto Struct) (ent any, err error)
	ToDataTransferObject(ent any) (dto Struct, err error)

	CheckEntity(ent any) bool
	CheckDataTransferObject(dto Struct) bool
}

func (r *Registry) ToValue(dto Struct) (any, error) {
	for _, m := range r.structs {
		if m.CheckDataTransferObject(dto) {
			return m.ToEntity(dto)
		}
	}
	return nil, fmt.Errorf("unexpected data transfer object")
}

func (r *Registry) toDataTransferObject(ent any) (any, bool, error) {
	for _, m := range r.structs {
		if m.CheckEntity(ent) {
			dto, err := m.ToDataTransferObject(ent)
			return dto, true, err
		}
	}
	return nil, false, nil
}

/* INTERFACE */

func RegisterInterface[INTERFACE any](r *Registry, implementations ...INTERFACE) struct{} {
	iface := reflect.TypeOf((*INTERFACE)(nil)).Elem()
	if iface.Kind() != reflect.Interface {
		panic(fmt.Sprintf("INTERFACE type argument must be an interface type (%s)", iface.String()))
	}
	var impls []reflect.Type
	for _, implv := range implementations {
		implr := reflect.TypeOf(implv)
		if !implr.Implements(iface) {
			panic(fmt.Sprintf("%s should implement %s", implr.String(), iface.String()))
		}
		impls = append(impls, implr)
	}
	r.interfaces = append(r.interfaces, interfaceMapping{
		Interface:       iface,
		Implementations: impls,
	})
	return struct{}{}
}

type interfaceMapping struct {
	Interface       reflect.Type
	Implementations []reflect.Type
}

/* STRUCT */

func RegisterStruct[Entity any](r *Registry, dtoTypeID string,
	ToEnt func(str Struct) (Entity, error),
	ToDTO func(ent Entity) (Struct, error),
) struct{} {
	r.structs = append(r.structs, StructMapping[Entity]{
		TypeID: dtoTypeID,
		ToEnt:  ToEnt,
		ToDTO:  ToDTO,
	})
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

type StructMapping[Entity any] struct {
	TypeID string
	ToEnt  func(str Struct) (Entity, error)
	ToDTO  func(ent Entity) (Struct, error)
}

func (m StructMapping[Entity]) ToEntity(dto Struct) (_ any, rErr error) {
	defer mappingRecover(&rErr)
	return m.ToEnt(dto)
}

func (m StructMapping[Entity]) ToDataTransferObject(iEnt any) (_ Struct, rErr error) {
	defer mappingRecover(&rErr)
	ent, ok := iEnt.(Entity)
	if !ok {
		return nil, fmt.Errorf("expected %T but got %T", *new(Entity), iEnt)
	}
	dto, err := m.ToDTO(ent)
	if err != nil {
		return nil, err
	}
	if dto == nil {
		return nil, nil
	}
	dto[structTypeFieldKey] = m.TypeID
	return dto, nil
}

func (m StructMapping[Entity]) CheckEntity(ent any) bool {
	_, ok := ent.(Entity)
	return ok
}

const structTypeFieldKey = "__type"

func (m StructMapping[Entity]) CheckDataTransferObject(str Struct) bool {
	return str[structTypeFieldKey] == m.TypeID
}

/* DSL-ERROR */

const Err errorkit.Error = "DATA_TRANSFER_OBJECT_MAPPING_ERROR"

func Must[T any](v T, err error) T {
	if err != nil {
		panic(wrapErr(err))
	}
	return v
}

func wrapErr(err error) error {
	if !errors.Is(err, Err) {
		err = errorkit.Merge(Err, err)
	}
	return err
}

func ErrKeyNotFound(key string) error { return wrapErr(fmt.Errorf("%s key not found", key)) }

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
