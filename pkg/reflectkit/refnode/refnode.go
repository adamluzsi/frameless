package refnode

import (
	"fmt"

	"go.llib.dev/frameless/pkg/slicekit"
)

type Type int

var branchTypes = map[Type]struct{}{
	ArrayElem:     {},
	SliceElem:     {},
	PointerElem:   {},
	InterfaceElem: {},
	StructField:   {},
	MapKey:        {},
	MapValue:      {},
}

func (k Type) IsBranchType() bool {
	_, ok := branchTypes[k]
	return ok
}

func (k Type) String() string {
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
	Unknown Type = iota

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

	// UnsafePointer
)

type Path []Type

func (p Path) Contains(ntp ...Type) bool {
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
