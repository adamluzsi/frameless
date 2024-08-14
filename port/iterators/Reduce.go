package iterators

func Reduce[
	R, T any,
	FN func(R, T) R |
		func(R, T) (R, error),
](i Iterator[T], initial R, blk FN) (result R, rErr error) {
	var do func(R, T) (R, error)
	switch blk := any(blk).(type) {
	case func(R, T) R:
		do = func(result R, t T) (R, error) {
			return blk(result, t), nil
		}
	case func(R, T) (R, error):
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
