package reflectkit

import (
	"iter"
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit/reftree"
)

func Visit(v reflect.Value) iter.Seq[reftree.Node] {
	return reftree.Iter(v)
}
