package inmemory

import (
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/fixtures"
	"reflect"
	"time"
)

func errNotFound(T, id frameless.T) error {
	return fmt.Errorf(`%T entitiy not found by id: %v`, T, id)
}

func newDummyID(T frameless.T) (interface{}, error) {
	id, _ := extid.Lookup(T)

	var moreOrLessUniqueInt = func() int64 {
		return time.Now().UnixNano() +
			int64(fixtures.SecureRandom.IntBetween(100000, 900000)) +
			int64(fixtures.SecureRandom.IntBetween(1000000, 9000000)) +
			int64(fixtures.SecureRandom.IntBetween(10000000, 90000000)) +
			int64(fixtures.SecureRandom.IntBetween(100000000, 900000000))
	}

	// TODO: deprecate the unsafe id generation approach.
	//       Fixtures are not unique enough for this responsibility.
	//
	switch id.(type) {
	case string:
		return fixtures.Random.StringN(13), nil
	case int:
		return int(moreOrLessUniqueInt()), nil
	case int64:
		return moreOrLessUniqueInt(), nil
	default:
		return fixtures.New(reflect.New(reflect.TypeOf(id)).Elem().Interface()), nil
	}
}
