package memory

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"

	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/pkg/reflects"
	"github.com/adamluzsi/frameless/ports/crud"
)

func errNotFound(T, id any) error {
	return errorutil.With(crud.ErrNotFound).Detailf(`%T entity not found by id: %v`, T, id)
}

func MakeID[ID any](context.Context) (ID, error) {
	returnError := func() (ID, error) {
		var id ID
		const format = "%T id type is not supported by default, please provide id generator in the .NewID field"
		return id, fmt.Errorf(format, id)
	}

	switch reflect.TypeOf(*new(ID)).Kind() {
	case reflect.String:
		id, ok := reflects.Cast[ID](genStringUID())
		if !ok {
			return returnError()
		}
		return id, nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		id, ok := reflects.Cast[ID](genIntUID())
		if !ok {
			return returnError()
		}
		return id, nil

	default:
		return returnError()

	}
}

var (
	uidMutex  = &sync.Mutex{}
	uidSerial int64
)

func genStringUID() string {
	return strconv.FormatInt(int64(genIntUID()), 10)
}

func genIntUID() int {
	uidMutex.Lock()
	defer uidMutex.Unlock()
	uidSerial++
	return int(uidSerial)
}
