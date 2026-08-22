// Package comp provides primitives for comparing values.
//
// The package exposes Equal, a single equality predicate that
// dispatches to the most specific comparison mechanism the value type
// provides. This allows downstream packages to ask "are these two
// values semantically equal?" without re-implementing the dispatch
// against the predicate framework each time.
package comp

import (
	"bytes"
	"cmp"
	"math"

	"go.llib.dev/frameless/pkg/internal/refeq"
	"go.llib.dev/frameless/port/predicate"
)

func Equal[T any](a, b T, config ...EqualConfig) (equality bool) {
	c := mergeEqualConfig(config)
	switch a := any(a).(type) {
	case predicate.Equatable[T]:
		return a.Equal(b)
	case predicate.Comparable[T]:
		return a.Compare(b) == 0
	case predicate.ComparableShort[T]:
		return a.Cmp(b) == 0
	case []byte:
		// When T is an interface type, a and b can hold different dynamic
		// types. Only take the fast path when both sides actually match it,
		// otherwise fall through to the general comparison below.
		if b, ok := any(b).([]byte); ok {
			return bytesEqual(a, b)
		}
	case float64:
		if b, ok := any(b).(float64); ok {
			if a == b {
				return true
			}
			if c.NaN && math.IsNaN(a) && math.IsNaN(b) {
				return true
			}
			return false
		}
	}
	defer fallbackReflectEqual(&equality, a, b)
	return any(a) == any(b)
}

func mergeEqualConfig(configs []EqualConfig) EqualConfig {
	if len(configs) == 0 {
		return EqualConfig{}
	}
	if len(configs) == 1 {
		return configs[0]
	}
	var c EqualConfig
	for _, o := range configs {
		c.NaN = cmp.Or(c.NaN, o.NaN)
	}
	return c
}

type EqualConfig struct {
	// NaN enables NaN equality checking
	NaN bool
}

func fallbackReflectEqual[T any](equality *bool, a, b T) {
	if recover() == nil {
		return
	}
	*equality = refeq.EqualT[T](a, b)
}

func bytesEqual(a, b []byte) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return bytes.Equal(a, b)
}
