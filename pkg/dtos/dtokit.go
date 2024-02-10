package dtos

import (
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"reflect"
	"sync"
)

type M struct {
	mutex  sync.RWMutex
	_init  sync.Once
	byType map[MK]*mRec
}

func (m *M) FromMappings(v any) []MK {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	var (
		fromType = reflectkit.BaseTypeOf(v)
		keys     []MK
	)
	for mk, _ := range m.byType {
		if mk.From == fromType {
			keys = append(keys, mk)
		}
	}
	return keys
}

type MK struct {
	From reflect.Type
	To   reflect.Type
}

type mRec struct {
	EntType reflect.Type
	DTOType reflect.Type
	ToDTO   func(ent any) (dto any, err error)
	ToEnt   func(dto any) (ent any, err error)
}

func (rec mRec) Map(v any) (any, error) {
	switch reflect.TypeOf(v) {
	case rec.EntType:
		return rec.ToDTO(v)
	case rec.DTOType:
		return rec.ToEnt(v)
	default:
		return nil, fmt.Errorf("type not recognised: %T", v)
	}
}

type Mapping[Ent, DTO any] interface {
	ToEnt(*M, DTO) (Ent, error)
	ToDTO(*M, Ent) (DTO, error)
}

func Register[Ent, DTO any](m *M, mapping Mapping[Ent, DTO]) func() {
	mr := mRec{
		EntType: reflectkit.TypeOf[Ent](),
		DTOType: reflectkit.TypeOf[DTO](),
		ToDTO: func(ent any) (dto any, err error) {
			return mapping.ToDTO(m, ent.(Ent))
		},
		ToEnt: func(dto any) (ent any, err error) {
			return mapping.ToEnt(m, dto.(DTO))
		},
	}
	m.register(mr)
	return func() { m.unregister(mr) }
}

func (m *M) init() {
	m._init.Do(func() { m.byType = make(map[MK]*mRec) })
}

func (m *M) register(r mRec) {
	m.init()
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.byType[MK{From: r.EntType, To: r.DTOType}] = &r
	m.byType[MK{From: r.DTOType, To: r.EntType}] = &r
}

func (m *M) unregister(r mRec) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.byType, MK{From: r.EntType, To: r.DTOType})
	delete(m.byType, MK{From: r.DTOType, To: r.EntType})
}

func (m *M) lookupByType(from, to reflect.Type) (*mRec, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.byType == nil {
		return nil, false
	}
	rec, ok := m.byType[MK{From: from, To: to}]
	return rec, ok
}

const ErrNoMapping errorkit.Error = "[dtos] missing mapping"

func Map[To, From any](m *M, from From) (_ To, returnErr error) {
	var (
		fromType = reflectkit.TypeOf[From]()
		toType   = reflectkit.TypeOf[To]()
	)
	torv, err := m.Map(fromType, toType, from)
	if err != nil {
		return *new(To), err
	}
	return torv.Interface().(To), nil
}

func (m *M) Map(fromType, toType reflect.Type, from any) (_ reflect.Value, returnErr error) {
	if m == nil {
		return reflect.Value{}, fmt.Errorf("[dtos] M is not supplied")
	}
	defer recoverMustMap(&returnErr)
	var toBaseType, depth = reflectkit.BaseType(toType)
	r, ok := m.lookupByType(fromType, toBaseType)
	if !ok {
		return reflect.Value{}, fmt.Errorf("%w from %s to %s", ErrNoMapping,
			fromType.String(), toType.String())
	}
	to, err := r.Map(reflectkit.BaseValue(reflect.ValueOf(from)).Interface())
	if err != nil {
		return reflect.Value{}, err
	}
	v := reflect.ValueOf(to)
	for i := 0; i < depth; i++ {
		v = reflectkit.PointerOf(v)
	}
	return v, nil
}

type ErrMustMap struct{ Err error }

func (err ErrMustMap) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return "ErrMustMap"
}

func MustMap[To, From any](m *M, from From) To {
	to, err := Map[To, From](m, from)
	if err != nil {
		panic(ErrMustMap{Err: err})
	}
	return to
}

func recoverMustMap(returnErr *error) {
	r := recover()
	if r == nil {
		return
	}
	err, ok := r.(error)
	if !ok {
		panic(r)
	}
	var emm ErrMustMap
	if errors.As(err, &emm) {
		*returnErr = emm.Err
		return
	}
	panic(err)
}
