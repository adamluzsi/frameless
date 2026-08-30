package reftree

import (
	"errors"
	"iter"
	"reflect"
)

// Stop will command the Visitor to stop with the traversing of the reflect.Value.
const Stop visitCTRL = "stop"

const Break visitCTRL = "break"

// Skip will instruct the Visitor to stop processing of the current node,
// and step back to the outer node and proceed from there with the reflection walking.
//
// It will not break iterations, such as struct field or slice element visiting.
const Skip visitCTRL = "skip"

func Walk(v reflect.Value, visit VisitorFunc) error {
	var (
		vis  = valueVisitor{On: visit}
		root = Node{Value: v}
	)
	return vis.Visit(root)
}

func Iter(v reflect.Value) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		var visitor valueVisitor
		visitor.On = func(v Node) error {
			if !yield(v) {
				return Stop
			}
			return nil
		}
		_ = visitor.Visit(Node{Value: v})
	}
}

type valueVisitor struct {
	On VisitorFunc
	RG *RecursionGuard
}

type VisitorFunc func(node Node) error

type visitCTRL string

func (ctrl visitCTRL) Error() string {
	return string(ctrl)
}

func (vis *valueVisitor) Visit(v Node) (errReturn error) {
	defer vis.errFilter(&errReturn, v)
	guard := vis.getRecursionGuard()
	seen := guard.Seen(v.Value)

	var kind = v.Value.Kind()
	switch kind {
	case reflect.Struct:
		var v = v.next(Node{
			Value: v.Value,
			Type:  Struct,
		})
		if err := vis.yield(v); err != nil {
			return err
		}
		var typ = v.Value.Type()
		for i := range typ.NumField() {
			var (
				field = typ.Field(i)
				value = v.Value.Field(i)
			)
			var vFieldValue = v.next(Node{
				Value:       value,
				Type:        StructField,
				StructField: field,
			})
			if cont, err := vis.yieldElem(vFieldValue); err != nil {
				return err
			} else if !cont {
				break
			}
		}
		return nil

	case reflect.Array, reflect.Slice:
		v := v.next(Node{
			Value: v.Value,
			Type:  vNodeTypeOf[kind],
		})
		if err := vis.yield(v); err != nil {
			return err
		}
		if seen { // avoid recursion with self referencing slices
			return nil
		}
		for i := range v.Value.Len() {
			var elem = v.next(Node{
				Value: v.Value.Index(i),
				Type:  vNodeTypeElemOf[v.Type],
				Index: i,
			})
			if cont, err := vis.yieldElem(elem); err != nil {
				return err
			} else if !cont {
				break
			}
		}
		return nil

	case reflect.Map:
		var v = v.next(Node{
			Value: v.Value,
			Type:  Map,
		})
		if err := vis.yield(v); err != nil {
			return err
		}
		if seen { // avoid recursion with self referencing maps
			return nil
		}
		i := v.Value.MapRange()
		for i.Next() {
			var (
				key   = i.Key()
				value = i.Value()
			)
			var vMapKey = v.next(Node{
				Value:  key,
				Type:   MapKey,
				MapKey: key,
			})
			if cont, err := vis.yieldElem(vMapKey); err != nil {
				return err
			} else if !cont {
				break
			}
			var vMapValue = v.next(Node{
				Value:  value,
				Type:   MapValue,
				MapKey: key,
			})
			if cont, err := vis.yieldElem(vMapValue); err != nil {
				return err
			} else if !cont {
				break
			}
		}
		return nil
	case reflect.Pointer, reflect.Interface:
		v := v.next(Node{
			Value: v.Value,
			Type:  vNodeTypeOf[kind],
		})
		if err := vis.yield(v); err != nil {
			return err
		}
		if v.Value.IsNil() {
			return nil
		}
		if seen { // avoid recursion with pointers
			return nil
		}
		return vis.Visit(v.next(Node{
			Value: v.Value.Elem(),
			Type:  vNodeTypeElemOf[v.Type],
		}))
	default:
		if v.Type == Unknown {
			v.Type = Value
		}
		return vis.yield(v)
	}
}

func (vis *valueVisitor) yield(v Node) error {
	if vis.On != nil {
		if err := vis.On(v); err != nil {
			return err
		}
	}
	return nil
}

func (vis *valueVisitor) yieldElem(v Node) (cont bool, rerr error) {
	if err := vis.yield(v); err != nil {
		if errors.Is(err, Skip) {
			return true, nil
		}
		if errors.Is(err, Break) {
			return false, nil
		}
		return false, err
	}
	if vis.canStepIn(v.Value) {
		if err := vis.Visit(v); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (vis *valueVisitor) getRecursionGuard() *RecursionGuard {
	if vis.RG == nil {
		vis.RG = &RecursionGuard{}
	}
	return vis.RG
}

// kindStepIn tells which kinds the Visitor is able to descend into.
//
// It must mirror the kinds which valueVisitor#Visit handles with a case of
// their own. A kind which is listed here but falls through to the default arm
// of that switch would be yielded twice: once as the element which was just
// visited, then once more by the default arm. A channel is such a kind: it is
// a reference type, yet its content is not reachable without consuming it,
// so there is nothing to step into.
var kindStepIn = map[reflect.Kind]struct{}{
	reflect.Map:       {},
	reflect.Array:     {},
	reflect.Slice:     {},
	reflect.Struct:    {},
	reflect.Pointer:   {},
	reflect.Interface: {},
}

func (vis *valueVisitor) canStepIn(v reflect.Value) bool {
	_, ok := kindStepIn[v.Kind()]
	return ok
}

func (vis *valueVisitor) errFilter(err *error, v Node) {
	if err == nil {
		return
	}
	if *err == nil {
		return
	}
	if errors.Is(*err, Stop) && v.Parent == nil {
		*err = nil
	}
	if errors.Is(*err, Skip) {
		*err = nil
	}
}

var vNodeTypeOf = map[reflect.Kind]NodeType{
	reflect.Array:     Array,
	reflect.Slice:     Slice,
	reflect.Pointer:   Pointer,
	reflect.Interface: Interface,
}

var vNodeTypeElemOf = map[NodeType]NodeType{
	Array:     ArrayElem,
	Slice:     SliceElem,
	Pointer:   PointerElem,
	Interface: InterfaceElem,
}

type RecursionGuard struct {
	seen map[recursionKey]struct{}
}

// recursionKey identifies a memory region which a value tree is able to
// reference back to.
//
// An address on its own is not an identity: a struct shares its address with
// its first field, and a slice or an array shares it with its first element.
// Pairing the address with the type of the referenced value keeps them apart.
type recursionKey struct {
	typ reflect.Type
	ptr uintptr
	len int
}

func (g *RecursionGuard) init() {
	if g.seen == nil {
		g.seen = make(map[recursionKey]struct{})
	}
}

func (g *RecursionGuard) Clone() *RecursionGuard {
	var seen = map[recursionKey]struct{}{}
	for key := range g.seen {
		seen[key] = struct{}{}
	}
	return &RecursionGuard{seen: seen}
}

func (g *RecursionGuard) Seen(v reflect.Value) bool {
	g.init()

	key, ok := g.keyOf(v)
	if !ok {
		return false
	}

	_, seenBefore := g.seen[key]
	g.seen[key] = struct{}{}
	return seenBefore
}

// keyOf returns the cycle identity of the value,
// or false when the value is not able to take part in a reference cycle.
//
// Only reference types are able to close a reference cycle,
// since a struct or an array can only nest into itself through one of them.
//
// A reference qualifies only when it points at a memory region which has room
// for content of its own. This one criteria is what rules out the nil pointer,
// the nil and empty map, the nil and empty slice, and everything zero sized:
//
//   - an empty region has nothing which could reference back to it,
//     so it is unable to close a cycle in the first place;
//   - Go hands out a distinct address only to allocations which occupy space,
//     while the empty ones share a single base address, be it the nil address
//     or the runtime's zero base, so unrelated empty regions cannot be told
//     apart from each other anyway.
//
// Both halves point the same way, so emptiness alone decides it.
func (g *RecursionGuard) keyOf(v reflect.Value) (recursionKey, bool) {
	if !v.IsValid() {
		return recursionKey{}, false
	}

	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || v.Type().Elem().Size() == 0 {
			return recursionKey{}, false
		}
		// The key describes the region being referenced, so the type is taken
		// from the pointee. Pointers of a different type which hold the same
		// address, such as a *T and a pointer to T's first field, describe a
		// different region each, and stay apart because of it.
		return recursionKey{typ: v.Type().Elem(), ptr: v.Pointer()}, true

	case reflect.Map:
		if v.Len() == 0 { // nil maps included
			return recursionKey{}, false
		}
		return recursionKey{typ: v.Type(), ptr: v.Pointer()}, true

	case reflect.Slice:
		if v.Len() == 0 || v.Type().Elem().Size() == 0 { // nil slices included
			return recursionKey{}, false
		}
		// Re-slices of the same backing array share their base address,
		// so the length takes part in the identity as well,
		// else s[:1] and s[:2] would be taken for the same node.
		return recursionKey{typ: v.Type(), ptr: v.Pointer(), len: v.Len()}, true

	default:
		// A value is never memorised, not even when it is addressable,
		// because a struct shares its address with its first field,
		// and an array with its first element, which would make a pointer
		// to that field or element look already seen.
		return recursionKey{}, false
	}
}

// visitor control errors

type errVisitorBreak struct{}

func (errVisitorBreak) Error() string { return "Visitor#Stop" }

type errVisitorStepOver struct{}

func (errVisitorStepOver) Error() string { return "Visitor#StepOver" }

type errVisitorStepOut struct{}

func (errVisitorStepOut) Error() string { return "Visitor#StepOver" }
