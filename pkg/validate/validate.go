package validate

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/option"
)

type Validatable interface {
	Validate(context.Context) error
}

var interfaceValidatable = reflectkit.TypeOf[Validatable]()

func Value[T any](ctx context.Context, v T) error {
	rv := reflectkit.ToValue(v)

	if rv.Kind() == reflect.Struct {
		return Struct(ctx, rv)
	}

	if err := tryValidatorValidate(ctx, rv); err != nil {
		return err
	}

	c, ok := ctxConfig.Lookup(ctx)
	if !ok || !c.SkipEnum {
		if err := enum.ReflectValidate(rv, enum.Type[T]()); err != nil {
			return Error{Cause: err}
		}
	}

	return nil
}

func Struct(ctx context.Context, v any) error {
	rStruct := reflectkit.ToValue(v)

	if rStruct.Kind() != reflect.Struct {
		return ImplementationError.F("non struct type type: %s", rStruct.Type().String())
	}

	if err := tryValidatorValidate(ctx, rStruct); err != nil {
		return err
	}

	var (
		T        = rStruct.Type()
		NumField = T.NumField()
	)
	ctx = ctxWithStructFieldScope(ctx, rStruct)
	for i := 0; i < NumField; i++ {
		if err := StructField(ctx, T.Field(i), rStruct.Field(i)); err != nil {
			return err
		}
	}

	return nil
}

func StructField(ctx context.Context, field reflect.StructField, value reflect.Value) error {
	if !field.IsExported() {
		return nil
	}

	if field.Type != value.Type() {
		return ImplementationError.F("struct field doesn't belong to the provided field value (%s <=> %s)",
			field.Type.String(), value.Type().String())
	}

	if err := enum.ValidateStructField(field, value); err != nil {
		return Error{Cause: err}
	}

	c, _ := ctxConfig.Lookup(ctx)
	c.SkipEnum = true
	ctx = ctxConfig.ContextWith(ctx, c)

	if err := rangeTag.HandleStructField(field, value); err != nil {
		return Error{Cause: err}
	}

	if err := charTag.HandleStructField(field, value); err != nil {
		return Error{Cause: err}
	}

	if err := minTag.HandleStructField(field, value); err != nil {
		return Error{Cause: err}
	}

	if err := maxTag.HandleStructField(field, value); err != nil {
		return Error{Cause: err}
	}

	if err := lengthTag.HandleStructField(field, value); err != nil {
		return Error{Cause: err}
	}

	if err := regexpTag.HandleStructField(field, value); err != nil {
		return Error{Cause: err}
	}

	return Value(ctx, value)
}

type Option option.Option[config]

type config struct {
	Path []string

	SkipEnum        bool
	SkipValidate    bool
	SkipValidateFor map[reflectkit.UID]struct{}
}

func (c config) Context() context.Context {
	return context.Background()
}

type ctxKeyConfig struct{}

var ctxConfig contextkit.ValueHandler[ctxKeyConfig, config]

func ctxWithStructFieldScope(ctx context.Context, rStruct reflect.Value) context.Context {
	var c config
	if cfg, ok := ctxConfig.Lookup(ctx); ok {
		c = cfg
	}
	return ctxConfig.ContextWith(ctx, c.StructFieldScope(rStruct))
}

func (c config) StructFieldScope(rStruct reflect.Value) config {
	c.SkipValidate = false                                         // Skip validate only applies to the given value, not to its filds
	c.SkipEnum = false                                             // SkipEnum not needed
	c.Path = append(slicekit.Clone(c.Path), rStruct.Type().Name()) // add struct name to the validation path
	return c
}

func (c config) Configure(t *config) { *t = c }

// SkipValidate option indicates that the function is being used inside a Validate(context.Context) error method.
//
// When this option is set, the Validate function call is skipped to prevent an infinite loop caused by a circular Validate call.
func SkipValidate(ctx context.Context) context.Context {
	c, _ := ctxConfig.Lookup(ctx)
	c.SkipValidate = true
	return ctxConfig.ContextWith(ctx, c)
}

func skipValidateOf(ctx context.Context, uid reflectkit.UID) context.Context {
	c, _ := ctxConfig.Lookup(ctx)
	if c.SkipValidateFor == nil {
		c.SkipValidateFor = make(map[reflectkit.UID]struct{})
	}
	c.SkipValidateFor[uid] = struct{}{}
	return ctxConfig.ContextWith(ctx, c)
}

func tryValidatorValidate(ctx context.Context, rv reflect.Value) error {
	c, _ := ctxConfig.Lookup(ctx)
	if c.SkipValidate {
		return nil
	}
	if !rv.Type().Implements(interfaceValidatable) {
		return nil
	}
	uid := reflectkit.UIDOf(rv, c.Path...)
	if _, ok := c.SkipValidateFor[uid]; ok {
		return nil
	}
	ctx = skipValidateOf(ctx, uid)
	outTuble := rv.MethodByName("Validate").Call([]reflect.Value{reflect.ValueOf(ctx)})
	err, ok := outTuble[0].Interface().(error)
	if ok && err != nil {
		return Error{Cause: err}
	}
	return nil
}

type checkFunc func(value reflect.Value) error

type rangeTagCheck struct {
	Min *reflect.Value
	Max *reflect.Value
}

func (rtc rangeTagCheck) Validate(v reflect.Value) error {
	if rtc.Min != nil {
		cmp, err := reflectkit.Compare(*rtc.Min, v)
		if err != nil {
			return err
		}
		if !(cmp <= 0) {
			return fmt.Errorf("[min] %v <= %v [value]", rtc.Min.Interface(), v.Interface())
		}
	}
	if rtc.Max != nil {
		cmp, err := reflectkit.Compare(v, *rtc.Max)
		if err != nil {
			return err
		}
		if !(cmp <= 0) {
			return fmt.Errorf("[value] %v <= %v [max]", v.Interface(), rtc.Max.Interface())
		}
	}
	return nil
}

type rangeMinMax[T any] struct {
	Min *T
	Max *T
}

func splitList(tagValue string) []string {
	return slicekit.Map(strings.Split(tagValue, ","), strings.TrimSpace)
}

func parseMinMaxRanges(T reflect.Type, rawMinMaxRange, minMaxSepSym string) (rangeMinMax[reflect.Value], error) {
	var v rangeMinMax[reflect.Value]

	rawMinMax := strings.Split(rawMinMaxRange, minMaxSepSym)
	rawMinMax = slicekit.Map(rawMinMax, strings.TrimSpace)

	if len(rawMinMax) != 2 {
		return v, fmt.Errorf("invalid range valuem expected format: “{min}%s{max}”, but got: %s", minMaxSepSym, rawMinMaxRange)
	}

	if rawMin := rawMinMax[0]; 0 < len(rawMin) {
		min, err := convkit.ParseReflect(T, rawMin)
		if err != nil {
			return v, fmt.Errorf("the minimum range value for the %s type is invalid: %s", T.String(), rawMin)
		}
		v.Min = &min
	}

	if rawMax := rawMinMax[1]; 0 < len(rawMax) {
		max, err := convkit.ParseReflect(T, rawMax)
		if err != nil {
			return v, fmt.Errorf("the maximum range value for the %s type is invalid: %s", T.String(), rawMax)
		}
		v.Max = &max
	}

	if v.Min != nil && v.Max != nil {
		cmp, err := reflectkit.Compare(*v.Min, *v.Max)
		if err == nil && cmp == 1 {
			v.Min, v.Max = v.Max, v.Min
		}
	}

	return v, nil
}

const rangeSepSym = ".."
const charSepSym = "-"

func anyOfCheckFunc(checks []checkFunc) checkFunc {
	if len(checks) == 0 {
		return func(value reflect.Value) error { return nil }
	}
	return func(value reflect.Value) error {
		var errs []error
		for _, check := range checks {
			err := check(value)
			if err == nil { // if any check pass, we are good
				return nil
			}
			errs = append(errs, err)
		}
		return errorkit.Merge(errs...)
	}
}

var rangeTag = reflectkit.TagHandler[checkFunc]{
	Name: "range",

	ForceCache:        true,
	PanicOnParseError: true,

	Parse: func(field reflect.StructField, tagName, tagValue string) (checkFunc, error) {
		var checks []checkFunc
		var charChecks charTagChecks

		for _, raw := range splitList(tagValue) {
			raw := raw // copy bass by value

			if checkCharTagFormat(field.Type, raw) {
				charRange, err := charTag.Parse(field, tagName, raw)
				if err != nil {
					return nil, err
				}
				// in order to apply mixed char checks on a given value
				// we must collect all char checks, and use them as a unit.
				// for example to have `range:"a-c,e-g"` accept "abcefg"
				charChecks = append(charChecks, charRange...)
				continue
			}

			check, ok, err := tryRangeFormat(field.Type, raw)
			if err != nil {
				return nil, fmt.Errorf(".%s has an invalid range value: %w", field.Name, err)
			}
			if ok {
				checks = append(checks, check)
				continue
			}

			check, ok, err = tryComparisonFormat(field.Type, raw)
			if err != nil {
				return nil, err
			}
			if ok {
				checks = append(checks, check)
				continue
			}

			// unknown format
			return nil, fmt.Errorf("unrecognised range format: %s", raw)
		}

		if 0 < len(charChecks) {
			checks = append(checks, func(value reflect.Value) error {
				return charTag.Use(field, value, charChecks)
			})
		}
		return anyOfCheckFunc(checks), nil
	},
	Use: func(field reflect.StructField, value reflect.Value, check checkFunc) error {
		return check(value)
	},
}

func tryRangeFormat(T reflect.Type, raw string) (checkFunc, bool, error) {
	sep, ok := checkRangeFormat(T, raw)
	if !ok {
		return nil, false, nil
	}

	rangeMinMax, err := parseMinMaxRanges(T, raw, sep)
	if err != nil {
		return nil, false, err
	}

	rtc := rangeTagCheck(rangeMinMax)
	var check = func(value reflect.Value) error {
		return rtc.Validate(value)
	}

	return check, true, nil
}

func checkRangeFormat(typ reflect.Type, raw string) (string, bool) {
	if strings.Count(raw, rangeSepSym) == 1 {
		return rangeSepSym, true
	}
	return "", false
}

type charTagChecks []rangeMinMax[rune]

var stringType = reflectkit.TypeOf[string]()

func checkCharTagFormat(typ reflect.Type, raw string) bool {
	return typ.ConvertibleTo(stringType) && strings.Count(raw, rangeSepSym) == 0 && strings.Count(raw, charSepSym) == 1
}

func isCharFormat(field reflect.StructField, raw string) (rangeMinMax[rune], bool, error) {
	var check rangeMinMax[rune]

	if !field.Type.ConvertibleTo(stringType) {
		return check, false, nil
	}

	minMax, err := parseMinMaxRanges(stringType, raw, charSepSym)
	if err != nil {
		return check, false, fmt.Errorf(".%s field's char tag has an issue: %w", field.Name, err)
	}

	if minMax.Min != nil {
		chars := []rune(minMax.Min.String())
		if 1 < len(chars) {
			return check, false, fmt.Errorf("the min part of a \"min-max\" char tag must be a single character: %s", string(chars))
		}
		check.Min = pointer.Of(chars[0])
	}
	if minMax.Max != nil {
		chars := []rune(minMax.Max.String())
		if 1 < len(chars) {
			return check, false, fmt.Errorf("the max part of a \"min-max\" char tag must be a single character: %s", string(chars))
		}
		check.Max = pointer.Of(chars[0])
	}

	return check, true, nil
}

var charTag = reflectkit.TagHandler[charTagChecks]{
	Name: "char",

	ForceCache:        true,
	PanicOnParseError: true,

	Parse: func(field reflect.StructField, tagName, tagValue string) (charTagChecks, error) {
		var checks = charTagChecks{}

		if !field.Type.ConvertibleTo(stringType) {
			const format = "char range expression can only work with types which are converable to string, unlike %s field which is a %s type"
			return checks, fmt.Errorf(format, field.Name, field.Type.String())
		}

		for _, rawMinMax := range splitList(tagValue) {
			check, ok, err := isCharFormat(field, rawMinMax)
			if err != nil {
				return nil, fmt.Errorf(".%s field's char tag has an issue: %w", field.Name, err)
			}
			if !ok {
				return nil, fmt.Errorf(".%s field's char tag format is not recognised: %s", field.Name, rawMinMax)
			}
			checks = append(checks, check)
		}

		return checks, nil
	},
	Use: func(sf reflect.StructField, field reflect.Value, v charTagChecks) error {
		str := field.Convert(stringType).String()

		for _, char := range str {
			var pass bool
			for _, charRange := range v {
				if charRange.Min != nil {
					if char < *charRange.Min {
						continue
					}
				}
				if charRange.Max != nil {
					if *charRange.Max < char {
						continue
					}
				}
				pass = true
				break
			}

			if !pass {
				return fmt.Errorf(".%s field's was expected to be within the character set", sf.Name)
			}
		}

		return nil
	},
}

var minTag = reflectkit.TagHandler[reflect.Value]{
	Name: "min",

	ForceCache:        true,
	PanicOnParseError: true,

	Parse: func(field reflect.StructField, tagName, tagValue string) (reflect.Value, error) {
		return convkit.ParseReflect(field.Type, tagValue)
	},

	Use: func(field reflect.StructField, val, min reflect.Value) error {
		cmp, err := reflectkit.Compare(min, val)
		if err != nil {
			return err
		}
		if 0 < cmp {
			return fmt.Errorf("expected that %v is minimum %v", val.Interface(), min.Interface())
		}
		return nil
	},
}

var maxTag = reflectkit.TagHandler[reflect.Value]{
	Name: "max",

	ForceCache:        true,
	PanicOnParseError: true,

	Parse: func(field reflect.StructField, tagName, tagValue string) (reflect.Value, error) {
		return convkit.ParseReflect(field.Type, tagValue)
	},

	Use: func(field reflect.StructField, val, max reflect.Value) error {
		cmp, err := reflectkit.Compare(val, max)
		if err != nil {
			return err
		}
		if 0 < cmp {
			return fmt.Errorf("expected that %v is maximum %v", val.Interface(), max.Interface())
		}
		return nil
	},
}

func tryComparisonFormat(T reflect.Type, raw string) (checkFunc, bool, error) {
	cmpOp, rawVal, ok, err := checkComparisonFormat(T, raw)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	refVal, err := convkit.ParseReflect(T, rawVal)
	if err != nil {
		return nil, true, err
	}

	var check checkFunc = func(value reflect.Value) error {
		cmp, err := reflectkit.Compare(value, refVal)
		if err != nil {
			return err
		}
		if checkComparison(cmpOp, cmp) {
			return nil
		}
		return fmt.Errorf("comparison failed for %v, expected it to be %s", value.Interface(), raw)
	}

	return check, true, nil
}

type cmpOp string

const (
	less           cmpOp = "<"
	lessOrEqual    cmpOp = "<="
	equal          cmpOp = "="
	greater        cmpOp = ">"
	greaterOrEqual cmpOp = ">="
	notEqual       cmpOp = "!="
)

var mapToLeftHandCmpOp = map[string]cmpOp{
	"<":  less,
	"<=": lessOrEqual,
	"=":  equal,
	"==": equal,
	">":  greater,
	">=": greaterOrEqual,
	"!=": notEqual,
}

var mapToRightHandCmpOp = map[string]cmpOp{
	"<":  greater,
	"<=": greaterOrEqual,
	"=":  equal,
	"==": equal,
	">":  less,
	">=": lessOrEqual,
	"!=": notEqual,
}

func checkComparison(is cmpOp, cmp int) bool {
	switch is {
	case less:
		return cmp < 0
	case lessOrEqual:
		return cmp <= 0
	case equal:
		return cmp == 0
	case greater:
		return 0 < cmp
	case greaterOrEqual:
		return 0 <= cmp
	case notEqual:
		return cmp != 0
	default:
		return false
	}
}

const comparisonOperatorRegexpGroup = `(>=|<=|!=|>|<|=)`

var (
	rgxHasComparisonOperatorPrefix = regexp.MustCompile(fmt.Sprintf(`^%s\s*`, comparisonOperatorRegexpGroup))
	rgxHasComparisonOperatorSuffix = regexp.MustCompile(fmt.Sprintf(`\s*%s$`, comparisonOperatorRegexpGroup))
	rgxIsComparisonFormat          = regexp.MustCompile(fmt.Sprintf(`^%s?\s*.*%s?$`, comparisonOperatorRegexpGroup, comparisonOperatorRegexpGroup))
)

func checkComparisonFormat(typ reflect.Type, raw string) (cmpOp, string, bool, error) {
	if !rgxIsComparisonFormat.MatchString(raw) {
		return "", "", false, nil
	}

	if !rgxHasComparisonOperatorPrefix.MatchString(raw) && !rgxHasComparisonOperatorSuffix.MatchString(raw) {
		return "", "", false, nil
	}

	if rgxHasComparisonOperatorPrefix.MatchString(raw) &&
		rgxHasComparisonOperatorSuffix.MatchString(raw) {
		return "", "", false, fmt.Errorf("it is not supported to have comparison operator on both side of the value: %s", raw)
	}

	var op cmpOp
	switch {
	case rgxHasComparisonOperatorPrefix.MatchString(raw):
		opParts := rgxHasComparisonOperatorPrefix.FindAllStringSubmatch(raw, 1)
		if len(opParts) == 0 {
			return op, "", false, fmt.Errorf("malformed operator: %s", raw)
		}
		if len(opParts[0]) == 0 {
			return op, "", false, fmt.Errorf("malformed operator: %s", raw)
		}

		rawOp := opParts[0][1]

		op, ok := mapToLeftHandCmpOp[rawOp]
		if !ok {
			return op, "", false, fmt.Errorf("malformed operator: %s", raw)
		}

		rawValue := strings.TrimPrefix(raw, rawOp)
		return op, rawValue, true, nil

	case rgxHasComparisonOperatorSuffix.MatchString(raw):
		opParts := rgxHasComparisonOperatorSuffix.FindAllStringSubmatch(raw, 1)
		if len(opParts) == 0 {
			return op, "", false, fmt.Errorf("malformed operator: %s", raw)
		}
		if len(opParts[0]) == 0 {
			return op, "", false, fmt.Errorf("malformed operator: %s", raw)
		}

		rawOp := opParts[0][1]

		op, ok := mapToRightHandCmpOp[rawOp]
		if !ok {
			return op, "", false, fmt.Errorf("malformed operator: %s", raw)
		}

		rawValue := strings.TrimSuffix(raw, rawOp)
		return op, rawValue, true, nil

	default:
		return "", "", false, fmt.Errorf(`invalid comparison style format, expected "{op}{value}" like ">=42" but got: %s`, raw)
	}
}

var lengthTag = reflectkit.TagHandler[checkFunc]{
	Name: "length",

	Alias: []string{"len"},

	ForceCache: true,

	PanicOnParseError: true,

	Parse: func(field reflect.StructField, tagName, tagValue string) (checkFunc, error) {
		var checks []checkFunc
		for _, raw := range splitList(tagValue) {
			switch field.Type.Kind() {
			case reflect.Slice, reflect.String, reflect.Map, reflect.Chan:
				checkLen, ok, err := tryLengthTagLenFormat(raw)
				if err != nil {
					return nil, fmt.Errorf("%s field's %w", field.Name, err)
				}
				if ok {
					checks = append(checks, checkLen)
					continue
				}
				return nil, fmt.Errorf("unrecognised length tag format: %s (%s)", tagValue, raw)

			default:
				return nil, fmt.Errorf(`"length" tag doesn't support %s type (.%s)`, field.Type.String(), field.Name)
			}
		}
		return anyOfCheckFunc(checks), nil
	},

	Use: func(field reflect.StructField, value reflect.Value, check checkFunc) error {
		return check(value)
	},
}

var intType = reflectkit.TypeOf[int]()

var rgxIsDigit = regexp.MustCompile(`^\d+$`)

func tryLengthTagLenFormat(raw string) (checkFunc, bool, error) {
	if rgxIsDigit.MatchString(raw) {
		length, err := convkit.ParseReflect(intType, raw)
		if err != nil {
			return nil, false, err
		}
		var fn checkFunc = func(value reflect.Value) error {
			if value.Len() == int(length.Int()) {
				return nil
			}
			return fmt.Errorf("expected length of %v, but got %v", length.Interface(), value.Interface())
		}
		return fn, true, nil
	}

	fn, ok, err := tryComparisonFormat(intType, raw)
	if err != nil {
		return nil, false, fmt.Errorf(`"length" tag has an invalid comparison format for slice: %w`, err)
	}
	if ok {
		return func(value reflect.Value) error {
			return fn(reflect.ValueOf(value.Len()))
		}, true, nil
	}

	fn, ok, err = tryRangeFormat(intType, raw)
	if err != nil {
		return nil, false, fmt.Errorf(`"length" tag has an invalid range format for slice: %w`, err)
	}
	if ok {
		return func(value reflect.Value) error {
			return fn(reflect.ValueOf(value.Len()))
		}, true, nil
	}

	return nil, false, nil
}

var byteSliceType = reflectkit.TypeOf[[]byte]()

type tagRegexp struct {
	Name string
	*regexp.Regexp
}

type X struct {
	X string `match-posix:"^foo$"`
}

var regexpTag = reflectkit.TagHandler[tagRegexp]{
	Name: "regexp",

	Alias: []string{"rgx", "match"},

	ForceCache:        true,
	PanicOnParseError: true,

	Parse: func(field reflect.StructField, name, value string) (tagRegexp, error) {
		if !field.Type.ConvertibleTo(byteSliceType) {
			return tagRegexp{}, fmt.Errorf("regexp validation tag only supports []byte and string types")
		}
		rgx, err := regexp.Compile(value)
		if err != nil {
			return tagRegexp{}, err
		}
		return tagRegexp{
			Name:   name,
			Regexp: rgx,
		}, nil
	},

	Use: func(field reflect.StructField, value reflect.Value, rgx tagRegexp) error {
		if rgx.Regexp.Match(value.Convert(byteSliceType).Interface().([]byte)) {
			return nil
		}

		return fmt.Errorf("%#v doesn't match %q regexp [%s tag]", value.Interface(), rgx.String(), rgx.Name)
	},
}
