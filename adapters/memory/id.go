package memory

import (
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"github.com/adamluzsi/frameless/ports/crud"
	"strconv"
	"sync"
)

func errNotFound(T, id any) error {
	return errorutil.With(crud.ErrNotFound).Detailf(`%T entity not found by id: %v`, T, id)
}

func newDummyID(id any) (interface{}, error) {
	switch id.(type) {
	case string:
		return genStringUID(), nil
	case int:
		return int(genInt64UID()), nil
	case int8:
		return int8(genInt64UID()), nil
	case int16:
		return int16(genInt64UID()), nil
	case int32:
		return int32(genInt64UID()), nil
	case int64:
		return genInt64UID(), nil
	default:
		return nil, fmt.Errorf("%T id type is not supported by default, please provide id generator in the .NewID field", id)
	}
}

var (
	uidMutex  = &sync.Mutex{}
	uidSerial int64
)

func genStringUID() string {
	return strconv.FormatInt(genInt64UID(), 10)
}

func genInt64UID() int64 {
	uidMutex.Lock()
	defer uidMutex.Unlock()
	uidSerial++
	return uidSerial
}
