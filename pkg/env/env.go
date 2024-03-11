package env

import (
	"encoding/json"
	"fmt"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
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

		// We don't want to visit struct types which has their own registered parser.
		if _, ok := registry[field.Type()]; field.Kind() == reflect.Struct && !ok {
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
	rv, err := parseEnvValue(typ, val, parseEnvValueOptions{
		Separator:  opts.Separator,
		TimeLayout: opts.TimeLayout,
	})
	if err != nil {
		return reflect.Value{}, false, err
	}
	return rv, true, nil
}

func errMissingEnvironmentVariable(key string) error {
	return fmt.Errorf("missing environment variable: %s", key)
}

type parseEnvValueOptions struct {
	Separator  *string
	TimeLayout *string
}

var timeType = reflect.TypeOf(time.Time{})

// TODO: should we use it as default as it is the most common, or keep forcing the user to supply the time format?
const iso8601TimeLayout = "2006-01-02T15:04:05Z0700"

const missingTimeLayoutErrMsg = `Missing time layout!
Please use the "layout" or "env-time-layout" tag to supply it in a struct field
or use the TimeLayout Lookup option.`

func parseEnvValue(typ reflect.Type, val string, opts parseEnvValueOptions) (reflect.Value, error) {
	if parser, ok := registry[typ]; ok && parser != nil {
		out, err := parser(val)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(out), nil
	}
	if opts.Separator != nil && typ.Kind() != reflect.Slice {
		return reflect.Value{}, fmt.Errorf("separator tag is supplied for non list type: %s", typ.Name())
	}
	if typ == timeType {
		if opts.TimeLayout == nil {
			return reflect.Value{}, fmt.Errorf(missingTimeLayoutErrMsg)
		}
		date, err := time.Parse(*opts.TimeLayout, val)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(date), nil
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

	case reflect.Map:
		ptr := reflect.New(typ)
		m := reflect.MakeMap(typ)
		ptr.Elem().Set(m)
		if err := json.Unmarshal([]byte(val), ptr.Interface()); err != nil {
			return reflect.Value{}, err
		}
		return ptr.Elem(), nil

	case reflect.Ptr:
		rv, err := parseEnvValue(typ.Elem(), val, opts)
		if err != nil {
			return reflect.Value{}, err
		}
		ptr := reflect.New(typ.Elem())
		ptr.Elem().Set(rv)
		return ptr, nil

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
	var fn func(raw string) (any, error)
	if parser != nil {
		fn = func(raw string) (any, error) {
			val, err := parser(raw)
			if err != nil {
				return nil, err
			}
			return val, nil
		}
	}
	registry[reflect.TypeOf(*new(T))] = fn
	return struct{}{}
}

var _ = RegisterParser(func(envValue string) (time.Duration, error) {
	return time.ParseDuration(envValue)
})

// should never be called as it is not possible to parse time from this scope,
// because we don't have access to the layout defined in the tag
var _ = RegisterParser[time.Time](nil)

var _ = RegisterParser[url.URL](func(envValue string) (url.URL, error) {
	u, err := url.Parse(envValue)
	if err == nil {
		return *u, nil
	}
	u, err = url.ParseRequestURI(envValue)
	if err == nil {
		return *u, nil
	}
	return url.URL{}, fmt.Errorf("invalid url: %s", envValue)
})

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
