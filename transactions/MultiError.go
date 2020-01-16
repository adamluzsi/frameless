package transactions

import (
	"strings"
)

type MultiError struct {
	Errors []error
}

func (err MultiError) Error() string {
	return strings.Join(err.mapErrorMessages(), "\n")
}

func (err MultiError) mapErrorMessages() []string {
	var messages []string
	for _, errElement := range err.Errors {
		messages = append(messages, errElement.Error())
	}
	return messages
}
