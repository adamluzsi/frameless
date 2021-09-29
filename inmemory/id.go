package inmemory

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
)

func errNotFound(T, id frameless.T) error {
	return fmt.Errorf(`%T entitiy not found by id: %v`, T, id)
}

func newDummyID(T frameless.T) (interface{}, error) {
	id, _ := extid.Lookup(T)

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
		return nil, fmt.Errorf("%T id field type %T is not supported by default, please provide id generator: .NewID", T, id)
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
