package reflects

import (
	"errors"
	"fmt"
	"reflect"
)

// Link will make destination interface be linked with the src value.
//
func Link(src, ptr interface{}) (err error) {
	vPtr := reflect.ValueOf(ptr)

	if vPtr.Kind() != reflect.Ptr {
		return errors.New(`pointer type destination expected`)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New(fmt.Sprint(recovered))
		}
	}()

	vPtr.Elem().Set(reflect.ValueOf(src))

	return nil
}
