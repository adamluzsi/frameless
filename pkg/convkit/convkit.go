package convkit

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/datastruct"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/zerokit"
)

func Parse[T any, Raw encoded](raw Raw, opts ...Option) (T, error) {
	var (
		typ     = reflectkit.TypeOf[T]()
		val     = string(raw)
		options = toOptions(opts)
	)
	rv, err := parse(typ, val, options)
	if err != nil {
		return *new(T), fmt.Errorf("convkit.Parse failed: %w", err)
	}
	out, ok := rv.Interface().(T)
	if !ok {
		return *new(T), fmt.Errorf("error, incorrect return value made during parsing")
	}
	return out, nil
}

func ParseReflect[Raw encoded](typ reflect.Type, raw Raw, opts ...Option) (reflect.Value, error) {
	var (
		val     = string(raw)
		options = toOptions(opts)
	)
	return parse(typ, val, options)
}

type encoded interface{ ~string | []byte }

type Option interface{ configure(*Options) }

type Options struct {
	// Separator is the separator character which will be used to detect list elements.
	Separator string
	// TimeLayout is the time layout format which will be used to parse time values.
	TimeLayout string
	// ParseFunc is used to Parse the input data. It follows the signature of the json.Unmarshal function.
	ParseFunc func(data []byte, ptr any) error
}

func toOptions(opts []Option) Options {
	var options Options
	for _, opt := range opts {
		opt.configure(&options)
	}
	return options
}

func (o Options) configure(options *Options) { options.Merge(o) }

func (o *Options) Merge(oth Options) {
	o.Separator = zerokit.Coalesce(oth.Separator, o.Separator)
	o.TimeLayout = zerokit.Coalesce(oth.TimeLayout, o.TimeLayout)
	o.ParseFunc = zerokit.Coalesce(oth.ParseFunc, o.ParseFunc)
}

var (
	typeTime   = reflectkit.TypeOf[time.Time]()
	typeString = reflectkit.TypeOf[string]()
	typeInt    = reflectkit.TypeOf[int]()
	typeUint64 = reflectkit.TypeOf[uint64]()
)

const missingTimeLayoutErrMsg = `missing TimeLayout ParseOption`

func parse(typ reflect.Type, val string, opts Options) (reflect.Value, error) {
	if opts.ParseFunc != nil {
		var ptr = reflect.New(typ)
		err := opts.ParseFunc([]byte(val), ptr.Interface())
		if err != nil {
			return reflect.Value{}, fmt.Errorf("ParseWith func error: %w", err)
		}
		return ptr.Elem(), nil
	}
	if rec, ok := registry[typ]; ok && rec.CanParse() {
		var ptr = reflect.New(typ)
		err := rec.Parse(val, ptr.Interface())
		if err != nil {
			return reflect.Value{}, err
		}
		return ptr.Elem(), nil
	}
	if typ == typeTime {
		if opts.TimeLayout == "" {
			return reflect.Value{}, fmt.Errorf(missingTimeLayoutErrMsg)
		}
		date, err := time.Parse(opts.TimeLayout, val)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(date), nil
	}
	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf(val).Convert(typ), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		num, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(num).Convert(typ), nil

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
		if zerokit.IsZero(opts.Separator) {
			if data := []byte(val); json.Valid(data) {
				return parseJSONEnvValue(typ, rv, data)
			}
			return reflect.Value{}, fmt.Errorf("the Separator option is not configured for %s", typ.String())
		}
		for _, elem := range split(val, opts.Separator) {
			re, err := parse(typ.Elem(), elem, opts)
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

	case reflect.Pointer:
		rv, err := parse(typ.Elem(), val, opts)
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

func split(s string, sep string) []string {
	var (
		out []string
		cur string
	)
	var push = func() {
		cur = strings.TrimSuffix(cur, sep)
		out = append(out, cur)
		cur = ""
	}
	for _, r := range s { // UTF-8 compliant
		cur += string(r)
		if strings.HasSuffix(cur, sep) {
			escaped := "\\" + sep
			if strings.HasSuffix(cur, escaped) {
				cur = strings.TrimSuffix(cur, escaped) + sep
			} else {
				push()
			}
		}
	}
	if 0 < len(cur) {
		push()
	}
	return out
}

func parseJSONEnvValue(typ reflect.Type, rv reflect.Value, data []byte) (reflect.Value, error) {
	ptr := reflect.New(typ)
	ptr.Elem().Set(rv)
	if err := json.Unmarshal(data, ptr.Interface()); err != nil {
		return reflect.Value{}, err
	}
	return ptr.Elem(), nil
}

var registry = datastruct.Map[reflect.Type, registryRecord]{}

type registryRecord interface {
	Parse(data string, ptr any) error
	Format(v any) (string, error)
	CanParse() bool
	CanFormat() bool
}

type regrec[T any] struct {
	ParseFunc  func(data string) (T, error)
	FormatFunc func(T) (string, error)
}

func (r regrec[T]) Parse(data string, ptr any) error {
	if r.ParseFunc == nil {
		return fmt.Errorf("no parser registered for %s",
			reflectkit.TypeOf[T]().String())
	}
	out, err := r.ParseFunc(data)
	if err != nil {
		return err
	}
	return pointer.Link[T](out, ptr)
}

func (r regrec[T]) Format(v any) (string, error) {
	if r.FormatFunc == nil {
		return "", fmt.Errorf("no formatter registered for %s",
			reflectkit.TypeOf[T]().String())
	}
	val, ok := v.(T)
	if !ok {
		return "", fmt.Errorf("incorrect type received. Expected %s, but got %T",
			reflectkit.TypeOf[T](), v)
	}
	return r.FormatFunc(val)
}

func (r regrec[T]) CanParse() bool  { return r.ParseFunc != nil }
func (r regrec[T]) CanFormat() bool { return r.FormatFunc != nil }

type parseFunc[T any] func(data string) (T, error)
type formatFunc[T any] func(T) (string, error)

func IsRegistered[T any](i ...T) bool {
	typ := reflectkit.TypeOf[T](i...)
	typ, _ = reflectkit.DerefType(typ)
	if typ == typeTime {
		return true
	}
	_, ok := registry.Lookup(typ)
	return ok
}

func Register[T any](
	parser parseFunc[T],
	formatter formatFunc[T],
) func() {
	var (
		typ = reflectkit.TypeOf[T]()
		rec = regrec[T]{}
	)
	if parser != nil {
		rec.ParseFunc = func(data string) (T, error) {
			return parser(data)
		}
	}
	if formatter != nil {
		rec.FormatFunc = func(v T) (string, error) {
			out, err := formatter(v)
			return string(out), err
		}
	}
	return datastruct.MapAdd[reflect.Type, registryRecord](registry, typ, rec)
}

func ParseWith[T any](parser parseFunc[T]) Option {
	return Options{
		ParseFunc: func(data []byte, ptr any) error {
			out, err := parser(string(data))
			if err != nil {
				return err
			}
			*ptr.(*T) = out
			return nil
		},
	}
}

var _ = Register(func(envValue string) (time.Duration, error) {
	return time.ParseDuration(envValue)
}, func(duration time.Duration) (string, error) {
	return duration.String(), nil
})

//// should never be called as it is not possible to parse time from this scope,
//// because we don't have access to the layout defined in the tag
//var _ = Register[time.Time, string](nil, nil)

var _ = Register[url.URL](func(data string) (url.URL, error) {
	u, err := url.Parse(data)
	if err == nil {
		return *u, nil
	}
	u, err = url.ParseRequestURI(data)
	if err == nil {
		return *u, nil
	}
	return url.URL{}, fmt.Errorf("invalid url: %s", data)
}, func(u url.URL) (string, error) {
	return u.String(), nil
})

// Format allows you to format a value into a string format
func Format[T any](v T, opts ...Option) (string, error) {
	var (
		value   = reflect.ValueOf(v)
		options = toOptions(opts)
	)
	return format(value, options)
}

func format(val reflect.Value, opts Options) (string, error) {
	val = reflectkit.BaseValue(val)
	if rec, ok := registry[val.Type()]; ok && rec.CanFormat() {
		return rec.Format(val.Interface())
	}
	if val.Type() == typeTime {
		if opts.TimeLayout == "" {
			return "", fmt.Errorf("%s", missingTimeLayoutErrMsg)
		}
		return val.Interface().(time.Time).Format(opts.TimeLayout), nil
	}
	switch val.Kind() {
	case reflect.String:
		return val.Convert(typeString).String(), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(val.Convert(typeUint64).Interface().(uint64), 10), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.Itoa(val.Convert(typeInt).Interface().(int)), nil

	case reflect.Float32, reflect.Float64:
		// In this example, we call strconv.FormatFloat with the following arguments:
		//   val.Float(): the floating-point number to convert (of type float64)
		//   'f': the format to use for the conversion (in this case, we want a decimal point and digits after it)
		//   -1: the precision to use (the number of digits after the decimal point).
		//       We specify -1 to use the default precision for the format.
		//   64: the bit size of the floating-point number (float64 in this case)
		return strconv.FormatFloat(val.Float(), 'f', -1, 64), nil

	case reflect.Bool:
		return strconv.FormatBool(val.Bool()), nil

	case reflect.Slice:
		if zerokit.IsZero(opts.Separator) {
			data, err := json.Marshal(val.Interface())
			return string(data), err
		}

		list := make([]string, 0, val.Len())
		for i, length := 0, val.Len(); i < length; i++ {
			out, err := format(val.Index(i), opts)
			if err != nil {
				return "", fmt.Errorf("error while formatting eleement at index: %d\n%#v", i,
					val.Index(i).Interface())
			}
			out = strings.Replace(out, opts.Separator, `\`+opts.Separator, -1)
			list = append(list, out)
		}

		return strings.Join(list, opts.Separator), nil

	case reflect.Map:
		data, err := json.Marshal(val)
		return string(data), err

	case reflect.Pointer:
		return format(val.Elem(), opts)

	default:
		return "", fmt.Errorf("unknown type: %s", val.Type().String())
	}
}

var (
	matchInt   = regexp.MustCompile(`^\d+$`)
	matchFloat = regexp.MustCompile(`^\d+\.\d+$`)
)

func DuckParse[Raw encoded](raw Raw, opts ...Option) (any, error) {
	options := toOptions(opts)
	if matchInt.Match([]byte(raw)) {
		return Parse[int](raw, options)
	}
	if matchFloat.Match([]byte(raw)) {
		return Parse[float64](raw, options)
	}
	if out, err := strconv.ParseBool(string(raw)); err == nil {
		return out, nil
	}
	if !zerokit.IsZero(options.TimeLayout) {
		return Parse[time.Time](raw, options)
	}
	if !zerokit.IsZero(options.Separator) {
		return duckParseList(string(raw), options)
	}
	if data := []byte(raw); json.Valid(data) { // enable the registration of serializers
		var (
			out any
			err = json.Unmarshal(data, &out)
		)
		return out, err
	}
	if dur, err := time.ParseDuration(string(raw)); err == nil {
		return dur, nil
	}
	return string(raw), nil
}

func duckParseList(raw string, options Options) (any, error) {
	var (
		values   []any
		types    = map[reflect.Type]struct{}{}
		elements = split(raw, options.Separator)
		subopt   = options
	)
	// To prevent recursive DuckParse, we need to remove the separator option,
	// otherwise, it would assume that each element is also a list.
	subopt.Separator = ""
	for i, elem := range elements {
		out, err := DuckParse(elem, subopt)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %d. element in %q: %w", i, raw, err)
		}
		values = append(values, out)
		types[reflect.TypeOf(out)] = struct{}{}
	}
	if len(types) == 1 {
		var typ reflect.Type
		for gotType := range types {
			typ = gotType
		}
		list := reflect.MakeSlice(reflect.SliceOf(typ), 0, len(values))
		for _, val := range values {
			list = reflect.Append(list, reflect.ValueOf(val))
		}
		return list.Interface(), nil
	}
	return values, nil
}
