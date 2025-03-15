package enum

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"go.llib.dev/frameless/internal/interr"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/runtimekit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/synckit"
	"go.llib.dev/frameless/port/option"

	"go.llib.dev/frameless/pkg/errorkit"
)

const ErrInvalid errorkit.Error = "The value does not match the enumerator specification"

const ImplementationError errorkit.Error = "ImplementationError"

var (
	registry = make(map[reflect.Type][]any)
	regLock  sync.RWMutex
)

func lookupEnumerators(typ reflect.Type) ([]any, bool) {
	regLock.RLock()
	defer regLock.RUnlock()
	enumerators, ok := registry[typ]
	return enumerators, ok
}

func Register[T any](enums ...T) (unregister func()) {
	regLock.Lock()
	defer regLock.Unlock()

	var choices []any
	for _, e := range enums {
		choices = append(choices, e)
	}

	key := reflectkit.TypeOf[T]()
	if reflectkit.IsBuiltInType(key) {
		panic("Making an enum registration for a built-in type is almost always happens due to a typo, and forbidden.")
	}

	registry[key] = choices

	return func() {
		regLock.Lock()
		defer regLock.Unlock()
		delete(registry, key)
	}
}

func Values[T any]() []T {
	var out []T
	enumerators, ok := lookupEnumerators(reflectkit.TypeOf[T]())
	if !ok {
		return out
	}
	for _, v := range enumerators {
		o, ok := v.(T)
		if ok {
			out = append(out, o)
		}
	}
	return out
}

// ReflectValues will return the enumerables for
// - reflect.Type        -> the type specific values
// - reflect.StructField -> the struct field type specific values
func ReflectValues(reflectTypeOrStructType any) []reflect.Value {
	switch T := reflectTypeOrStructType.(type) {
	case reflect.Type:
		var out []reflect.Value
		if vs, ok := lookupEnumerators(T); ok {
			for _, v := range vs {
				out = append(out, reflect.ValueOf(v))
			}
		}
		return out

	case reflect.StructField:
		if !reflectkit.IsMutableType(T.Type) {
			vs, ok, err := valuesForTag(T)
			if err == nil && ok {
				// to avoid the external modification of the enum list
				return slicekit.Clone(vs)
			}
		}
		if tag, ok := T.Tag.Lookup(enumTagName); ok {
			if reflectkit.IsMutableType(T.Type) {
				enumerators, err := parseTag(T.Type, tag)
				if err == nil {
					return enumerators
				}
			}

		}
		return ReflectValues(T.Type)

	default:
		const format = "implementation error, incorrect value type for enum.ReflectValues.\nprovide either a reflect.StructType or a reflect.Type.\nGot: %T"
		panic(fmt.Sprintf(format, T))
	}
}

// Deprecate: use ReflectValues instead
func ReflectValuesOfStructField(field reflect.StructField) ([]reflect.Value, error) {
	if tag, ok := field.Tag.Lookup(enumTagName); ok {
		return parseTag(field.Type, tag)
	}
	return ReflectValues(field.Type), nil
}

// Validate will check if the given value is a registered enum member.
// It is not a recursive operation, only check the given value.
func Validate[T any](v T) error {
	return ReflectValidate(reflect.ValueOf(v), ReflectType(reflectkit.TypeOf[T](v)))
}

func ValidateStruct(v any) error {
	rv := reflectkit.ToValue(v)
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

func ValidateStructField(field reflect.StructField, value reflect.Value) error {
	{
		enumerators, hasTag, err := valuesForTag(field)
		if err != nil {
			return ImplementationError.Wrap(err)
		}
		if hasTag {
			if !matchStructField(enumerators, value) {
				return ErrInvalid.F(".%v=%v does not match enumerator specification", field.Name, value.Interface())
			}
			return nil
		}
	}
	if value.CanInterface() {
		if err := ReflectValidate(value, ReflectType(field.Type)); err != nil {
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

func valuesForTag(field reflect.StructField) (_ []reflect.Value, _ bool, rErr error) {
	tag, ok := field.Tag.Lookup(enumTagName)
	if !ok {
		return nil, false, nil
	}
	defer errorkit.Recover(&rErr)
	id := tagIDStructField{
		Name:    field.Name,
		Type:    field.Type.String(),
		Tag:     field.Tag,
		PkgPath: field.PkgPath,
	}
	return tagCache.GetOrInit(id, func() []reflect.Value {
		enumerators, err := parseTag(field.Type, tag)
		if err != nil {
			panic(err)
		}
		return enumerators
	}), true, nil
}

const enumTagName = "enum"

// var enumTag = reflectkit.TagHandler[[]reflect.Value]{
// 	Name: "enum",
// 	Parse: func(field reflect.StructField, tagValue string) ([]reflect.Value, error) {
// 		return parseTag(field.Type, tagValue)
// 	},
// 	Use: func(field reflect.StructField, value reflect.Value, enumerators []reflect.Value) error {
// 		if matchStructField(enumerators, value) {
// 			return nil
// 		}
// 		var enumeratorValues []any = slicekit.Map(enumerators, reflect.Value.Interface)
// 		const format = ".%v=%v does not match enumerator specification, accepted values: %#v"
// 		return ErrInvalid.F(format, field.Name, value.Interface(), enumeratorValues)
// 	},
// 	ForceCache:        true,
// 	PanicOnParseError: true,
// }

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

func isSpecialCharacter(char rune) bool {
	if unicode.IsSymbol(char) || unicode.IsPunct(char) {
		return true
	}
	if unicode.IsSpace(char) && char != ' ' {
		return true
	}
	return false
}

func parseTag(rt reflect.Type, raw string) ([]reflect.Value, error) {
	var osMaxBitSupport = runtimekit.ArchBitSize()

	if len(raw) == 0 {
		return nil, nil
	}

	switch rt.Kind() {
	case reflect.Slice:
		return parseTag(rt.Elem(), raw)
	}

	chars := []rune(raw)
	seperatorIndex := len(chars) - 1
	seperator := chars[seperatorIndex]

	var elements []string
	if isSpecialCharacter(seperator) {
		elements = strings.Split(string(chars[:seperatorIndex]), string(seperator))
	} else {
		const commaSeperator = ","
		const spaceSeperator = " "
		switch {
		case 0 < strings.Count(raw, commaSeperator):
			elements = strings.Split(raw, commaSeperator)
		case 0 < strings.Count(raw, spaceSeperator):
			elements = strings.Split(raw, spaceSeperator)
		default:
			return nil, ImplementationError.F("unrecognised enum format for %q", raw)
		}
		// default seperators also apply space trimming for convinence
		elements = slicekit.Map(elements, strings.TrimSpace)
		elements = slicekit.Filter(elements, func(v string) bool { return 0 < len(v) })
	}

	if len(elements) == 0 {
		return nil, nil
	}

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

// ReflectValidate checks whether a given value exists within its predefined set of valid enum options.
// If the given type has no enum constraint, error is never exepcted.
func ReflectValidate(value any, opts ...Option) error {
	c := option.Use(opts)
	v := reflectkit.ToValue(value)
	T := c.CorrelateType(v)

	if !v.IsValid() {
		return nil
	}

	if reflectkit.IsNil(v) &&
		reflectkit.IsNilable(T.Kind()) {
		return nil
	}

	if T.Kind() == reflect.Pointer {
		if reflectkit.IsNil(v) {
			return nil
		}
		if !v.CanConvert(T) {
			panic(fmt.Sprintf("%#v is not compatible with %s type",
				v.Interface(), T.String()))
		}
		return ReflectValidate(v.Elem(), ReflectType(T.Elem()))
	}

	enums, ok := lookupEnumerators(T)
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

// Type allows you to inject the expected enum type for the enum validation.
func Type[T any]() Option {
	return option.Func[config](func(c *config) {
		c.Type = reflectkit.TypeOf[T]()
	})
}

// ReflectType allows you to inject the expected enum type as a reflect.Type
func ReflectType(T reflect.Type) Option {
	return option.Func[config](func(c *config) {
		c.Type = T
	})
}

type Option option.Option[config]

type config struct {
	Type reflect.Type
}

var anyType = reflectkit.TypeOf[any]()

func (c *config) CorrelateType(value reflect.Value) reflect.Type {
	var T = c.Type
	if T == anyType { // we don't consider `any` as a valid enum type
		T = nil
	}
	if T != nil {
		return T
	}
	if value.IsValid() {
		return value.Type()
	}
	return nil
}
