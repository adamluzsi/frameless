package validate

import "context"

// Deprecated: use validate.Error instead
type ValidationError = Error

// SkipValidate option indicates that the function is being used inside a Validate(context.Context) error method.
//
// When this option is set, the Validate function call is skipped to prevent an infinite loop caused by a circular Validate call.
//
// Deprecated: no longer needed to use if you migrated to the new Validatable interface signature.
func SkipValidate(ctx context.Context) context.Context {
	c, _ := ctxConfig.Lookup(ctx)
	c.SkipValidate = true
	return ctxConfig.ContextWith(ctx, c)
}

// Validator
//
// Deprecated: use validate.Validatable
type Validator interface {
	Validate() error
}
