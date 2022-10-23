package errorutil

import "errors"

func IsUserError(err error) bool {
	var ue userError
	return errors.As(err, &ue)
}

func UserError(err error) error {
	return userError{Err: err}
}

type userError struct {
	Err error
}

func (err userError) Error() string {
	return err.Err.Error()
}

func (err userError) Unwrap() error {
	return err.Err
}
