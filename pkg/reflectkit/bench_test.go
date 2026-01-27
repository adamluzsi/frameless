package reflectkit_test

import (
	"reflect"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
)

func Benchmark_typeAssertion(b *testing.B) {
	type T struct{ V string }
	var v = T{V: rnd.String()}
	rType := reflectkit.TypeOf[T]()

	b.Run("reflect.TypeOf", func(b *testing.B) {
		var n int
		for range b.N {
			if reflect.ValueOf(v).Type() == rType {
				n++
			}
		}
	})

	b.Run("type assertion", func(b *testing.B) {
		var n int
		for range b.N {
			if _, ok := any(v).(T); ok {
				n++
			}
		}
	})
}
