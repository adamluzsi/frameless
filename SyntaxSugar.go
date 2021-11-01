package frameless

type (
	T  = interface{}
	ID = interface{}
)

// Option is a custom struct type with fields that hold details about the requested option's property
//opts
// For example:
//   r.Create(ctx, &ent, options.Timestamp{V: time.Now()}}
//
type Option = interface{}
