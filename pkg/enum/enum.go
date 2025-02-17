package enum

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"go.llib.dev/frameless/internal/interr"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/synckit"

	"go.llib.dev/frameless/pkg/errorkit"
)

const ErrInvalid errorkit.Error = "The value does not match the enumerator specification"

const ImplementationError errorkit.Error = "ImplementationError"

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

	key := reflectkit.TypeOf[T]()
	registry[key] = choices

	return func() {
		regLock.Lock()
		defer regLock.Unlock()
		delete(registry, key)
	}
}

func Values[T any]() []T {
	regLock.Lock()
	defer regLock.Unlock()
	var out []T
	if vs, ok := registry[reflectkit.TypeOf[T]()]; ok {
		for _, v := range vs {
			out = append(out, v.(T))
		}
	}
	return out
}

func ReflectValues(typ any) []reflect.Value {
	switch typ := typ.(type) {
	case reflect.Type:
		regLock.Lock()
		defer regLock.Unlock()
		var out []reflect.Value
		if vs, ok := registry[typ]; ok {
			for _, v := range vs {
				out = append(out, reflect.ValueOf(v))
			}
		}
		return out

	case reflect.StructField:
		if tag, ok := typ.Tag.Lookup(structTagName); ok {
			enumerators, err := parseTag(typ.Type, tag)
			if err == nil {
				return enumerators
			}
		}
		return ReflectValues(typ.Type)

	default:
		panic(fmt.Sprintf("implementation error, incorrect value type for enum.ReflectValues: %T", typ))
	}
}

func ReflectValuesOfStructField(sf reflect.StructField) ([]reflect.Value, error) {
	if tag, ok := sf.Tag.Lookup(structTagName); ok {
		return parseTag(sf.Type, tag)
	}
	return ReflectValues(sf.Type), nil
}

// Validate will check if the given value is a registered enum member.
func Validate[T any](v T) error {
	return validate(reflectkit.TypeOf[T](v), reflect.ValueOf(v))
}

func ValidateStruct(v any) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Struct {
		return interr.ImplementationError.F("only struct types are supported. (%T)", v)
	}
	for field, value := range reflectkit.OverStruct(rv) {
		if err := ValidateStructField(field, value); err != nil {
			return err
		}
	}
	return nil
}

var enumTag = reflectkit.TagHandler[[]reflect.Value]{
	Name: "enum",
	Parse: func(sf reflect.StructField, tag string) ([]reflect.Value, error) {
		return parseTag(sf.Type, tag)
	},
	Use: func(sf reflect.StructField, field reflect.Value, enumerators []reflect.Value) error {
		if !field.CanInterface() { // ToAccess
			return nil // TODO: maybe implementation error?
		}
		if enumerators == nil {
			enumerators = ReflectValues(sf.Type)
		}
		if len(enumerators) == 0 {
			return nil
		}
		for _, enum := range enumerators {
			if reflectkit.Equal(field, enum) {
				return nil
			}
		}
		return ErrInvalid.F("%#v is not part of the enumerators for %s", field.Interface(), sf.Type.String())
	},
	ForceCache:     true,
	HandleUntagged: true,
}

func ValidateStructField(sf reflect.StructField, field reflect.Value) error {
	{
		enumerators, hasTag, err := valuesForTag(sf)
		if err != nil {
			return ImplementationError.Wrap(err)
		}
		if hasTag {
			if !matchStructField(enumerators, field) {
				return ErrInvalid.F(".%v=%v does not match enumerator specification", sf.Name, field.Interface())
			}
			return nil
		}
	}
	if field.CanInterface() {
		if err := validate(sf.Type, field); err != nil {
			return err
		}
	}
	return nil
}

var tagCache synckit.Map[tagIDStructField, []reflect.Value]

type tagIDStructField struct {
	Name    string
	Type    string
	Tag     reflect.StructTag
	PkgPath string
}

func valuesForTag(sf reflect.StructField) (_ []reflect.Value, _ bool, rErr error) {
	tag, ok := sf.Tag.Lookup(structTagName)
	if !ok {
		return nil, false, nil
	}
	defer errorkit.Recover(&rErr)
	id := tagIDStructField{
		Name:    sf.Name,
		Type:    sf.Type.String(),
		Tag:     sf.Tag,
		PkgPath: sf.PkgPath,
	}
	return tagCache.GetOrInit(id, func() []reflect.Value {
		enumerators, err := parseTag(sf.Type, tag)
		if err != nil {
			panic(err)
		}
		return enumerators
	}), true, nil
}

func matchStructField(enumerators []reflect.Value, rv reflect.Value) bool {
	if len(enumerators) == 0 {
		return true
	}
	switch rv.Type().Kind() {
	case reflect.Slice:
		for i, length := 0, rv.Len(); i < length; i++ {
			if !matchStructField(enumerators, rv.Index(i)) {
				return false
			}
		}
		return true
	default:
		var enums []any
		for _, e := range enumerators {
			enums = append(enums, e.Interface())
		}
		return validateEnumerators(enums, rv.Interface()) == nil
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

// validate
func validate(typ reflect.Type, v reflect.Value) error {
	regLock.RLock()
	defer regLock.RUnlock()

	if typ.Kind() == reflect.Pointer {
		if reflectkit.IsNil(v) {
			return nil
		}
		if !v.CanConvert(typ) {
			panic(fmt.Sprintf("%#v is not compatible with %s type",
				v.Interface(), typ.String()))
		}
		return validate(typ.Elem(), v.Elem())
	}

	enums, ok := registry[typ]
	if !ok {
		return nil
	}

	return validateEnumerators(enums, v.Interface())
}

func validateEnumerators(enums []any, v any) error {
	for _, enum := range enums {
		if reflectkit.Equal(v, enum) {
			return nil
		}
	}
	return fmt.Errorf("%w\nvalue: %#v\nenumerators: %v", ErrInvalid, v, enums)
}
