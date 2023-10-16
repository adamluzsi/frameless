package enum

import (
	"fmt"
	"go.llib.dev/frameless/pkg/reflectkit"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"go.llib.dev/frameless/pkg/errorkit"
)

var ErrInvalid = errorkit.UserError{
	ID:      "enum-invalid-value",
	Message: "The value does not match the enumerator specification",
}

func ValidateStruct(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("only struct types are supported. (%T)", v)
	}

	rt := rv.Type()
	for i, fnum := 0, rt.NumField(); i < fnum; i++ {
		field := rt.Field(i)
		value := rv.Field(i)

		if tag, ok := field.Tag.Lookup(structTagName); ok {
			enumerators, err := parseTag(field.Type, tag)
			if err != nil {
				return err
			}

			if !match(value, enumerators) {
				return ErrInvalid.With().Detailf("%v does not match enumerator specification: %s", v, tag)
			}
		}

		if value.CanInterface() {
			if err := validate(value.Interface()); err != nil {
				return err
			}
		}
	}
	return nil
}

func match(rv reflect.Value, enumerators []reflect.Value) bool {
	if len(enumerators) == 0 {
		return true
	}
	switch rv.Type().Kind() {
	case reflect.Slice:
		for i, length := 0, rv.Len(); i < length; i++ {
			if !match(rv.Index(i), enumerators) {
				return false
			}
		}
		return true
	default:
		v := rv.Interface()
		for _, enumerator := range enumerators {
			if enumerator.Interface() == v {
				return true
			}
		}
		return false
	}
}

const structTagName = "enum"

func parseTag(rt reflect.Type, raw string) ([]reflect.Value, error) {
	const osMaxBitSupport = 64

	if len(raw) == 0 {
		return nil, nil
	}

	switch rt.Kind() {
	case reflect.Slice:
		return parseTag(rt.Elem(), raw)
	}

	chars := []rune(raw)
	sepCharPos := len(chars) - 1
	separator := string(chars[sepCharPos:])
	elements := strings.Split(string(chars[:sepCharPos]), separator)

	switch rt.Kind() {
	case reflect.String:
		return mapVS(elements, rt, func(s string) (reflect.Value, error) {
			return reflect.ValueOf(s), nil
		})

	case reflect.Bool:
		return mapVS(elements, rt, func(s string) (reflect.Value, error) {
			b, err := strconv.ParseBool(s)
			return reflect.ValueOf(b), err
		})

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return mapVS(elements, rt, func(s string) (reflect.Value, error) {
			var bitSize = osMaxBitSupport
			switch rt.Kind() {
			case reflect.Int:
				bitSize = osMaxBitSupport
			case reflect.Int8:
				bitSize = 8
			case reflect.Int16:
				bitSize = 16
			case reflect.Int32:
				bitSize = 32
			case reflect.Int64:
				bitSize = 64
			}
			n, err := strconv.ParseInt(s, 10, bitSize)
			return reflect.ValueOf(n), err
		})

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return mapVS(elements, rt, func(s string) (reflect.Value, error) {
			var bitSize = osMaxBitSupport
			switch rt.Kind() {
			case reflect.Uint:
				bitSize = osMaxBitSupport
			case reflect.Uint8:
				bitSize = 8
			case reflect.Uint16:
				bitSize = 16
			case reflect.Uint32:
				bitSize = 32
			case reflect.Uint64:
				bitSize = 64
			}
			b, err := strconv.ParseUint(s, 10, bitSize)
			return reflect.ValueOf(b), err
		})

	case reflect.Float32, reflect.Float64:
		return mapVS(elements, rt, func(s string) (reflect.Value, error) {
			var bitSize = osMaxBitSupport
			switch rt.Kind() {
			case reflect.Float32:
				bitSize = 32
			case reflect.Float64:
				bitSize = 64
			}
			float, err := strconv.ParseFloat(s, bitSize)
			return reflect.ValueOf(float), err
		})

	default:
		return nil, fmt.Errorf("enum is not supported for %T", rt.String())
	}
}

func mapVS(vs []string, rt reflect.Type, transform func(string) (reflect.Value, error)) ([]reflect.Value, error) {
	var out []reflect.Value
	for _, v := range vs {
		value, err := transform(v)
		if err != nil {
			return nil, err
		}
		if !value.CanConvert(rt) {
			return nil, fmt.Errorf("%T is not converatble to %T", value.Type().String(), rt.String())
		}
		value = value.Convert(rt)
		out = append(out, value)
	}
	return out, nil
}

var (
	registry = make(map[reflect.Type][]any)
	regLock  sync.RWMutex
)

func Register[T any](enums ...T) (unregister func()) {
	regLock.Lock()
	defer regLock.Unlock()

	var choices []any
	for _, e := range enums {
		choices = append(choices, e)
	}

	key := reflect.TypeOf((*T)(nil)).Elem()
	registry[key] = choices

	return func() {
		regLock.Lock()
		defer regLock.Unlock()
		delete(registry, key)
	}
}

// validate
// TODO: make it recursive and then export it
func validate(v any) error {
	regLock.RLock()
	defer regLock.RUnlock()
	key := reflect.TypeOf(v)
	enums, ok := registry[key]
	if !ok {
		return nil
	}
	var match bool
	for _, enum := range enums {
		if reflectkit.Equal(v, enum) {
			match = true
			break
		}
	}
	if !match {
		return ErrInvalid
	}
	return nil
}
