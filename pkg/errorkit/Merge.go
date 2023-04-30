package errorkit

import (
	"errors"
	"strings"
)

// Merge will combine all given non nil error values into a single error value.
// If no valid error is given, nil is returned.
// If only a single non nil error value is given, the error value is returned.
func Merge(errs ...error) error {
	errs = clean(errs)
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return multiError(errs)
}

func clean(errs []error) []error {
	var cleanErrs []error
	for _, err := range errs {
		if err == nil {
			continue
		}
		cleanErrs = append(cleanErrs, err)
	}
	return cleanErrs
}

type multiError []error

func (errs multiError) Error() string {
	var msgs []string
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "\n")
}

func (errs multiError) As(target any) bool {
	for _, err := range errs {
		if errors.As(err, target) {
			return true
		}
	}
	return false
}

func (errs multiError) Is(target error) bool {
	for _, err := range errs {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
