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
		vis  = visitor{On: visit}
		root = Node{Value: v}
	)
	return vis.Visit(root)
}

func Iter(v reflect.Value) iter.Seq[Node] {
	return func(yield func(Node) bool) {
		var visitor visitor
		visitor.On = func(v Node) error {
			if !yield(v) {
				return Stop
			}
			return nil
		}
		_ = visitor.Visit(Node{Value: v})
	}
}

type visitor struct {
	On VisitorFunc
	RG *RecursionGuard
}

type VisitorFunc func(node Node) error

type visitCTRL string

func (ctrl visitCTRL) Error() string {
	return string(ctrl)
}

func (vis *visitor) Visit(v Node) (errReturn error) {
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
		var (
			typ = v.Value.Type()
			num = typ.NumField()
		)
		for i := 0; i < num; i++ {
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

func (vis *visitor) yield(v Node) error {
	if vis.On != nil {
		if err := vis.On(v); err != nil {
			return err
		}
	}
	return nil
}

func (vis *visitor) yieldElem(v Node) (cont bool, rerr error) {
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

func (vis *visitor) getRecursionGuard() *RecursionGuard {
	if vis.RG == nil {
		vis.RG = &RecursionGuard{}
	}
	return vis.RG
}

var kindStepIn = map[reflect.Kind]struct{}{
	reflect.Map:       {},
	reflect.Chan:      {},
	reflect.Array:     {},
	reflect.Slice:     {},
	reflect.Struct:    {},
	reflect.Pointer:   {},
	reflect.Interface: {},
}

func (vis *visitor) canStepIn(v reflect.Value) bool {
	_, ok := kindStepIn[v.Kind()]
	return ok
}

func (vis *visitor) errFilter(err *error, v Node) {
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
	seen map[uintptr]struct{}
}

func (g *RecursionGuard) init() {
	if g.seen == nil {
		g.seen = make(map[uintptr]struct{})
	}
}

func (g *RecursionGuard) Seen(v reflect.Value) bool {
	g.init()

	if v.Kind() == reflect.Pointer {
		return g.seenPointer(v)
	}

	return g.seenValue(v)
}

func (g *RecursionGuard) seenValue(v reflect.Value) bool {
	// if we are seeint the root value of an addressable value,
	// we memorise it for future reference
	if len(g.seen) == 0 && v.CanAddr() {
		g.seenPtr(v.Addr().Pointer())
	}
	return false
}

func (g *RecursionGuard) seenPointer(v reflect.Value) bool {
	return g.seenPtr(v.Pointer())
}

func (g *RecursionGuard) seenPtr(ptr uintptr) bool {
	_, seenBefore := g.seen[ptr]
	g.seen[ptr] = struct{}{}
	return seenBefore
}

// visitor control errors

type errVisitorBreak struct{}

func (errVisitorBreak) Error() string { return "Visitor#Stop" }

type errVisitorStepOver struct{}

func (errVisitorStepOver) Error() string { return "Visitor#StepOver" }

type errVisitorStepOut struct{}

func (errVisitorStepOut) Error() string { return "Visitor#StepOver" }
