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
	torv, err := m.Map(P[From, To]{}, from)
	if err != nil {
		return *new(To), err
	}
	return torv.(To), nil
}

// P is a Mapping pair
type P[A, B any] struct{}

func (p P[A, B]) MapA(m *M, v B) (A, error) { return Map[A, B](m, v) }
func (p P[A, B]) MapB(m *M, v A) (B, error) { return Map[B, A](m, v) }

func (p P[A, B]) NewA() *A { return new(A) }
func (p P[A, B]) NewB() *B { return new(B) }

// FromType specify that P[A] is the from type.
func (p P[A, B]) FromType() reflect.Type { return reflectkit.TypeOf[A]() }

// ToType specify that P[B] is the from type.
func (p P[A, B]) ToType() reflect.Type { return reflectkit.TypeOf[B]() }

type MP interface {
	FromType() reflect.Type
	ToType() reflect.Type
}

func (m *M) Map(mp MP, from any) (_ any, returnErr error) {
	if mp.FromType() == mp.ToType() { // passthrough mode
		return from, nil
	}
	if m == nil {
		return nil, fmt.Errorf("[dtos] M is not supplied")
	}
	defer recoverMustMap(&returnErr)
	var toBaseType, depth = reflectkit.BaseType(mp.ToType())
	r, ok := m.lookupByType(mp.FromType(), toBaseType)
	if !ok {
		return nil, fmt.Errorf("%w from %s to %s", ErrNoMapping,
			mp.FromType().String(), mp.ToType().String())
	}
	to, err := r.Map(reflectkit.BaseValue(reflect.ValueOf(from)).Interface())
	if err != nil {
		return nil, err
	}
	v := reflect.ValueOf(to)
	for i := 0; i < depth; i++ {
		v = reflectkit.PointerOf(v)
	}
	return v.Interface(), nil
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
