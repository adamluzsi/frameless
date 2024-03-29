package dtos

import (
	"context"
	"errors"
	"fmt"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"reflect"
	"sync"
)

// Register function facilitates the registration of a mapping between two types.
// Optionally, if you don't intend to support bidirectional mapping, you can pass nil for the mapFrom argument.
// It's important to consider that supporting bidirectional mapping between an entity type and a DTO type
// often leads to the creation of non-partial DTO structures, enhancing their usability on the client side.
func Register[From, To any](mapTo mapFunc[From, To], mapFrom mapFunc[To, From]) func() {
	mr := mRec{
		TypeA: reflectkit.TypeOf[From](),
		TypeB: reflectkit.TypeOf[To](),
	}
	if mapTo == nil {
		panic(fmt.Errorf("dtos.Register: %s to %s mapping is required",
			reflectkit.TypeOf[From]().String(), reflectkit.TypeOf[To]().String()))
	}
	mr.MapAToB = func(ctx context.Context, from any) (to any, err error) {
		return mapTo(ctx, from.(From))
	}
	if mapFrom != nil { // non partial DTO mapping
		mr.MapBToA = func(ctx context.Context, to any) (from any, err error) {
			return mapFrom(ctx, to.(To))
		}
	}
	m.register(mr)
	return func() { m.unregister(mr) }
}

func Map[To, From any](ctx context.Context, from From) (_ To, returnErr error) {
	torv, err := m.Map(ctx, P[From, To]{}, from)
	if err != nil {
		return *new(To), err
	}
	return torv.(To), nil
}

func MustMap[To, From any](ctx context.Context, from From) To {
	to, err := Map[To, From](ctx, from)
	if err != nil {
		panic(ErrMustMap{Err: err})
	}
	return to
}

func (m *_M) Map(ctx context.Context, mp MP, from any) (_ any, returnErr error) {
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
	to, err := r.Map(ctx, reflectkit.BaseValue(reflect.ValueOf(from)).Interface())
	if err != nil {
		return nil, err
	}
	v := reflect.ValueOf(to)
	for i := 0; i < depth; i++ {
		v = reflectkit.PointerOf(v)
	}
	return v.Interface(), nil
}

const ErrNoMapping errorkit.Error = "[dtos] missing mapping"

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

var m _M

type _M struct {
	mutex  sync.RWMutex
	_init  sync.Once
	byType map[MK]*mRec
}

type MK struct {
	From reflect.Type
	To   reflect.Type
}

type mRec struct {
	TypeA   reflect.Type
	TypeB   reflect.Type
	MapAToB func(ctx context.Context, a any) (b any, err error)
	MapBToA func(ctx context.Context, b any) (a any, err error)
}

func (rec mRec) Map(ctx context.Context, v any) (any, error) {
	switch reflect.TypeOf(v) {
	case rec.TypeA:
		return rec.useMapping(rec.MapAToB, ctx, v)
	case rec.TypeB:
		return rec.useMapping(rec.MapBToA, ctx, v)
	default:
		return nil, fmt.Errorf("type not recognised: %T", v)
	}
}

func (rec mRec) useMapping(fn func(context.Context, any) (any, error), ctx context.Context, v any) (any, error) {
	if fn == nil {
		return nil, ErrNoMapping
	}
	return fn(ctx, v)
}

type mapFunc[From, To any] func(context.Context, From) (To, error)

func (m *_M) init() {
	m._init.Do(func() { m.byType = make(map[MK]*mRec) })
}

func (m *_M) register(r mRec) {
	m.init()
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if r.MapAToB != nil {
		m.byType[MK{From: r.TypeA, To: r.TypeB}] = &r
	}
	if r.MapBToA != nil {
		m.byType[MK{From: r.TypeB, To: r.TypeA}] = &r
	}
}

func (m *_M) unregister(r mRec) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.byType, MK{From: r.TypeA, To: r.TypeB})
	delete(m.byType, MK{From: r.TypeB, To: r.TypeA})
}

func (m *_M) lookupByType(from, to reflect.Type) (*mRec, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if m.byType == nil {
		return nil, false
	}
	rec, ok := m.byType[MK{From: from, To: to}]
	return rec, ok
}

// P is a Mapping pair
type P[A, B any] struct{}

func (p P[A, B]) MapA(ctx context.Context, v B) (A, error) { return Map[A, B](ctx, v) }
func (p P[A, B]) MapB(ctx context.Context, v A) (B, error) { return Map[B, A](ctx, v) }

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

type ErrMustMap struct{ Err error }

func (err ErrMustMap) Error() string {
	if err.Err != nil {
		return err.Err.Error()
	}
	return "ErrMustMap"
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
