package dtokit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
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
	var hasMapping bool
	if mapTo != nil { // support partial mapping
		hasMapping = true
		mr.MapAToB = func(ctx context.Context, from any) (to any, err error) {
			return mapTo(ctx, from.(From))
		}
	}
	if mapFrom != nil { // support partial mapping
		hasMapping = true
		mr.MapBToA = func(ctx context.Context, to any) (from any, err error) {
			return mapFrom(ctx, to.(To))
		}
	}
	if !hasMapping {
		var (
			fromTypeName = reflectkit.TypeOf[From]().String()
			toTypeName   = reflectkit.TypeOf[To]().String()
		)
		panic(fmt.Errorf("dtokit.Register: at partial mapping between %s and %s is required", fromTypeName, toTypeName))
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
		return nil, fmt.Errorf("[dtokit] M is not supplied")
	}
	defer recoverMustMap(&returnErr)
	var toBaseType, depth = reflectkit.BaseType(mp.ToType())
	var fromValue = reflect.ValueOf(from)
	r, ok := m.lookupByType(mp.FromType(), toBaseType)
	if !ok {
		fromType, _ := reflectkit.BaseType(mp.FromType())
		r, ok = m.lookupByType(fromType, toBaseType)
		if ok { // if base FromType is recognised, then BaseValue of fromValue is needed for correct mapping
			fromValue = reflectkit.BaseValue(fromValue)
		}
	}
	if v, err, isSliceCase := m.checkForSliceSyntaxSugarMapping(ctx, mp, from); !ok && isSliceCase {
		return v, err
	}
	if !ok {
		return nil, fmt.Errorf("%w from %s to %s", ErrNoMapping,
			mp.FromType().String(), mp.ToType().String())
	}
	to, err := r.Map(ctx, fromValue.Interface())
	if err != nil {
		return nil, err
	}
	v := reflect.ValueOf(to)
	for i := 0; i < depth; i++ {
		v = reflectkit.PointerOf(v)
	}
	return v.Interface(), nil
}

func (m *_M) checkForSliceSyntaxSugarMapping(ctx context.Context, mp MP, from any) (_ any, _ error, isSliceCase bool) {
	if !(mp.FromType().Kind() == reflect.Slice && mp.ToType().Kind() == reflect.Slice) {
		return nil, nil, false
	}
	isSliceCase = true
	input := reflectkit.BaseValueOf(from)
	if !input.CanConvert(mp.FromType()) {
		return nil, fmt.Errorf(`type mismatch, expected %s, but got %s for "From" mapping input`,
			mp.FromType().String(), input.Type().String()), isSliceCase
	}
	input = input.Convert(mp.FromType())
	pair := _MP{
		From: mp.FromType().Elem(),
		To:   mp.ToType().Elem(),
	}
	list := reflect.MakeSlice(mp.ToType(), 0, 0)
	for i, l := 0, input.Len(); i < l; i++ {
		elem, err := m.Map(ctx, pair, input.Index(i).Interface())
		if err != nil {
			return nil, err, isSliceCase
		}
		list = reflect.Append(list, reflect.ValueOf(elem))
	}
	return list.Interface(), nil, isSliceCase
}

const ErrNoMapping errorkit.Error = "[dtokit] missing mapping"

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

type MP interface {
	FromType() reflect.Type
	ToType() reflect.Type
}

// P is a Mapping pair
type P[A, B any] struct{}

func (p P[A, B]) MapA(ctx context.Context, v B) (A, error) { return Map[A, B](ctx, v) }
func (p P[A, B]) MapB(ctx context.Context, v A) (B, error) { return Map[B, A](ctx, v) }

func (p P[A, B]) NewA() *A { return new(A) }
func (p P[A, B]) NewB() *B { return new(B) }

// FromType specify that P[A] is the form type.
func (p P[A, B]) FromType() reflect.Type { return reflectkit.TypeOf[A]() }

// ToType specify that P[B] is the form type.
func (p P[A, B]) ToType() reflect.Type { return reflectkit.TypeOf[B]() }

type _MP struct{ From, To reflect.Type }

func (p _MP) FromType() reflect.Type { return p.From }
func (p _MP) ToType() reflect.Type   { return p.To }

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
