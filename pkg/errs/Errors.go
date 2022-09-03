package errs

import "strings"

type Errors []error

func (errs Errors) Error() string {
	var msgs []string
	for _, err := range errs.Clean() {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "\n")
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
	errors := errs.Clean()
	if len(errors) == 0 {
		return nil
	}
	if len(errors) == 1 {
		return errors[0]
	}
	return errors
}
