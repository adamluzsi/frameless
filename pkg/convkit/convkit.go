package convkit

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.llib.dev/frameless/internal/errorkitlite"
	"go.llib.dev/frameless/pkg/bytekit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/codec"
	"go.llib.dev/frameless/port/option"
)

func Unmarshal[T any](data []byte, p *T, opts ...Option) error {
	var (
		typ     = reflectkit.TypeOf[T]()
		options = option.ToConfig(opts)
		rv, err = parse(typ, data, options)
	)
	if err != nil {
		return fmt.Errorf("convkit.Unmarshal failed: %w", err)
	}
	var out, ok = rv.Interface().(T)
	if !ok {
		return fmt.Errorf("error, incorrect return value made during parsing")
	}
	*p = out
	return nil
}

func UnmarshalReflect(typ reflect.Type, data []byte, ptr reflect.Value, opts ...Option) error {
	if ptr.Kind() != reflect.Pointer {
		return fmt.Errorf("convkit.UnmarshalReflect called with non-pointer type: %s", ptr.Type().String())
	}
	if typ == emptyInterfaceType {
		typ = ptr.Type().Elem()
	}
	var rv, err = parse(typ, data, option.ToConfig(opts))
	if err != nil {
		return fmt.Errorf("convkit.UnmarshalReflect failed: %w", err)
	}
	if rvt := rv.Type(); rvt != typ {
		if typ.Kind() == reflect.Interface && rvt.Implements(typ) {
			iface := reflect.New(typ).Elem()
			iface.Set(rv)
			ptr.Elem().Set(iface)
			return nil
		}
		return fmt.Errorf("error, incorrect return value type")
	}
	ptr.Elem().Set(rv)
	return nil
}

type encoded interface{ ~string | ~[]byte }

func Parse[T any, Raw encoded](raw Raw, opts ...Option) (T, error) {
	var (
		typ     = reflectkit.TypeOf[T]()
		val     = []byte(raw)
		options = option.ToConfig(opts)
		rv, err = parse(typ, val, options)
	)
	if err != nil {
		var zero T
		return zero, fmt.Errorf("convkit.Parse failed: %w", err)
	}
	var out, ok = rv.Interface().(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("error, incorrect return value made during parsing")
	}
	return out, nil
}

func ParseReflect[Raw encoded](typ reflect.Type, raw Raw, opts ...Option) (reflect.Value, error) {
	var (
		val     = []byte(raw)
		options = option.ToConfig(opts)
	)
	return parse(typ, val, options)
}

type Option option.Option[Options]

type Options struct {
	// Separator is the separator character which will be used to detect list elements.
	Separator string
	// TimeLayout is the time layout format which will be used to parse time values.
	TimeLayout string
	// ParseFunc is used to Parse the input data. It follows the signature of the json.Unmarshal function.
	ParseFunc func(data []byte, ptr any) error
}

func (o Options) Configure(options *Options) { options.Merge(o) }

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

const errMissingTimeLayout errorkitlite.Error = `missing TimeLayout ParseOption`

var emptyInterfaceType = reflectkit.TypeOf[any]()

func parse(typ reflect.Type, val []byte, opts Options) (reflect.Value, error) {
	if typ == emptyInterfaceType {
		out, err := DuckParse(val)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(out), nil
	}
	if opts.ParseFunc != nil {
		var ptr = reflect.New(typ)
		err := opts.ParseFunc([]byte(val), ptr.Interface())
		if err != nil {
			return reflect.Value{}, fmt.Errorf("ParseWith func error: %w", err)
		}
		return ptr.Elem(), nil
	}
	if rec, ok := registry[typ]; ok {
		var ptr = reflect.New(typ)
		err := rec.Unmarshal(val, ptr.Interface())
		if err != nil {
			return reflect.Value{}, err
		}
		return ptr.Elem(), nil
	}
	if typ == typeTime {
		if opts.TimeLayout == "" {
			return reflect.Value{}, errMissingTimeLayout
		}
		date, err := time.Parse(opts.TimeLayout, string(val))
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(date), nil
	}
	if got, err, ok := textUnmarshal(typ, []byte(val)); ok {
		return got, err
	}
	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf(val).Convert(typ), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		num, err := strconv.ParseUint(string(val), 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(num).Convert(typ), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		num, err := strconv.ParseInt(string(val), 10, 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(num).Convert(typ), nil

	case reflect.Float32, reflect.Float64:
		num, err := strconv.ParseFloat(string(val), 64)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(num).Convert(typ), nil

	case reflect.Bool:
		bl, err := strconv.ParseBool(string(val))
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

func split(s []byte, sep string) [][]byte {
	var (
		out [][]byte
		cur string
	)
	var push = func() {
		cur = strings.TrimSuffix(cur, sep)
		out = append(out, []byte(cur))
		cur = ""
	}
	for r := range bytekit.IterUTF8(s) { // UTF-8 compliant
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

var registry = map[reflect.Type]registryRecord{}

type registryRecord interface {
	codec.Codec
}

type regrec[T any] struct {
	TextCodec TextCodec[T]
}

func (r regrec[T]) Marshal(v any) ([]byte, error) {
	val, ok := v.(T)
	if !ok {
		return nil, fmt.Errorf("incorrect type received. Expected %s, but got %T",
			reflectkit.TypeOf[T](), v)
	}
	return r.TextCodec.Marshal(val)
}

func (r regrec[T]) Unmarshal(data []byte, ptr any) error {
	p, ok := ptr.(*T)
	if !ok {
		return fmt.Errorf("type mismatch, expected %T but got %T", (*T)(nil), ptr)
	}
	return r.TextCodec.Unmarshal(data, p)
}

type MarshalFunc[T any] func(T) ([]byte, error)
type UnmarshalFunc[T any] func(data []byte, p *T) error

func IsRegistered[T any](i ...T) bool {
	typ := reflectkit.TypeOf[T](i...)
	typ, _ = reflectkit.DerefType(typ)
	if typ == typeTime {
		return true
	}
	_, ok := registry[typ]
	return ok
}

type TextCodec[T any] interface {
	Marshal(v T) ([]byte, error)
	Unmarshal(data []byte, p *T) error
}

func Register[T any](c TextCodec[T]) func() {
	var (
		typ = reflectkit.TypeOf[T]()
		rec = regrec[T]{TextCodec: c}
	)
	return registerAdd(typ, rec)
}

func registerAdd(k reflect.Type, v registryRecord) func() {
	og, ok := registry[k]
	registry[k] = v
	return func() {
		if ok {
			registry[k] = og
		} else {
			delete(registry, k)
		}
	}
}

func UnmarshalWith[T any](parser UnmarshalFunc[T]) Option {
	return Options{
		ParseFunc: func(data []byte, ptr any) error {
			return parser(data, ptr.(*T))
		},
	}
}

var _ = Register[time.Duration](timeDuractionTextCodec{})

type timeDuractionTextCodec struct{}

func (timeDuractionTextCodec) Marshal(d time.Duration) ([]byte, error) {
	return []byte(d.String()), nil
}

func (timeDuractionTextCodec) Unmarshal(data []byte, p *time.Duration) error {
	d, err := time.ParseDuration(string(data))
	if err != nil {
		return err
	}
	*p = d
	return nil
}

//// should never be called as it is not possible to parse time from this scope,
//// because we don't have access to the layout defined in the tag
//var _ = Register[time.Time, string](nil, nil)

var _ = Register[url.URL](urlURLTextCodec{})

type urlURLTextCodec struct{}

func (urlURLTextCodec) Marshal(u url.URL) ([]byte, error) {
	return []byte(u.String()), nil
}

func (urlURLTextCodec) Unmarshal(data []byte, p *url.URL) error {
	raw := string(data)
	u, err := url.Parse(raw)
	if err == nil {
		*p = *u
		return nil
	}
	u, err = url.ParseRequestURI(raw)
	if err == nil {
		*p = *u
		return nil
	}
	return fmt.Errorf("invalid url: %s", raw)
}

// Format allows you to format a value into a string format
func Format[T any](v T, opts ...Option) (string, error) {
	data, err := MarshalReflect(reflect.ValueOf(v), option.ToConfig(opts))
	return string(data), err
}

// FormatReflect allows you to Format a value into a string, passed as a reflect.Value.
func FormatReflect(v reflect.Value, opts ...Option) (string, error) {
	data, err := MarshalReflect(v, option.ToConfig(opts))
	return string(data), err
}

func Marshal[T any](v T, opts ...Option) ([]byte, error) {
	return MarshalReflect(reflect.ValueOf(v), option.ToConfig(opts))
}

func MarshalReflect(val reflect.Value, opts ...Option) ([]byte, error) {
	options := option.ToConfig(opts)
	if enc, err, ok := pformat(val, options); ok {
		return enc, err
	}
	if base := reflectkit.BaseValue(val); base.IsValid() {
		if enc, err, ok := pformat(base, options); ok {
			return enc, err
		}
	}
	return nil, fmt.Errorf("unknown type: %s", val.Type().String())
}

func pformat(val reflect.Value, opts Options) ([]byte, error, bool) {
	if !val.IsValid() {
		return nil, nil, false
	}
	if rec, ok := registry[val.Type()]; ok {
		text, err := rec.Marshal(val.Interface())
		return text, err, true
	}
	if val.Type() == typeTime {
		if opts.TimeLayout == "" {
			return nil, errMissingTimeLayout, true
		}
		return []byte(val.Interface().(time.Time).Format(opts.TimeLayout)), nil, true
	}
	if text, err, ok := textMarshal(val); ok {
		return text, err, true
	}
	switch val.Kind() {
	case reflect.String:
		return []byte(val.Convert(typeString).String()), nil, true

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []byte(strconv.FormatUint(val.Convert(typeUint64).Interface().(uint64), 10)), nil, true

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return []byte(strconv.Itoa(val.Convert(typeInt).Interface().(int))), nil, true

	case reflect.Float32, reflect.Float64:
		// In this example, we call strconv.FormatFloat with the following arguments:
		//   val.Float(): the floating-point number to convert (of type float64)
		//   'f': the format to use for the conversion (in this case, we want a decimal point and digits after it)
		//   -1: the precision to use (the number of digits after the decimal point).
		//       We specify -1 to use the default precision for the format.
		//   64: the bit size of the floating-point number (float64 in this case)
		return []byte(strconv.FormatFloat(val.Float(), 'f', -1, 64)), nil, true

	case reflect.Bool:
		return []byte(strconv.FormatBool(val.Bool())), nil, true

	case reflect.Slice:
		if len(opts.Separator) == 0 {
			data, err := json.Marshal(val.Interface())
			return data, err, true
		}

		var (
			list   = make([][]byte, 0, val.Len())
			sep    = []byte(opts.Separator)
			escSep = slicekit.Merge([]byte(`\`), sep)
		)
		for i, length := 0, val.Len(); i < length; i++ {
			out, err := MarshalReflect(val.Index(i), opts)
			if err != nil {
				return nil, fmt.Errorf("error while formatting eleement at index: %d\n%#v", i,
					val.Index(i).Interface()), true
			}
			out = bytes.Replace(out, sep, escSep, -1)
			list = append(list, out)
		}
		return []byte(bytes.Join(list, sep)), nil, true

	case reflect.Map:
		data, err := json.Marshal(val.Interface())
		return data, err, true

	case reflect.Pointer: // ???
		if val.IsNil() {
			return nil, nil, true
		}
		return pformat(val.Elem(), opts)

	default:
		return nil, nil, false
	}
}

var (
	matchInt   = regexp.MustCompile(`^\d+$`)
	matchFloat = regexp.MustCompile(`^\d+\.\d+$`)
)

func DuckParse[Raw encoded](raw Raw, opts ...Option) (any, error) {
	options := option.ToConfig(opts)
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
		return duckParseList([]byte(raw), options)
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

func duckParseList(raw []byte, options Options) (any, error) {
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

var encodingTextMarshaler = reflectkit.TypeOf[encoding.TextMarshaler]()

func textMarshal(val reflect.Value) ([]byte, error, bool) {
	if reflect.PointerTo(val.Type()).Implements(encodingTextMarshaler) {
		val = reflectkit.PointerOf(val)
	}
	if val.Type().Implements(encodingTextMarshaler) {
		result := val.MethodByName("MarshalText").Call([]reflect.Value{})
		text, _ := result[0].Interface().([]byte)
		err, _ := result[1].Interface().(error)
		return text, err, true
	}
	return nil, nil, false
}

var encodingTextUnmarshaler = reflectkit.TypeOf[encoding.TextUnmarshaler]()

func textUnmarshal(typ reflect.Type, data []byte) (reflect.Value, error, bool) {
	var ptr = reflect.New(typ)
	if ptr.Type().Implements(encodingTextUnmarshaler) {
		res := ptr.MethodByName("UnmarshalText").Call([]reflect.Value{reflect.ValueOf(data)})
		err, _ := res[0].Interface().(error)
		return ptr.Elem(), err, true
	}
	if ptr.Type().Elem().Implements(encodingTextUnmarshaler) {
		value := ptr.Elem()
		res := value.MethodByName("UnmarshalText").Call([]reflect.Value{reflect.ValueOf(data)})
		err, _ := res[0].Interface().(error)
		return value, err, true
	}
	return reflect.Value{}, nil, false
}
