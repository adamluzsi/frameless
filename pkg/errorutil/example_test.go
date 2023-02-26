package errorutil_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/adamluzsi/frameless/pkg/errorutil"
)

func ExampleUserError() {
	fooEntityID := rand.Int()
	bazEntityID := rand.Int()

	usrerr := errorutil.UserError{
		ID:      "foo-is-forbidden-with-active-baz",
		Message: "It is forbidden to execute Foo when you have an active Baz",
	}

	var err error = usrerr.With().Detailf("Foo(ID:%d) /Baz(ID:%d)", fooEntityID, bazEntityID)

	// add some details using error wrapping
	err = fmt.Errorf("some wrapper layer: %w", err)

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
}

func ExampleUserError_With() {
	usrErr := errorutil.UserError{
		ID:      "foo-is-forbidden-with-active-baz",
		Message: "It is forbidden to execute Foo when you have an active Baz",
	}

	// returns with Err that has additional concrete details
	_ = usrErr.With().Detailf("Foo(ID:%d) /Baz(ID:%d)", 42, 7)

}

func ExampleLookupUserError() {
	err := errorutil.UserError{
		ID:      "constant-err-scenario-code",
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

func ExampleWithBuilder_Context() {
	err := fmt.Errorf("foo bar baz")
	ctx := context.Background()

	err = errorutil.With(err).
		Context(ctx)

	_, _ = errorutil.LookupContext(err) // ctx, true
}

func ExampleWithBuilder_Detail() {
	err := fmt.Errorf("foo bar baz")

	err = errorutil.With(err).
		Detail("it was the foo or bar or baz")

	_, _ = errorutil.LookupDetail(err) // "it was the foo or bar or baz", true
}

func ExampleWithBuilder_Detailf() {
	err := fmt.Errorf("foo bar baz")

	err = errorutil.With(err).
		Detailf("--> %s <--", "it was the foo or bar or baz")

	_, _ = errorutil.LookupDetail(err) // "--> it was the foo or bar or baz <--", true
}
