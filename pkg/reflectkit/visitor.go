package reflectkit

import (
	"fmt"
	"iter"
	"reflect"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/reflectkit/refnode"
)

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

func (v V) next(n V) V {
	if v.NodeType != refnode.Unknown {
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
