package errorutil

import "errors"

func Tag[Tag ~struct{}](err error, tag Tag) error {
	var tagErr *tagError
	if !errors.As(err, &tagErr) {
		return &tagError{
			Err:  err,
			Tags: []any{tag},
		}
	}
	tagErr.Tags = append(tagErr.Tags, tag)
	return err
}

func HasTag(err error, tags ...any) bool {
	var tagErr *tagError
	if !errors.As(err, &tagErr) {
		return false
	}
	for _, expectedTag := range tags {
		for _, actualTag := range tagErr.Tags {
			if expectedTag == actualTag {
				return true
			}
		}
	}
	return false
}

type tagError struct {
	Err  error
	Tags []any
}

func (err *tagError) Error() string { return err.Err.Error() }
func (err *tagError) Unwrap() error { return err.Err }
