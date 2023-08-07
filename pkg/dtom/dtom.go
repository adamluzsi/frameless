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
			return val, errf("unexpected data transfer object")
		}
		v, err := mapper.ToValueType(dto)
		if err != nil {
			return val, wrapErr(err)
		}
		val, ok := v.(T)
		if !ok {
			return val, errf("%s, %T", "invalid type received", val)
		}
		return val, nil

	default:
		var typ = reflect.TypeOf((*T)(nil)).Elem()

		if !isPrimitiveKind(typ) {
			return *new(T), errf("%s, expected %T but got %T", "unrecognised DTO value", *new(T), dto)
		}

		val, ok := reflectkit.Cast[T](dto)
		if !ok {
			return *new(T), errf("failed to cast value from %T to %s", dto, typ.String())
		}

		return val, nil
	}
}

func MapDTO[DTO, V any](r *Registry, val V) (DTO, error) {
	dto, ok, err := r.toDataTransferObject(val)
	if err != nil {
		return *new(DTO), wrapErr(err)
	}
	if ok {
		return dto, nil
	}
	refVal := reflect.ValueOf(val)
	switch refVal.Kind() {
	case reflect.Slice:
		var out DTO
		for i, l := 0, refVal.Len(); i < l; i++ {
			elem, err := MapDTO(r, refVal.Index(i).Interface())
			if err != nil {
				return *new(DTO), err
			}
			out = append(out, elem)
		}
		return out, nil

		Array, Chan, Func, Interface, Map,
		Pointer, Slice,
		, Struct, UnsafePointer,


	default:
		if isPrimitiveKind(refVal.Type()) {
			dto, ok := reflectkit.Cast[DTO](val)
			if !ok {
				return *new(DTO), errf("unable to cast %T to %T", val, dto)
			}
		}
		return *new(DTO), errf("DTO mapping not found for %T", val)
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
	CheckEntity(ent any) bool
	ToValueType(dto any) (ent any, err error)
	CheckDataTransferObject(dto any) bool
	ToDataTransferObject(ent any) (dto any, err error)
}

func (r *Registry) ToValue(dto Struct) (any, error) {
	for _, m := range r.structs {
		if m.CheckDataTransferObject(dto) {
			return m.ToValueType(dto)
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

func RegisterStruct[Value, DTO any](r *Registry, dtoTypeID string,
	ToVal func(DTO) (Value, error),
	ToDTO func(Value) (DTO, error),
) struct{} {
	r.structs = append(r.structs, StructMapping[Value, DTO]{
		TypeID: dtoTypeID,
		ToEnt:  ToVal,
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

type StructMapping[Value, DTO any] struct {
	TypeID string
	ToEnt  func(str DTO) (Value, error)
	ToDTO  func(ent Value) (DTO, error)
}

func (m StructMapping[Value, DTO]) ToValueType(idto any) (_ any, rErr error) {
	defer mappingRecover(&rErr)
	dto, ok := idto.(DTO)
	if !ok {
		return nil, fmt.Errorf("invalid type, expected %T but got %T", *new(DTO), idto)
	}
	return m.ToEnt(dto)
}

func (m StructMapping[Value, DTO]) ToDataTransferObject(ival any) (_ any, rErr error) {
	defer mappingRecover(&rErr)
	val, ok := ival.(Value)
	if !ok {
		return nil, errf("expected %T but got %T", *new(Value), ival)
	}
	return m.ToDTO(val)
}

func (m StructMapping[Value, DTO]) CheckEntity(ent any) bool {
	_, ok := ent.(Value)
	return ok
}

const structTypeFieldKey = "__type"

func (m StructMapping[Value, DTO]) CheckDataTransferObject(str Struct) bool {
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

func errf(format string, args ...any) error {
	return wrapErr(fmt.Errorf(format, args...))
}

func wrapErr(err error) error {
	if !errors.Is(err, Err) {
		err = errorkit.Merge(Err, err)
	}
	return err
}

func ErrKeyNotFound(key string) error { return errf("%s key not found", key)) }

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
