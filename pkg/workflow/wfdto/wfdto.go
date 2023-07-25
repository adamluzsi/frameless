package wfdto

import (
	"reflect"
	"sync"
)

type Mapping struct {
	init   sync.Once
	byType map[reflect.Type]func(v reflect.Value) (reflect.Value, error)
	byName map[string]reflect.Type
	names  []func(v any) (TypeName string, ok bool)
}

func (m *Mapping) Init() {
	m.init.Do(func() {
		m.byName = map[string]reflect.Type{}
		m.byType = map[reflect.Type]func(v reflect.Value) (reflect.Value, error){}
		m.names = []func(v any) (TypeName string, ok bool){}
	})
}

type ObjectDTO map[string]any

type RegistryRecord[Ent any] struct {
	TypeName    string
	GetTypeName func(dto ObjectDTO) (string, bool)
	ToDTO       func(Ent) (ObjectDTO, error)
	ToEnt       func(dto ObjectDTO) (Ent, error)
}

func Register[Ent any](m *Mapping, regrec RegistryRecord[Ent]) struct{} {
	var (
		dtoType = reflect.TypeOf((*ObjectDTO)(nil)).Elem()
		entType = reflect.TypeOf((*Ent)(nil)).Elem()
	)
	m.Init()
	m.names = append(m.names, func(v any) (string, bool) {
		dto, ok := v.(ObjectDTO)
		if !ok {
			return "", false
		}
		return regrec.GetTypeName(dto)
	})
	m.byType[dtoType] = func(v reflect.Value) (reflect.Value, error) {
		ent, err := regrec.ToEnt(v.Interface().(ObjectDTO))
		return reflect.ValueOf(ent), err
	}
	m.byType[entType] = func(v reflect.Value) (reflect.Value, error) {
		dto, err := regrec.ToDTO(v.Interface().(Ent))
		return reflect.ValueOf(dto), err
	}
	return struct{}{}
}

func ToEnt[Ent any](m *Mapping, dto any) Ent {
	return *new(Ent)
}
