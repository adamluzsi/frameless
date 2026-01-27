package reftree

import (
	"fmt"
	"iter"
	"reflect"
	"strings"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

type Node struct {
	Value  reflect.Value
	Type   NodeType
	Parent *Node

	Index       int
	MapKey      reflect.Value
	StructField reflect.StructField
}

func (v Node) Is(nt NodeType) bool {
	if v.Type == nt {
		return true
	}
	if nt.IsBranchType() && v.Parent != nil {
		return v.Parent.Is(nt)
	}
	return false
}

func (v Node) Path() Path {
	i := iterkit.Map(v.Iter(), func(v Node) NodeType {
		return v.Type
	})
	return iterkit.Collect(i)
}

func (v Node) validate() error {
	if v.Type == Unknown {
		return errorkit.F("unknown Path Kind")
	}
	if v.Type == StructField {
		if v.Parent == nil {
			return errorkit.F("StructField Path doesn't have Path#Parent")
		}
		if v.Parent.Type != Struct {
			return errorkit.F("StructField Path doesn't have Struct Parent")
		}
	}
	if v.Parent != nil && v.Parent.Type == Struct && v.Type != StructField {
		return errorkit.F("Unexpected %s Path Kind, StructField was expected", v.Type.String())
	}
	if v.Type == MapKey || v.Type == MapValue {
		if v.Parent == nil {
			return errorkit.F("%s Path doesn't have Path#Parent", v.Type.String())
		}
		if v.Parent.Type != Map {
			return errorkit.F("%s Path doesn't have Map Parent", v.Type.String())
		}
		if v.Type == MapValue && v.MapKey == (reflect.Value{}) {
			return errorkit.F("%s is missing the Path#MapKey", v.Type.String())
		}
	}
	return nil
}

// func (v Node) Pathway() string {
// 	var (
// 		out   string
// 		parts []string
// 	)
// 	for path := range v.Iter() {
// 		parts = append(parts, path.Type.String())
// 	}
// 	if 0 < len(parts) {
// 		out += strings.Join(parts, "/") + " "
// 	}
// 	if v.Value.CanInterface() {
// 		out += fmt.Sprintf("%#v", v.Value.Interface())
// 	} else {
// 		out += fmt.Sprintf("%#v", v.Value.String())
// 	}
// 	return out
// }

// func (v V) isZero() bool {
// 	return v.NodeType == 0 &&
// 		v.Index == 0 &&
// 		v.MapKey == reflect.Value{} &&
// 		len(v.StructField.Name) == 0 &&
// 		len(v.StructField.PkgPath) == 0 &&
// 		v.Parent == nil
// }

func (v Node) isUnindentifiedRoot() bool {
	return v.Type == Unknown
}

func (v Node) next(n Node) Node {
	if !v.isUnindentifiedRoot() {
		n.Parent = &v
	}
	if err := n.validate(); err != nil {
		panic(err)
	}
	return n
}

func (v Node) IterUpward() iter.Seq[Node] {
	return func(yield func(Node) bool) {
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

func (v Node) Iter() iter.Seq[Node] {
	return func(yield func(Node) bool) {
		for v := range iterkit.Reverse(v.IterUpward()) {
			if !yield(v) {
				return
			}
		}
	}
}

type NodeType int

var branchTypes = map[NodeType]struct{}{
	ArrayElem:     {},
	SliceElem:     {},
	PointerElem:   {},
	InterfaceElem: {},
	StructField:   {},
	MapKey:        {},
	MapValue:      {},
}

func (k NodeType) IsBranchType() bool {
	_, ok := branchTypes[k]
	return ok
}

func (k NodeType) String() string {
	switch k {
	case Unknown:
		return "Unknown"
	case Value:
		return "Value"
	case Struct:
		return "Struct"
	case StructField:
		return "StructField"
	case Array:
		return "Array"
	case ArrayElem:
		return "ArrayElem"
	case Slice:
		return "Slice"
	case SliceElem:
		return "SliceElem"
	case Interface:
		return "Interface"
	case InterfaceElem:
		return "InterfaceElem"
	case Pointer:
		return "Pointer"
	case PointerElem:
		return "PointerElem"
	case Map:
		return "Map"
	case MapKey:
		return "MapKey"
	case MapValue:
		return "MapValue"
	default:
		return fmt.Sprintf("%v", int(k))
	}
}

const (
	Unknown NodeType = iota

	Value

	Struct
	StructField

	Array
	ArrayElem

	Slice
	SliceElem

	Interface
	InterfaceElem

	Pointer
	PointerElem

	Map
	MapKey
	MapValue
)

type Path []NodeType

func (p Path) String() string {
	return strings.Join(slicekit.Map(p, NodeType.String), "/")
}

func (p Path) Contains(ntp ...NodeType) bool {
	if len(ntp) == 0 {
		return true
	}
	var i int
	for _, gnt := range p {
		ent, ok := slicekit.Lookup(ntp, i)
		if !ok {
			break
		}
		if ent == gnt {
			i++
			continue
		} else if 0 < i {
			return false
		}
	}
	return len(ntp) == i
}

// func (v V) PathString() string {
// 	var path string
// 	for v := range v.IterUpward() {
// 		switch v.NodeType {
// 		case StructField:
// 			path = fmt.Sprintf(".%s%s", v.StructField.Name, path)
// 		case Array, Slice, Map, Struct:
// 			path = fmt.Sprintf("%s%s", v.Value.Type().String(), path)
// 		case ArrayElem, SliceElem:
// 			path = fmt.Sprintf("[%d]%s", v.Index, path)
// 		case MapKey:
// 			path = fmt.Sprintf("{%#v:%s}", v.MapKey.Interface(), path)
// 		case MapValue:
// 			path = fmt.Sprintf("[%#v]%s", v.MapKey.Interface(), path)
// 		case InterfaceElem, PointerElem:
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
