package reflectkit

import (
	"iter"
	"reflect"
)

// VisitValues will return an iterator that will visit each values.
//
// Deprecated: use reflectkit.Visit instead
func VisitValues(v reflect.Value) iter.Seq[V] {
	return Visit(v)
}
