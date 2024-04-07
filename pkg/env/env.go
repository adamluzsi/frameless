package env

import (
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
	"os"
	"reflect"
	"strconv"
)

const ErrLoadInvalidData errorkit.Error = "ErrLoadInvalidData"

func Lookup[T any](key string, opts ...LookupOption) (T, bool, error) {
	typ := reflectkit.TypeOf[T]()
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

func TimeLayout(layout string) LookupOption {
	return funcLookupOption(func(options *lookupEnvOptions) {
		options.TimeLayout = &layout
	})
}

func Required() LookupOption {
	return funcLookupOption(func(options *lookupEnvOptions) {
		options.IsRequired = true
	})
}

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

func loadVisitStruct(rStruct reflect.Value) error {
	for i, numField := 0, rStruct.NumField(); i < numField; i++ {
		rStructField := rStruct.Type().Field(i)
		if !rStructField.IsExported() {
			continue
		}

		field := rStruct.Field(i)

		// We don't want to visit struct types which have their own registered parser.
		if field.Kind() == reflect.Struct && !convkit.IsRegistered(field.Interface()) {
			if err := loadVisitStruct(field); err != nil {
				return err
			}
			continue
		}

		osEnvKey, ok := rStructField.Tag.Lookup(envTagKey)
		if !ok {
			continue
		}

		opts, err := getLookupEnvOptions(rStructField.Tag)
		if err != nil {
			return err
		}

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

const envTagKey = "env"

var (
	tagsForDefaultValue = []string{"env-default", "default"}
	tagsForRequired     = []string{"env-required", "env-require", "required", "require"}
	tagsForSeparator    = []string{"env-separator", "separator"}
	tagsForTimeLayout   = []string{"env-time-layout", "layout"}
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
	for _, key := range tagsForTimeLayout {
		value, ok := tag.Lookup(key)
		if ok {
			opts.TimeLayout = &value
			break
		}
	}
	return opts, nil
}

type lookupEnvOptions struct {
	DefaultValue *string
	Separator    *string
	IsRequired   bool
	TimeLayout   *string
	Parser       func(string) (any, error)
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
			err = errMissingEnvironmentVariable(key)
		}
		return reflect.Value{}, false, err
	}
	parseOpts := convkit.Options{
		Separator:  zerokit.Coalesce(pointer.Deref(opts.Separator), ","),
		TimeLayout: pointer.Deref(opts.TimeLayout),
	}
	if json.Valid([]byte(val)) { // then prefer JSON format instead
		parseOpts.Separator = ""
	}
	if opts.Parser != nil {
		parseOpts.ParseFunc = func(data []byte, ptr any) error {
			v, err := opts.Parser(string(data))
			if err != nil {
				return err
			}
			return reflectkit.Link(v, ptr)
		}
	}
	rv, err := convkit.ParseReflect(typ, val, parseOpts)
	if err != nil {
		return reflect.Value{}, false, err
	}
	return rv, true, nil
}

func errMissingEnvironmentVariable(key string) error {
	return fmt.Errorf("missing environment variable: %s", key)
}

func errParsingEnvValue(structField reflect.StructField, err error) error {
	return fmt.Errorf("error parsing the value for %s: %w", structField.Name, err)
}

type ParserFunc[T any] func(envValue string) (T, error)

func ParseWith[T any](parser ParserFunc[T]) LookupOption {
	return funcLookupOption(func(options *lookupEnvOptions) {
		options.Parser = func(ev string) (any, error) {
			return parser(ev)
		}
	})
}

type Set struct {
	lookups []func() error
}

// SetLookup function registers a Lookup within a specified Set.
// When 'Set.Parse' is invoked, all the registered lookups will be executed.
// Unlike the 'Lookup' function, 'SetLookup' doesn't permit missing environment variables without a fallback value
// and will raise it as an issue.
func SetLookup[T any](set *Set, ptr *T, key string, opts ...LookupOption) {
	if set == nil {
		panic("SetLookup requires a non nil Set pointer")
	}
	if ptr == nil {
		panic(fmt.Sprintf("SetLookup requires a non nil %T pointer",
			reflectkit.TypeOf[T]().String()))
	}
	var lookup = func() error {
		value, ok, err := Lookup[T](key, opts...)
		if err != nil {
			return err
		}
		if !ok {
			return errMissingEnvironmentVariable(key)
		}
		*ptr = value
		return nil
	}
	set.lookups = append(set.lookups, lookup)
}

func (es Set) Parse() error {
	var errs []error
	for _, lookup := range es.lookups {
		errs = append(errs, lookup())
	}
	return errorkit.Merge(errs...)
}
