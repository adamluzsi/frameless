package iterators

import (
	"github.com/adamluzsi/frameless"
)

func Reduce[
	T, Result any,
	BLK func(Result, T) Result |
		func(Result, T) (Result, error),
](i frameless.Iterator[T], initial Result, blk BLK) (rv Result, rErr error) {
	var do func(Result, T) (Result, error)
	switch blk := any(blk).(type) {
	case func(Result, T) Result:
		do = func(result Result, t T) (Result, error) {
			return blk(result, t), nil
		}
	case func(Result, T) (Result, error):
		do = blk
	}
	defer func() {
		cErr := i.Close()
		if rErr != nil {
			return
		}
		rErr = cErr
	}()
	var v = initial
	for i.Next() {
		var err error
		v, err = do(v, i.Value())
		if err != nil {
			return v, err
		}
	}
	return v, i.Err()
}
