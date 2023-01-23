package rfc7807_test

import "github.com/adamluzsi/frameless/pkg/errorutil"

const ErrExample errorutil.Error = "example error"

var ErrUsrMistake = errorutil.UserError{
	ID:      "foo-bar-baz",
	Message: "It's a Layer 8 Issue",
}

type ExampleExtension struct {
	Error ExampleExtensionError `json:"error"`
}

type ExampleExtensionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
