package reflectkit

import (
	"reflect"
	"strings"
)

type UID struct {
	t reflect.Type
	p uintptr
	i string
}

func UIDOf(v any, i ...string) UID {
	rv := ToValue(v)
	var uid UID
	uid.t = rv.Type()
	if rv.CanAddr() {
		uid.p = rv.UnsafeAddr()
	}
	uid.i = strings.Join(i, ".")
	return uid
}
