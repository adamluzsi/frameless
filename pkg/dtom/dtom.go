package dtom

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/zerokit"
	"reflect"
)

type DataTransferObject map[string]any

type Registry struct{ _mapping map[reflect.Type]mapper }

func (r *Registry) mapping() map[reflect.Type]mapper {
	return zerokit.Init(&r._mapping, func() map[reflect.Type]mapper {
		return map[reflect.Type]mapper{}
	})
}

func (r Registry) ToEntity(dto DataTransferObject) (any, error) {
	return nil, nil
}

func (r Registry) ToDataTransferObject(ent any) (DataTransferObject, error) {
	return nil, nil
}

func Register[Entity any](r *Registry, m Mapping[Entity]) struct{} {
	r.mapping()
	
	return struct{}{}
}

type mapper interface {
	ToEntity(DataTransferObject) (any, error)
	ToDataTransferObject(ent any) (DataTransferObject, error)
}

type Mapping[Entity any] struct {
	ToEnt func(DataTransferObject) (Entity, error)
	ToDTO func(Entity) (DataTransferObject, error)
}

func (m Mapping[Entity]) ToEntity(dto DataTransferObject) (any, error) {
	ent, err := m.ToEnt(dto)
	return ent, err
}

func (m Mapping[Entity]) ToDataTransferObject(iEnt any) (DataTransferObject, error) {
	ent, ok := iEnt.(Entity)
	if !ok {
		return nil, fmt.Errorf("expected %T but got %T", *new(Entity), iEnt)
	}

	dto, err := m.ToDTO()
}
