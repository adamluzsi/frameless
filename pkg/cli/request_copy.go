package cli

import (
	"reflect"

	"go.llib.dev/frameless/pkg/internal/tagint"
	"go.llib.dev/frameless/pkg/reflectkit/reftree"
)

// requestHandlerPtr leaves pointer handlers in place. For a value handler, only
// references holding input (or leading to it) need request-local storage.
func requestHandlerPtr(h Handler) reflect.Value {
	v := reflect.ValueOf(h)
	ptr := reflect.New(v.Type())
	ptr.Elem().Set(v)
	if v.Kind() == reflect.Pointer {
		return ptr
	}
	p := requestCopyPlan{}
	p.scan(v, nil, scanInputs)
	p.copyInto(ptr.Elem(), v)
	return ptr
}

type requestRef struct {
	typ              reflect.Type
	ptr              uintptr
	length, capacity int // distinguish slice views starting at the same element
}

func requestIdentity(v reflect.Value) (requestRef, bool) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice:
		if !v.IsNil() {
			key := requestRef{typ: v.Type(), ptr: v.Pointer()}
			if v.Kind() == reflect.Slice {
				key.length, key.capacity = v.Len(), v.Cap()
			}
			return key, true
		}
	}
	return requestRef{}, false
}

type scanMode uint8

const (
	scanInputs scanMode = 1 << iota // find CLI fields
	scanData                        // detach mutable data in a tagged field, including JSON merge targets
)

type requestCopyPlan map[requestRef]*requestCopyNode

type requestCopyNode struct {
	seen    scanMode
	parents []*requestCopyNode
	needed  bool
	copy    reflect.Value
}

// Every container leading to a detached reference must itself be detached,
// including a second path discovered after the first path was already scanned.
func (n *requestCopyNode) mark() {
	if n == nil || n.needed {
		return
	}
	n.needed = true
	for _, parent := range n.parents {
		parent.mark()
	}
}

func (p requestCopyPlan) scan(v reflect.Value, parent *requestCopyNode, mode scanMode) {
	if mode == 0 {
		return
	}
	if id, ok := requestIdentity(v); ok {
		n := p[id]
		if n == nil {
			n = new(requestCopyNode)
			p[id] = n
		}
		if parent != nil {
			n.parents = append(n.parents, parent)
			if n.needed {
				parent.mark()
			}
		}
		if mode&scanData != 0 {
			n.mark()
		}
		mode &^= n.seen
		if mode == 0 {
			return
		}
		n.seen |= mode
		parent = n
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Type().Field(i)
			if !walkableField(f) {
				continue
			}
			child := mode
			if f.Tag.Get("json") == "-" {
				child &^= scanData
			}
			if mode&scanInputs != 0 {
				if tagint.IsOpaque(f) {
					child &^= scanInputs
				}
				ref := metaRef{Node: reftree.Node{StructField: f}}
				if f.IsExported() && ref.IsRelevant() {
					parent.mark()
					child |= scanData
				}
			}
			p.scan(v.Field(i), parent, child)
		}
	case reflect.Interface, reflect.Pointer:
		if !v.IsNil() {
			p.scan(v.Elem(), parent, mode)
		}
	case reflect.Array, reflect.Slice:
		for i := range v.Len() {
			p.scan(v.Index(i), parent, mode)
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			p.scan(it.Key(), parent, mode&scanInputs)
			p.scan(it.Value(), parent, mode)
		}
	}
}

// dst is always fresh storage, initially a shallow copy of src. Install each
// new reference before filling it so aliases and back edges reuse that copy.
func (p requestCopyPlan) copyInto(dst, src reflect.Value) {
	if id, ok := requestIdentity(src); ok {
		n := p[id]
		if n == nil || !n.needed {
			return
		}
		if !dst.CanSet() {
			// An unexported embedded pointer can expose writable promoted fields,
			// but reflection cannot replace the pointer in the request copy.
			panic("cli: cannot isolate input through unexported embedded " + src.Type().String() + "; export the field or use a pointer handler")
		}
		if n.copy.IsValid() {
			dst.Set(n.copy)
			return
		}
		switch src.Kind() {
		case reflect.Pointer:
			n.copy = reflect.New(src.Type().Elem()).Convert(src.Type())
			dst.Set(n.copy)
			dst.Elem().Set(src.Elem())
			p.copyInto(dst.Elem(), src.Elem())
		case reflect.Slice:
			n.copy = reflect.MakeSlice(src.Type(), src.Len(), src.Len())
			dst.Set(n.copy)
			reflect.Copy(dst, src)
			for i := range src.Len() {
				p.copyInto(dst.Index(i), src.Index(i))
			}
		case reflect.Map:
			n.copy = reflect.MakeMapWithSize(src.Type(), src.Len())
			dst.Set(n.copy)
			it := src.MapRange()
			for it.Next() {
				dst.SetMapIndex(p.copyValue(it.Key()), p.copyValue(it.Value()))
			}
		}
		return
	}
	switch src.Kind() {
	case reflect.Struct:
		for i := range src.NumField() {
			f := src.Type().Field(i)
			if walkableField(f) {
				p.copyInto(dst.Field(i), src.Field(i))
			}
		}
	case reflect.Array:
		for i := range src.Len() {
			p.copyInto(dst.Index(i), src.Index(i))
		}
	case reflect.Interface:
		if !src.IsNil() {
			dst.Set(p.copyValue(src.Elem()))
		}
	}
}

func (p requestCopyPlan) copyValue(src reflect.Value) reflect.Value {
	dst := reflect.New(src.Type()).Elem()
	dst.Set(src)
	p.copyInto(dst, src)
	return dst
}
