package errutils

import (
	"errors"
	"strings"
)

func Clean(errs []error) []error {
	var cleanErrs []error
	for _, err := range errs {
		if err == nil {
			continue
		}
		cleanErrs = append(cleanErrs, err)
	}
	return cleanErrs
}

func Merge(errs ...error) error {
	errs = Clean(errs)
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	return Errors(errs)
}

type Errors []error

func (errs Errors) Error() string {
	var msgs []string
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "\n")
}

func (errs Errors) As(target any) bool {
	for _, err := range errs {
		if errors.As(err, target) {
			return true
		}
	}
	return false
}

func (errs Errors) Clean() Errors {
	var errors Errors
	for _, err := range errs {
		if err == nil {
			continue
		}
		errors = append(errors, err)
	}
	return errors
}

func (errs Errors) Err() error {
	return Merge(errs...)
}
