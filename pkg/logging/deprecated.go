package logging

import "context"

// RegisterFieldType is an alias for RegisterType
//
// Deprecated: use logging.RegisterType
func RegisterFieldType[T any](mapping func(T) Detail) func() {
	return RegisterType[T](func(ctx context.Context, v T) Detail {
		return mapping(v)
	})
}
