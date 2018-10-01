package reflects

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/adamluzsi/frameless"
)

// Link will make destination interface be linked with the src value.
// if the src is a pointer to a value, the value will be linked
func Link(src, dst frameless.Entity) (err error) {

	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New(fmt.Sprint(recovered))
		}
	}()

	value := reflect.ValueOf(src)

	if value.Kind() != reflect.Ptr {
		ptr := reflect.New(reflect.TypeOf(src))
		ptr.Elem().Set(value)
		value = ptr
	}

	reflect.ValueOf(dst).Elem().Set(value.Elem())

	return nil
}
