package reflectkit

import (
	"iter"
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit/refvis"
)

func Visit(v reflect.Value) iter.Seq[refvis.Node] {
	return refvis.Iter(v)
}
