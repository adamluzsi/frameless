package env

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/internal/tagint"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/pkg/validate"
)

const (
	ErrInvalidType  errorkit.Error = "ErrInvalidType"
	ErrInvalidValue errorkit.Error = "ErrInvalidValue"
)

const ErrMissingEnvironmentVariable errorkit.Error = "ErrMissingEnvironmentVariable"

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
		return ErrInvalidType.F("nil value received")
	}
	ctx := context.Background()
	return reflectLoad(ctx, reflect.ValueOf(ptr))
}

func reflectLoad(ctx context.Context, ptr reflect.Value) error {
	rStruct, err := reflectStructValueOfPointer(ptr)
	if err != nil {
		return err
	}
	if err := ReflectTryLoad(ptr); err != nil {
		return err
	}
	return validate.Struct(ctx, rStruct.Interface())
}

// ReflectTryLoad attempts to load configuration struct, but does not produce an error if the overall config is not valid.
// Instead, it raises a validation issue only when a given struct field's value is present in the environment variables
// but does not meet the validation requirements for the specified field type/tag.
func ReflectTryLoad(ptr reflect.Value) error {
	ctx := context.Background()
	val, err := reflectStructValueOfPointer(ptr)
	if err != nil {
		return err
	}
	if val.Kind() != reflect.Struct {
		return ErrInvalidValue.F("non struct value type passed to TryLoad: %s", val.Type().String())
	}
	if err := vistiLoadStruct(ctx, val); err != nil {
		return err
	}
	return nil
}

func reflectStructValueOfPointer(ptr reflect.Value) (reflect.Value, error) {
	if ptr.Kind() != reflect.Pointer {
		return reflect.Value{}, ErrInvalidType.F("non pointer type received")
	}
	if ptr.IsNil() {
		return reflect.Value{}, ErrInvalidType.F("nil pointer received")
	}
	return reflectkit.BaseValue(ptr), nil
}

func vistiLoadStruct(ctx context.Context, rStruct reflect.Value) error {
	for _, phase := range loadPhases {
		if err := phase.visitStruct(ctx, rStruct, false); err != nil {
			return err
		}
	}
	return nil
}

// loadPhase is a pass over the fields of the struct being loaded.
//
// The values are loaded in the order of their precedence, so a later one overwrites an earlier one:
//
//  1. the defaults, inner fields first, then the default of their struct field, merged onto them;
//  2. the env vars, inner fields first, then the env var of their struct field, merged onto them.
//
// So a default never overwrites an env var,
// and the JSON value of a struct field overwrites only the inner fields it mentions.
// The required fields and the validation tags are checked only once every value is loaded,
// since an inner field can get its value from the JSON value of its struct field as well.
type loadPhase int

const (
	loadDefaults loadPhase = iota
	loadEnvVars
	checkFields
)

var loadPhases = []loadPhase{loadDefaults, loadEnvVars, checkFields}

// visitStruct runs the phase on the fields of rStruct.
// hasOuterJSON tells whether an outer struct field has a JSON value, which may give a value to these fields.
func (phase loadPhase) visitStruct(ctx context.Context, rStruct reflect.Value, hasOuterJSON bool) error {
	for i, numField := 0, rStruct.NumField(); i < numField; i++ {
		if err := phase.visitField(ctx, rStruct.Type().Field(i), rStruct.Field(i), hasOuterJSON); err != nil {
			return err
		}
	}
	return nil
}

func (phase loadPhase) visitField(ctx context.Context, sf reflect.StructField, field reflect.Value, hasOuterJSON bool) error {
	if !sf.IsExported() {
		return nil
	}
	f, ok, err := lookupEnvField(sf, field)
	if err != nil {
		return err
	}
	// We don't want to visit struct types which have their own registered parser,
	// or which are opaque, since their tags are not meant for the env package.
	isNested := field.Kind() == reflect.Struct && !tagint.IsOpaque(sf) && !convkit.IsRegistered(field.Interface())
	if isNested {
		// The inner fields go first, so the struct field's own JSON value is merged onto them.
		if err := phase.visitStruct(ctx, field, hasOuterJSON || (ok && f.HasValue())); err != nil {
			return err
		}
	}
	if !ok {
		return nil
	}
	switch phase {
	case loadDefaults:
		return f.LoadDefault()
	case loadEnvVars:
		return f.LoadEnvVar()
	case checkFields:
		return f.Check(ctx, isNested || hasOuterJSON)
	default:
		return nil
	}
}

// envField is a struct field with an env tag.
type envField struct {
	StructField reflect.StructField
	Value       reflect.Value
	Names       []string
	Options     lookupEnvOptions
}

func lookupEnvField(sf reflect.StructField, field reflect.Value) (envField, bool, error) {
	names, ok := LookupFieldEnvNames(sf)
	if !ok {
		return envField{}, false, nil
	}
	opts, err := getLookupEnvOptions(sf)
	if err != nil {
		return envField{}, false, err
	}
	return envField{
		StructField: sf,
		Value:       field,
		Names:       names,
		Options:     opts,
	}, true, nil
}

// LookupEnvVar returns the value of the field's first env var that is present in the environment.
func (f envField) LookupEnvVar() (string, bool) {
	for _, name := range f.Names {
		if val, ok := os.LookupEnv(name); ok {
			return val, true
		}
	}
	return "", false
}

// HasValue tells whether the field gets a value of its own, from its env var or its default.
func (f envField) HasValue() bool {
	_, ok := f.LookupEnvVar()
	return ok || f.Options.DefaultValue != nil
}

// LoadDefault sets the field to its default.
// A field with an env var skips it, since the env var overwrites it anyway,
// except a field taking a JSON object, since the env var's JSON object is merged onto the default.
func (f envField) LoadDefault() error {
	if f.Options.DefaultValue == nil {
		return nil
	}
	if _, ok := f.LookupEnvVar(); ok && !tagint.IsJSONObject(f.Value.Type()) {
		return nil
	}
	return f.set(*f.Options.DefaultValue)
}

func (f envField) LoadEnvVar() error {
	raw, ok := f.LookupEnvVar()
	if !ok {
		return nil
	}
	return f.set(raw)
}

func (f envField) set(raw string) error {
	opts := convkit.Options{
		Separator:  tagint.Separator(pointer.Deref(f.Options.Separator), raw),
		TimeLayout: pointer.Deref(f.Options.TimeLayout),
	}
	val, err := tagint.ParseOnto(f.Value, raw, opts)
	if err != nil {
		return errParsingEnvValue(f.StructField, err)
	}
	if !f.Value.CanSet() {
		return ErrInvalidValue.F("setable struct field was expected")
	}
	f.Value.Set(val)
	return nil
}

// Check validates the field when it has a value of its own, and reports it as missing when it is required but has none.
//
// mayHaveValue tells whether the field may have got a value without having one of its own:
// from the JSON value of an outer struct field, or, for a struct field, from its inner fields.
// Such a value satisfies required, unless it is a zero value.
func (f envField) Check(ctx context.Context, mayHaveValue bool) error {
	if f.HasValue() {
		return validate.StructField(ctx, f.StructField, f.Value)
	}
	if !f.Options.IsRequired {
		return nil
	}
	if mayHaveValue && !reflectkit.IsZero(f.Value) {
		return nil
	}
	name, _ := slicekit.First(f.Names)
	return errParsingEnvValue(f.StructField, errMissingEnvironmentVariable(name))
}

const envTagKey = "env"

var (
	tagsForDefaultValue = []string{"env-default", "default"}
	tagsForRequired     = []string{"env-required", "env-require", "required", "require"}
	tagsForSeparator    = []string{"env-separator", "separator"}
)

// envTimeLayoutTag is the env specific variant of the shared "layout" tag, and takes precedence over it.
const envTimeLayoutTag = "env-time-layout"

func getLookupEnvOptions(sf reflect.StructField) (lookupEnvOptions, error) {
	var (
		opts lookupEnvOptions
		tag  = sf.Tag
	)
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
	if value, ok := tag.Lookup(envTimeLayoutTag); ok {
		opts.TimeLayout = &value
	} else if value, ok, _ := tagint.TimeLayout.Lookup(sf); ok {
		opts.TimeLayout = &value
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
		Separator:  tagint.Separator(pointer.Deref(opts.Separator), val),
		TimeLayout: pointer.Deref(opts.TimeLayout),
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
	return ErrMissingEnvironmentVariable.F("%s", key)
}

func errParsingEnvValue(structField reflect.StructField, err error) error {
	return ErrInvalidValue.F("error parsing the value for %s: %w", structField.Name, err)
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

// Init is a syntax sugar for Load[T](&v).
// It can be easily used for global variable initialisation
func Init[ConfigStruct any]() (c ConfigStruct, err error) {
	err = Load[ConfigStruct](&c)
	return c, err
}

// InitGlobal is designed to be used as part of your global variable initialisation.
func InitGlobal[ConfigStruct any]() ConfigStruct {
	c, err := Init[ConfigStruct]()
	if err != nil {
		fmt.Fprintf(osint.Stderr(), "%s\n", err.Error())
		osint.Exit(1)
	}
	return c
}

func LookupFieldEnvNames(sf reflect.StructField) ([]string, bool) {
	raw, ok := sf.Tag.Lookup(envTagKey)
	if !ok {
		return nil, false
	}
	return slicekit.Map(strings.Split(raw, ","), strings.TrimSpace), true
}
