package validate

import (
	"fmt"
	"reflect"
	"strings"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/pkg/slicekit"
	"go.llib.dev/frameless/port/option"
)

type Validator interface {
	Validate() error
	// Validate(context.Context) error
}

// Super will validate a value but ignores if the Validator interface is implemented.
// func Super(v any) error {
// 	return Value(v, option.Func[config](func(c *config) {
// 		c.WithoutValidator = true
// 	}))
// }

var interfaceValidator = reflectkit.TypeOf[Validator]()

func Value(v any, opts ...Option) error {
	rv := reflectkit.ToValue(v)
	c := option.Use(opts)

	if rv.Kind() == reflect.Struct {
		return Struct(rv, opts...)
	}

	if err := tryValidatorValidate(rv, c); err != nil {
		return err
	}

	if err := enum.Validate(v); err != nil {
		return Error{Cause: err}
	}

	return nil
}

func Struct(v any, opts ...Option) error {
	rv := reflectkit.ToValue(v)
	c := option.Use(opts)

	if rv.Kind() != reflect.Struct {
		return ImplementationError.F("non struct type type: %s", rv.Type().String())
	}

	if err := tryValidatorValidate(rv, c); err != nil {
		return err
	}

	var (
		T   = rv.Type()
		num = T.NumField()
	)

	for i := 0; i < num; i++ {
		if err := StructField(T.Field(i), rv.Field(i), c); err != nil {
			return err
		}
	}

	return nil
}

func StructField(sf reflect.StructField, field reflect.Value, opts ...Option) error {
	if sf.Type != field.Type() {
		return ImplementationError.F("struct field doesn't belong to the provided field value (%s <=> %s)",
			sf.Type.String(), field.Type().String())
	}

	if err := enum.ValidateStructField(sf, field); err != nil {
		return Error{Cause: err}
	}

	if err := rangeTag.HandleStructField(sf, field); err != nil {
		return Error{Cause: err}
	}

	if err := charTag.HandleStructField(sf, field); err != nil {
		return Error{Cause: err}
	}

	if err := tryValidatorValidate(field, option.Use(opts)); err != nil {
		return err
	}

	return nil
}

type Option option.Option[config]

type config struct {
	WithoutValidator bool
}

func (c config) Configure(t *config) { *t = c }

func tryValidatorValidate(rv reflect.Value, c config) error {
	if c.WithoutValidator {
		return nil
	}
	if !rv.Type().Implements(interfaceValidator) {
		return nil
	}
	outTuble := rv.MethodByName("Validate").Call([]reflect.Value{})
	err, ok := outTuble[0].Interface().(error)
	if ok && err != nil {
		return Error{Cause: err}
	}
	return nil
}

type rangeTagChecks []func(field reflect.StructField, value reflect.Value) error

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

func parseMinMaxRanges(name string, typ reflect.Type, rawMinMaxRange, minMaxSepSym string) (rangeMinMax[reflect.Value], error) {
	var v rangeMinMax[reflect.Value]

	rawMinMax := strings.Split(rawMinMaxRange, minMaxSepSym)
	rawMinMax = slicekit.Map(rawMinMax, strings.TrimSpace)

	if len(rawMinMax) != 2 {
		return v, fmt.Errorf("invalid range value in the .%s field's tag. Expected format: “{min}%s{max}”, but got: %s", name, minMaxSepSym, rawMinMaxRange)
	}

	if rawMin := rawMinMax[0]; 0 < len(rawMin) {
		min, err := convkit.ParseReflect(typ, rawMin)
		if err != nil {
			return v, fmt.Errorf("the minimum range value for the %s type in the .%s field's tag is invalid: %s", typ.String(), name, rawMin)
		}
		v.Min = &min
	}

	if rawMax := rawMinMax[1]; 0 < len(rawMax) {
		max, err := convkit.ParseReflect(typ, rawMax)
		if err != nil {
			return v, fmt.Errorf("the maximum range value for the %s type in the .%s field's tag is invalid: %s", typ.String(), name, rawMax)
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

var rangeTag = reflectkit.TagHandler[rangeTagChecks]{
	Name: "range",
	Parse: func(sf reflect.StructField, tagValue string) (rangeTagChecks, error) {
		var checks rangeTagChecks
		var charChecks charTagChecks

		for _, rawRange := range splitList(tagValue) {
			switch {
			case isCharTagFormat(sf.Type, rawRange):
				charRange, err := charTag.Parse(sf, rawRange)
				if err != nil {
					return checks, err
				}
				// in order to apply mixed char checks on a given value
				// we must collect all char checks, and use them as a unit.
				// for example to have `range:"a-c,e-g"` accept "abcefg"
				charChecks = append(charChecks, charRange...)

			default:
				rangeMinMax, err := parseMinMaxRanges(sf.Name, sf.Type, rawRange, "..")
				if err != nil {
					return checks, err
				}

				checks = append(checks, func(field reflect.StructField, value reflect.Value) error {
					return rangeTagCheck(rangeMinMax).Validate(value)
				})
			}
		}

		if 0 < len(charChecks) {
			checks = append(checks, func(field reflect.StructField, value reflect.Value) error {
				return charTag.Use(field, value, charChecks)
			})
		}

		return checks, nil
	},
	Use: func(field reflect.StructField, value reflect.Value, checks rangeTagChecks) error {
		var errs []error
		for _, check := range checks {
			err := check(field, value)
			if err == nil { // if any check pass, we are good
				return nil
			}
			errs = append(errs, err)
		}
		return errorkit.Merge(errs...)
	},

	ForceCache: true,

	PanicOnParseError: true,
}

type charTagChecks []rangeMinMax[rune]

var stringType = reflectkit.TypeOf[string]()

func isCharTagFormat(typ reflect.Type, tagValue string) bool {
	return typ.ConvertibleTo(stringType) && strings.Count(tagValue, rangeSepSym) == 0 && strings.Count(tagValue, charSepSym) == 1
}

var charTag = reflectkit.TagHandler[charTagChecks]{
	Name: "char",
	Parse: func(sf reflect.StructField, tagValue string) (charTagChecks, error) {
		var checks = charTagChecks{}

		if !sf.Type.ConvertibleTo(stringType) {
			const format = "char range expression can only work with types which are converable to string, unlike %s field which is a %s type"
			return checks, fmt.Errorf(format, sf.Name, sf.Type.String())
		}

		for _, rawMinMax := range splitList(tagValue) {
			minMax, err := parseMinMaxRanges(sf.Name, stringType, rawMinMax, charSepSym)
			if err != nil {
				return checks, err
			}

			var check rangeMinMax[rune]
			if minMax.Min != nil {
				chars := []rune(minMax.Min.String())
				if 1 < len(chars) {
					return checks, fmt.Errorf("the min part of a \"min-max\" char tag must be a single character: %s", string(chars))
				}
				check.Min = pointer.Of(chars[0])
			}
			if minMax.Max != nil {
				chars := []rune(minMax.Max.String())
				if 1 < len(chars) {
					return checks, fmt.Errorf("the max part of a \"min-max\" char tag must be a single character: %s", string(chars))
				}
				check.Max = pointer.Of(chars[0])
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

	ForceCache:        true,
	PanicOnParseError: true,
}
