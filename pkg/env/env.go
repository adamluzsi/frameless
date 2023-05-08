package env

import (
	"encoding/json"
	"fmt"
	"github.com/adamluzsi/frameless/pkg/enum"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

const ErrLoadInvalidData errorkit.Error = "ErrLoadInvalidData"

func Load[T any](ptr *T) error {
	if ptr == nil {
		return fmt.Errorf("%w: nil value received", ErrLoadInvalidData)
	}

	rv := reflect.ValueOf(ptr)
	rv = reflectkit.BaseValue(rv)
	if rv.Kind() != reflect.Struct {
		return fmt.Errorf("%w: non-struct type received", ErrLoadInvalidData)
	}
	if err := loadVisitStruct(rv); err != nil {
		return err
	}

	return nil
}

const (
	envTagKey = "env"
)

func loadVisitStruct(rStruct reflect.Value) error {
	for i, numField := 0, rStruct.NumField(); i < numField; i++ {
		field := rStruct.Field(i)
		field = reflectkit.BaseValue(field)

		if field.Kind() == reflect.Struct {
			if err := loadVisitStruct(field); err != nil {
				return err
			}
			continue
		}

		rStructField := rStruct.Type().Field(i)
		osEnvKey, ok := rStructField.Tag.Lookup(envTagKey)
		if !ok {
			continue
		}

		opts, err := getLookupEnvOptions(rStructField.Tag)

		val, ok, err := lookupEnv(field.Type(), osEnvKey, opts)
		if err != nil {
			return errParsingEnvValue(rStructField, err)
		}
		if !ok {
			continue
		}
		field.Set(val)
	}
	return enum.ValidateStruct(rStruct.Interface())
}

var (
	tagsForDefaultValue = []string{"env-default", "default"}
	tagsForRequired     = []string{"env-required", "env-require", "required", "require"}
	tagsForSeparator    = []string{"env-separator", "separator"}
)

func getLookupEnvOptions(tag reflect.StructTag) (lookupEnvOptions, error) {
	var opts lookupEnvOptions
	for _, key := range tagsForDefaultValue {
		value, ok := tag.Lookup(key)
		if ok {
			opts.DefaultValue = &value
			break
		}
	}
	for _, key := range tagsForRequired {
		value, ok := tag.Lookup(key)
		if !ok {
			continue
		}
		isRequired, err := strconv.ParseBool(value)
		if err != nil {
			return opts, err
		}
		opts.IsRequired = isRequired
		break
	}
	for _, key := range tagsForSeparator {
		value, ok := tag.Lookup(key)
		if ok {
			opts.Separator = &value
			break
		}
	}
	return opts, nil
}

type lookupEnvOptions struct {
	DefaultValue *string
	Separator    *string
	IsRequired   bool
}

func lookupEnv(typ reflect.Type, key string, opts lookupEnvOptions) (reflect.Value, bool, error) {
	val, ok := os.LookupEnv(key)
	if !ok && opts.DefaultValue != nil {
		ok = true
		val = *opts.DefaultValue
	}
	if !ok {
		var err error
		if opts.IsRequired {
			err = fmt.Errorf("missing environment variable: %s", key)
		}
		return reflect.Value{}, false, err
	}
	rv, err := parseEnvValue(typ, val, parseEnvValueOptions{
		Separator: opts.Separator,
	})
	if err != nil {
		return reflect.Value{}, false, err
	}
	return rv, true, nil
}

type parseEnvValueOptions struct {
	Separator *string
}

func parseEnvValue(typ reflect.Type, val string, opts parseEnvValueOptions) (reflect.Value, error) {
	if parser, ok := registry[typ]; ok {
		out, err := parser(val)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(out), nil
	}
	if opts.Separator != nil && typ.Kind() != reflect.Slice {
		return reflect.Value{}, fmt.Errorf("separator tag is supplied for non list type: %s", typ.Name())
	}
	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf(val).Convert(typ), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		num, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(num).Convert(typ), nil

	case reflect.Float32, reflect.Float64:
		num, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(num).Convert(typ), nil

	case reflect.Bool:
		bl, err := strconv.ParseBool(val)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(bl).Convert(typ), nil

	case reflect.Slice:
		rv := reflect.MakeSlice(typ, 0, 0)
		if data := []byte(val); json.Valid(data) {
			return parseJSONEnvValue(typ, rv, data)
		}
		var separator = ","
		if opts.Separator != nil {
			separator = *opts.Separator
		}
		opts := opts
		opts.Separator = nil
		for _, elem := range strings.Split(val, separator) {
			re, err := parseEnvValue(typ.Elem(), elem, opts)
			if err != nil {
				return reflect.Value{}, err
			}
			rv = reflect.Append(rv, re)
		}
		return rv, nil

	default:
		return reflect.Value{}, fmt.Errorf("unknown type: %s", typ.String())
	}
}

func parseJSONEnvValue(typ reflect.Type, rv reflect.Value, data []byte) (reflect.Value, error) {
	ptr := reflect.New(typ)
	ptr.Elem().Set(rv)
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

func errParsingEnvValue(structField reflect.StructField, err error) error {
	return fmt.Errorf("error parsing the value for %s: %w", structField.Name, err)
}

var registry = map[reflect.Type]func(string) (any, error){}

func RegisterParser[T any](parser func(envValue string) (T, error)) struct{} {
	registry[reflect.TypeOf(*new(T))] = func(raw string) (any, error) {
		val, err := parser(raw)
		if err != nil {
			return nil, err
		}
		return val, nil
	}
	return struct{}{}
}

var _ = RegisterParser(func(envValue string) (time.Duration, error) {
	return time.ParseDuration(envValue)
})

func Lookup[T any](key string, opts ...LookupOption) (T, bool, error) {
	typ := reflect.TypeOf(*new(T))
	var conf lookupEnvOptions
	for _, opt := range opts {
		opt.configure(&conf)
	}
	val, ok, err := lookupEnv(typ, key, conf)
	if err != nil || !ok {
		return *new(T), ok, err
	}
	return val.Interface().(T), true, nil
}

type LookupOption interface{ configure(*lookupEnvOptions) }

type funcLookupOption func(*lookupEnvOptions)

func (fn funcLookupOption) configure(options *lookupEnvOptions) { fn(options) }

func ListSeparator[SEP rune | string](sep SEP) LookupOption {
	return funcLookupOption(func(options *lookupEnvOptions) {
		s := string(sep)
		options.Separator = &s
	})
}

func DefaultValue(val string) LookupOption {
	return funcLookupOption(func(options *lookupEnvOptions) {
		options.DefaultValue = &val
	})
}

func Required() LookupOption {
	return funcLookupOption(func(options *lookupEnvOptions) {
		options.IsRequired = true
	})
}
