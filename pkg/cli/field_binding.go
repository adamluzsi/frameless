package cli

import (
	"reflect"
	"slices"
	"strconv"

	"go.llib.dev/frameless/pkg/reflectkit/reftree"
)

// A boxed struct's fields have no address. Their stable location is the
// addressable interface slot and the path within its current value. Fields
// reached through a pointer (or slice element) have their own address instead.
type fieldBinding struct {
	root reflect.Value
	path []fieldStep
}

type fieldStep struct {
	parent reflect.Type
	index  int // field or array index; unused for interface unboxing
}

func bindingOf(field reftree.Node) fieldBinding {
	direct := fieldBinding{root: field.Value}
	if field.Value.CanAddr() {
		return direct
	}
	var path []fieldStep
	for n := &field; n.Parent != nil; n = n.Parent {
		p := n.Parent.Value
		switch n.Type {
		case reftree.StructField:
			path = append(path, fieldStep{p.Type(), n.StructField.Index[0]})
		case reftree.ArrayElem:
			path = append(path, fieldStep{p.Type(), n.Index})
		case reftree.InterfaceElem:
			path = append(path, fieldStep{parent: p.Type()})
			if p.CanSet() {
				slices.Reverse(path)
				return fieldBinding{root: p, path: path}
			}
		case reftree.Struct, reftree.Array, reftree.Interface:
		default:
			return direct
		}
	}
	return direct
}

type fieldKey struct {
	root  reflect.Value
	path  string
	owner reflect.Type
	name  string // zero-sized siblings may share their address and type
}

func (b fieldBinding) key(n reftree.Node) fieldKey {
	k := fieldKey{root: b.root, owner: n.Parent.Value.Type(), name: n.StructField.Name}
	for _, s := range b.path {
		k.path += "/" + strconv.Itoa(s.index)
	}
	return k
}

func (s fieldStep) child(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Interface:
		return v.Elem()
	case reflect.Array:
		return v.Index(s.index)
	default:
		return v.Field(s.index)
	}
}

func (b fieldBinding) value() reflect.Value {
	v := b.root
	for _, s := range b.path {
		if !v.IsValid() || v.Type() != s.parent {
			return reflect.Value{} // an enclosing input replaced the dynamic value
		}
		v = s.child(v)
	}
	return v
}

func (b fieldBinding) set(value reflect.Value) {
	v := b.root
	for i, s := range b.path {
		if v.Kind() == reflect.Interface {
			// Copy only at a box boundary, at write time. Other bindings may
			// already have updated siblings in this same struct.
			elem := v.Elem()
			copy := reflect.New(elem.Type()).Elem()
			copy.Set(elem)
			fieldBinding{root: copy, path: b.path[i+1:]}.set(value)
			v.Set(copy)
			return
		}
		v = s.child(v)
	}
	v.Set(value)
}
