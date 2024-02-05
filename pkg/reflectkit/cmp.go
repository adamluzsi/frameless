package reflectkit

import (
	"go.llib.dev/frameless/pkg/reflectkit/internal"
	"math/big"
	"net"
	"reflect"
	"time"
)

// Equal will compare two value deeply.
// It looks for Equal method on the value as well.
// You can define how a type's value should be compared by using RegisterEqual.
func Equal(x, y any) bool {
	return internal.Equal(x, y)
}

func RegisterEqual[T any](fn func(x, y T) bool) struct{} {
	internal.RegisterIsEqual(TypeOf[T](), func(x, y reflect.Value) bool {
		return fn(x.Interface().(T), y.Interface().(T))
	})
	return struct{}{}
}

var _ = RegisterEqual[time.Time](func(t1, t2 time.Time) bool {
	return t1.Equal(t2)
})

var _ = RegisterEqual[net.IP](func(ip1, ip2 net.IP) bool {
	return ip1.Equal(ip2)
})

var _ = RegisterEqual[big.Int](func(v1, v2 big.Int) bool {
	return v1.Cmp(&v2) == v2.Cmp(&v1)
})

var _ = RegisterEqual[big.Rat](func(v1, v2 big.Rat) bool {
	return v1.Cmp(&v2) == v2.Cmp(&v1)
})

var _ = RegisterEqual[big.Float](func(v1, v2 big.Float) bool {
	return v1.Cmp(&v2) == v2.Cmp(&v1)
})

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
