package fixtures

import (
	"github.com/adamluzsi/frameless/reflects"
	"math/rand"
	"reflect"
	"sync"
	"time"

	"github.com/Pallinder/go-randomdata"
	"github.com/adamluzsi/frameless"
)

// New returns a populated entity for a given business data entity.
// This is primary and only used for testing.
// With fixtures, it become easy to create generic query objects that use test cases that does not specify the concrete Structure type.
func New(entity frameless.Entity) frameless.Entity {
	ptr := reflect.New(reflects.BaseTypeOf(entity))
	elem := ptr.Elem()

	for i := 0; i < elem.NumField(); i++ {
		fv := elem.Field(i)

		if fv.CanSet() {
			newValue := newValue(fv)

			if newValue.IsValid() {
				fv.Set(newValue)
			}
		}
	}

	return ptr.Interface()
}

var mutex sync.Mutex

func newValue(value reflect.Value) reflect.Value {
	switch value.Type().Kind() {

	case reflect.Bool:
		mutex.Lock()
		defer mutex.Unlock()
		return reflect.ValueOf(randomdata.Boolean())

	case reflect.String:
		mutex.Lock()
		defer mutex.Unlock()
		return reflect.ValueOf(randomdata.SillyName())

	case reflect.Int:
		return reflect.ValueOf(rand.Int())

	case reflect.Int8:
		return reflect.ValueOf(int8(rand.Int()))

	case reflect.Int16:
		return reflect.ValueOf(int16(rand.Int()))

	case reflect.Int32:
		return reflect.ValueOf(rand.Int31())

	case reflect.Int64:
		switch value.Interface().(type) {
		case time.Duration:
			return reflect.ValueOf(time.Duration(rand.Int63()))
		default:
			return reflect.ValueOf(rand.Int63())
		}

	case reflect.Uint:
		return reflect.ValueOf(uint(rand.Uint32()))

	case reflect.Uint8:
		return reflect.ValueOf(uint8(rand.Uint32()))

	case reflect.Uint16:
		return reflect.ValueOf(uint16(rand.Uint64()))

	case reflect.Uint32:
		return reflect.ValueOf(rand.Uint32())

	case reflect.Uint64:
		return reflect.ValueOf(rand.Uint64())

	case reflect.Float32:
		return reflect.ValueOf(rand.Float32())

	case reflect.Float64:
		return reflect.ValueOf(rand.Float64())

	case reflect.Complex64:
		return reflect.ValueOf(complex64(42))

	case reflect.Complex128:
		return reflect.ValueOf(complex128(42.42))

	case reflect.Array:
		return reflect.New(value.Type()).Elem()

	case reflect.Slice:
		return reflect.MakeSlice(value.Type(), 0, 0)

	case reflect.Chan:
		return reflect.MakeChan(value.Type(), 0)

	case reflect.Map:
		return reflect.MakeMap(value.Type())

	case reflect.Ptr:
		return reflect.New(value.Type().Elem())

	case reflect.Uintptr:
		return reflect.ValueOf(uintptr(rand.Int()))

	case reflect.Struct:
		switch value.Interface().(type) {
		case time.Time:
			return reflect.ValueOf(time.Now().UTC().Add(time.Duration(rand.Int()) * time.Hour))
		default:
			return reflect.ValueOf(New(value.Interface())).Elem()
		}

	default:
		//reflect.UnsafePointer
		//reflect.Interface
		//reflect.Func
		return reflect.ValueOf(nil)
	}
}
