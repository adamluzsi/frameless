package dtom

import (
	"fmt"
	"reflect"
)

type DataTransferObject map[string]any

type Registry struct {
	mapping map[reflect.Type]mapper
}

func Register[Entity any](r *Registry, m Mapping[Entity]) struct{} {

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
