package reflectkit

import (
	"errors"
	"fmt"
	"iter"
	"reflect"
	"strings"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit/refnode"
)

type VisitorFunc func(receiver *Visitor, v V) error

type Visitor struct {
	OnVisit VisitorFunc
	OnKind  map[reflect.Kind]VisitorFunc
	OnType  map[reflect.Type]VisitorFunc
}

func (visitor Visitor) VisitValue(v reflect.Value) error {
	return visitor.Visit(V{Value: v})
}

type errVisitorBreak struct{}

func (errVisitorBreak) Error() string { return "Visitor#Break" }

func (visitor Visitor) Break() error           { return errVisitorBreak{} }
func (visitor Visitor) isBreak(err error) bool { return errors.Is(err, errVisitorBreak{}) }

func (visitor *Visitor) yield(v V) error {
	if visitor.OnVisit != nil {
		if err := visitor.OnVisit(visitor, v); err != nil {
			return err
		}
	}

	if 0 < len(visitor.OnType) {
		if fn, ok := visitor.OnType[v.Value.Type()]; ok {
			err := fn(visitor, v)
			if err != nil {
				return err
			}
		}
	}

	if 0 < len(visitor.OnKind) {
		if fn, ok := visitor.OnKind[v.Value.Kind()]; ok {
			err := fn(visitor, v)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (visitor Visitor) Visit(v V) (errReturn error) {
	defer visitor.errFilter(&errReturn, v)

	var kind = v.Value.Kind()
	switch kind {
	case reflect.Struct:
		v := v.next(V{
			Value:    v.Value,
			NodeType: refnode.Struct,
		})
		if err := visitor.yield(v); err != nil {
			return err
		}
		for field, value := range IterStructFields(v.Value) {
			vFieldValue := v.next(V{
				Value:       value,
				NodeType:    refnode.StructField,
				StructField: field,
			})
			if err := visitor.Visit(vFieldValue); err != nil {
				return err
			}
		}
		return nil

	case reflect.Array, reflect.Slice:
		v := v.next(V{
			Value:    v.Value,
			NodeType: vNodeTypeOf[kind],
		})
		if err := visitor.yield(v); err != nil {
			return err
		}
		for i := range v.Value.Len() {
			vElem := v.next(V{
				Value:    v.Value.Index(i),
				NodeType: vNodeTypeElemOf[v.NodeType],
				Index:    i,
			})
			if err := visitor.Visit(vElem); err != nil {
				return err
			}
		}
		return nil

	case reflect.Map:
		v := v.next(V{
			Value:    v.Value,
			NodeType: refnode.Map,
		})
		if err := visitor.yield(v); err != nil {
			return err
		}
		for key, value := range IterMap(v.Value) {
			vMapKey := v.next(V{
				Value:    key,
				NodeType: refnode.MapKey,
				MapKey:   key,
			})
			if err := visitor.Visit(vMapKey); err != nil {
				return err
			}
			vMapValue := v.next(V{
				Value:    value,
				NodeType: refnode.MapValue,
				MapKey:   key,
			})
			if err := visitor.Visit(vMapValue); err != nil {
				return err
			}
		}
		return nil
	case reflect.Pointer, reflect.Interface:
		v := v.next(V{
			Value:    v.Value,
			NodeType: vNodeTypeOf[kind],
		})
		if err := visitor.yield(v); err != nil {
			return err
		}
		if v.Value.IsNil() {
			return nil
		}
		return visitor.Visit(v.next(V{
			Value:    v.Value.Elem(),
			NodeType: vNodeTypeElemOf[v.NodeType],
		}))
	default:
		if v.NodeType == refnode.Unknown {
			v.NodeType = refnode.Value
		}
		return visitor.yield(v)
	}
}

func (visitor *Visitor) errFilter(err *error, v V) {
	if err == nil {
		return
	}
	if *err == nil {
		return
	}
	if visitor.isBreak(*err) && v.Parent == nil {
		*err = nil
	}
}

func (visitor Visitor) Iter(v reflect.Value) iter.Seq[V] {
	return func(yield func(V) bool) {
		var OnVisit VisitorFunc = func(receiver *Visitor, v V) error {
			if !yield(v) {
				return visitor.Break()
			}
			return nil
		}
		if visitor.OnVisit != nil {
			og := visitor.OnVisit
			iterVisit := OnVisit
			OnVisit = func(receiver *Visitor, v V) error {
				return errorkitlite.Merge(iterVisit(receiver, v), og(receiver, v))
			}
		}
		visitor := visitor
		visitor.OnVisit = OnVisit
		visitor.Visit(V{Value: v})
	}
}

func Visit(v reflect.Value) iter.Seq[V] {
	return func(yield func(V) bool) {
		visit(yield, V{Value: v})
	}
}

func visit(yield func(V) bool, v V) bool {
	var kind = v.Value.Kind()
	switch kind {
	case reflect.Struct:
		v := v.next(V{
			Value:    v.Value,
			NodeType: refnode.Struct,
		})
		if !yield(v) {
			return false
		}
		for field, value := range IterStructFields(v.Value) {
			v := v.next(V{
				Value:       value,
				NodeType:    refnode.StructField,
				StructField: field,
			})
			if !visit(yield, v) {
				return false
			}
		}
		return true

	case reflect.Array, reflect.Slice:
		v := v.next(V{
			Value:    v.Value,
			NodeType: vNodeTypeOf[kind],
		})
		if !yield(v) {
			return false
		}
		for i := range v.Value.Len() {
			vElem := v.next(V{
				Value:    v.Value.Index(i),
				NodeType: vNodeTypeElemOf[v.NodeType],
				Index:    i,
			})
			if !visit(yield, vElem) {
				return false
			}
		}
		return true

	case reflect.Map:
		v := v.next(V{
			Value:    v.Value,
			NodeType: refnode.Map,
		})
		if !yield(v) {
			return false
		}
		for key, value := range IterMap(v.Value) {
			vMapKey := v.next(V{
				Value:    key,
				NodeType: refnode.MapKey,
				MapKey:   key,
			})
			if !visit(yield, vMapKey) {
				return false
			}
			vMapValue := v.next(V{
				Value:    value,
				NodeType: refnode.MapValue,
				MapKey:   key,
			})
			if !visit(yield, vMapValue) {
				return false
			}
		}
		return true
	case reflect.Pointer, reflect.Interface:
		v := v.next(V{
			Value:    v.Value,
			NodeType: vNodeTypeOf[kind],
		})
		if !yield(v) {
			return false
		}
		if v.Value.IsNil() {
			return true
		}
		return visit(yield, v.next(V{
			Value:    v.Value.Elem(),
			NodeType: vNodeTypeElemOf[v.NodeType],
		}))
	default:
		if v.NodeType == refnode.Unknown {
			v.NodeType = refnode.Value
		}
		return yield(v)
	}
}

type V struct {
	Value reflect.Value

	NodeType refnode.Type

	Index       int
	MapKey      reflect.Value
	StructField reflect.StructField

	Parent *V
}

func (v V) Is(nt refnode.Type) bool {
	if v.NodeType == nt {
		return true
	}
	if nt.IsBranchType() && v.Parent != nil {
		return v.Parent.Is(nt)
	}
	return false
}

func (v V) Path() refnode.Path {
	i := iterkit.Map(v.Iter(), func(v V) refnode.Type {
		return v.NodeType
	})
	return iterkit.Collect(i)
}

func (v V) validate() error {
	if v.NodeType == refnode.Unknown {
		return errorkit.F("unknown Path Kind")
	}
	if v.NodeType == refnode.StructField {
		if v.Parent == nil {
			return errorkit.F("StructField Path doesn't have Path#Parent")
		}
		if v.Parent.NodeType != refnode.Struct {
			return errorkit.F("StructField Path doesn't have Struct Parent")
		}
	}
	if v.Parent != nil && v.Parent.NodeType == refnode.Struct && v.NodeType != refnode.StructField {
		return errorkit.F("Unexpected %s Path Kind, StructField was expected", v.NodeType.String())
	}
	if v.NodeType == refnode.MapKey || v.NodeType == refnode.MapValue {
		if v.Parent == nil {
			return errorkit.F("%s Path doesn't have Path#Parent", v.NodeType.String())
		}
		if v.Parent.NodeType != refnode.Map {
			return errorkit.F("%s Path doesn't have Map Parent", v.NodeType.String())
		}
		if v.NodeType == refnode.MapValue && v.MapKey == (reflect.Value{}) {
			return errorkit.F("%s is missing the Path#MapKey", v.NodeType.String())
		}
	}
	return nil
}

func (v V) String() string {
	var (
		out   string
		parts []string
	)
	for path := range v.Iter() {
		parts = append(parts, path.NodeType.String())
	}
	if 0 < len(parts) {
		out += strings.Join(parts, "/") + " "
	}
	if v.Value.CanInterface() {
		out += fmt.Sprintf("%#v", v.Value.Interface())
	} else {
		out += fmt.Sprintf("%#v", v.Value.String())
	}
	return out
}

// func (v V) isZero() bool {
// 	return v.NodeType == 0 &&
// 		v.Index == 0 &&
// 		v.MapKey == reflect.Value{} &&
// 		len(v.StructField.Name) == 0 &&
// 		len(v.StructField.PkgPath) == 0 &&
// 		v.Parent == nil
// }

func (v V) isUnindentifiedRoot() bool {
	return v.NodeType == refnode.Unknown
}

func (v V) next(n V) V {
	if !v.isUnindentifiedRoot() {
		n.Parent = &v
	}
	if err := n.validate(); err != nil {
		panic(err)
	}
	return n
}

func (v V) IterUp() iter.Seq[V] {
	return func(yield func(V) bool) {
		var cur = v
		for {
			if !yield(cur) {
				return
			}
			if cur.Parent == nil {
				break
			}
			cur = *cur.Parent
		}
	}
}

func (v V) Iter() iter.Seq[V] {
	return func(yield func(V) bool) {
		for v := range iterkit.Reverse(v.IterUp()) {
			if !yield(v) {
				return
			}
		}
	}
}

var vNodeTypeOf = map[reflect.Kind]refnode.Type{
	reflect.Array:     refnode.Array,
	reflect.Slice:     refnode.Slice,
	reflect.Pointer:   refnode.Pointer,
	reflect.Interface: refnode.Interface,
}

var vNodeTypeElemOf = map[refnode.Type]refnode.Type{
	refnode.Array:     refnode.ArrayElem,
	refnode.Slice:     refnode.SliceElem,
	refnode.Pointer:   refnode.PointerElem,
	refnode.Interface: refnode.InterfaceElem,
}

// func (v V) PathString() string {
// 	var path string
// 	for v := range v.IterUp() {
// 		switch v.NodeType {
// 		case refnode.StructField:
// 			path = fmt.Sprintf(".%s%s", v.StructField.Name, path)
// 		case refnode.Array, refnode.Slice, refnode.Map, refnode.Struct:
// 			path = fmt.Sprintf("%s%s", v.Value.Type().String(), path)
// 		case refnode.ArrayElem, refnode.SliceElem:
// 			path = fmt.Sprintf("[%d]%s", v.Index, path)
// 		case refnode.MapKey:
// 			path = fmt.Sprintf("{%#v:%s}", v.MapKey.Interface(), path)
// 		case refnode.MapValue:
// 			path = fmt.Sprintf("[%#v]%s", v.MapKey.Interface(), path)
// 		case refnode.InterfaceElem, refnode.PointerElem:
// 			var (
// 				val string
// 				typ string
// 			)
// 			if IsNil(v.Value) {
// 				val = "(nil)"
// 			} else {
// 				val = fmt.Sprintf("(%s)", path)
// 			}
// 			if v.Parent != nil {
// 				typ = v.Parent.Value.Type().String()
// 			} else {
// 				typ = v.Value.Type().String()
// 			}
// 			path = fmt.Sprintf("(%s)(%s)", typ, val)
// 		default:
// 			path = fmt.Sprintf("%#v%s", v.Value.Interface(), path)
// 		}
// 	}
// 	return path
// }
