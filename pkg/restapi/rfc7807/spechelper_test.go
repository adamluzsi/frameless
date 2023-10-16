package rfc7807_test

import "go.llib.dev/frameless/pkg/errorkit"

const ErrExample errorkit.Error = "example error"

var ErrUsrMistake = errorkit.UserError{
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
