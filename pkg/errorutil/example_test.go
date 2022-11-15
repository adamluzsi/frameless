package errorutil_test

import (
	"errors"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/errorutil"
	"math/rand"
)

func ExampleUserError() {
	fooEntityID := rand.Int()
	bazEntityID := rand.Int()

	err := fmt.Errorf("foo bar baz")
	err = errorutil.UserError{
		Err:     err,
		Code:    "foo-is-forbidden-with-active-baz",
		Message: fmt.Sprintf("It is forbidden to execute Foo(ID:%d) when you have an active Baz (ID:%d)", fooEntityID, bazEntityID),
	}

	// retrieve with errors pkg
	if ue := (errorutil.UserError{}); errors.As(err, &ue) {
		fmt.Printf("%#v\n", ue)
	}
	if errors.Is(err, errorutil.UserError{}) {
		fmt.Println("it's a Layer 8 error")
	}

	// retrieve with errorutil pkg
	if userError, ok := errorutil.LookupUserError(err); ok {
		fmt.Printf("%#v\n", userError)
	}
	if errorutil.IsUserError(err) {
		fmt.Println("it's a Layer 8 error")
	}
}

func ExampleIsUserError() {
	err := fmt.Errorf("foo bar baz")
	err = errorutil.UserError{
		Err:     err,
		Code:    "const-err-scenario-code",
		Message: "some message for the dev",
	}
	if errorutil.IsUserError(err) {
		fmt.Println("it's a Layer 8 error")
	}
}

func ExampleLookupUserError() {
	err := fmt.Errorf("foo bar baz")
	err = errorutil.UserError{
		Err:     err,
		Code:    "const-err-scenario-code",
		Message: "some message for the dev",
	}
	if userError, ok := errorutil.LookupUserError(err); ok {
		fmt.Printf("%#v\n", userError)
	}
}

func ExampleTag() {
	type MyTag struct{}
	err := fmt.Errorf("foo bar baz")
	err = errorutil.Tag(err, MyTag{})
	errorutil.HasTag(err, MyTag{}) // true
}

func ExampleHasTag() {
	type MyTag struct{}
	err := fmt.Errorf("foo bar baz")
	errorutil.HasTag(err, MyTag{}) // false
	err = errorutil.Tag(err, MyTag{})
	errorutil.HasTag(err, MyTag{}) // true
}

func ExampleMerge() {
	// creates an error value that combines the input errors.
	err := errorutil.Merge(fmt.Errorf("foo"), fmt.Errorf("bar"), nil)
	_ = err
}
