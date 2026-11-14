// Package validateint holds the internals of the validate package, which other frameless packages rely on.
package validateint

import "context"

type ctxKeySkipValidatable struct{}

// SkipValidatable returns a context, with which validate checks a value
// without calling the Validate method of the validate.Validatable values it finds,
// because the caller validates them on its own, and they must be validated only once.
//
// The context must not reach a Validate method,
// as the validate calls made inside it would skip the Validatable values as well.
func SkipValidatable(ctx context.Context) context.Context {
	return context.WithValue(ctx, ctxKeySkipValidatable{}, true)
}

// IsValidatableSkipped tells whether the context is one from SkipValidatable.
func IsValidatableSkipped(ctx context.Context) bool {
	skip, _ := ctx.Value(ctxKeySkipValidatable{}).(bool)
	return skip
}
