package convkit_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

func Test_smoke(t *testing.T) {
	var (
		duck any
		foo  = "forty-two"
		bar  = "42"
		baz  = "42.42"
		qux  = "1;23;4"

		refTime = rnd.Time()
		layout  = time.RFC3339
		quux    = refTime.Format(layout)
	)

	strval, err := convkit.Parse[string]("")
	assert.NoError(t, err)
	assert.Empty(t, strval)

	duck, err = convkit.DuckParse("")
	assert.NoError(t, err)
	assert.Equal[any](t, duck, "")

	gotstrval, err := convkit.Format[string](strval)
	assert.NoError(t, err)
	assert.Equal(t, "", gotstrval)

	strval, err = convkit.Parse[string](foo)
	assert.NoError(t, err)
	assert.Equal(t, "forty-two", strval)

	duck, err = convkit.DuckParse(foo)
	assert.NoError(t, err)
	assert.Equal[any](t, "forty-two", duck)

	gotstrval, err = convkit.Format[string](strval)
	assert.NoError(t, err)
	assert.Equal(t, "forty-two", gotstrval)

	gotstrval, err = convkit.Format[uint](42)
	assert.NoError(t, err)
	assert.Equal(t, "42", gotstrval)

	_, err = convkit.Parse[int](foo)
	assert.Error(t, err)

	intval, err := convkit.Parse[int](bar)
	assert.NoError(t, err)
	assert.Equal(t, 42, intval)

	duck, err = convkit.DuckParse(bar)
	assert.NoError(t, err)
	assert.Equal[any](t, int(42), duck)

	gotInt, err := convkit.Format[int](intval)
	assert.NoError(t, err)
	assert.Equal(t, bar, gotInt)

	fltval, err := convkit.Parse[float64](baz)
	assert.NoError(t, err)
	assert.Equal(t, 42.42, fltval)

	duck, err = convkit.DuckParse(baz)
	assert.NoError(t, err)
	assert.Equal[any](t, float64(42.42), duck)

	gotFloat, err := convkit.Format[float64](fltval)
	assert.NoError(t, err)
	assert.Equal(t, gotFloat, baz)

	vals, err := convkit.Parse[[]int](qux, convkit.Options{Separator: ";"})
	assert.NoError(t, err)
	assert.Equal(t, []int{1, 23, 4}, vals)

	duck, err = convkit.DuckParse(qux, convkit.Options{Separator: ";"})
	assert.NoError(t, err)
	assert.Equal[any](t, []int{1, 23, 4}, duck)

	gotVals, err := convkit.Format[[]int](vals, convkit.Options{Separator: ";"})
	assert.NoError(t, err)
	assert.Equal(t, gotVals, qux)

	timeval, err := convkit.Parse[time.Time](quux, convkit.Options{TimeLayout: layout})
	assert.NoError(t, err)
	assert.True(t, timeval.Equal(refTime))

	duck, err = convkit.DuckParse(quux, convkit.Options{TimeLayout: layout})
	assert.NoError(t, err)
	timeDuck, ok := duck.(time.Time)
	assert.True(t, ok, "expected to have time.Time result from DuckParse")
	assert.Equal[any](t, timeDuck, refTime)

	gotTimeval, err := convkit.Format[time.Time](timeval, convkit.Options{TimeLayout: layout})
	assert.NoError(t, err)
	assert.Equal(t, gotTimeval, quux)
}

type ImplT[T any] struct {
	convkit.MarshalFunc[T]
	convkit.UnmarshalFunc[T]
}

func (i ImplT[T]) Marshal(v T) ([]byte, error) {
	return i.MarshalFunc(v)
}

func (i ImplT[T]) Unmarshal(data []byte, p *T) error {
	return i.UnmarshalFunc(data, p)
}

func TestIsRegistered(t *testing.T) {
	assert.False(t, convkit.IsRegistered[string]())
	assert.True(t, convkit.IsRegistered[time.Time]())
	assert.True(t, convkit.IsRegistered[*time.Time]())
	assert.True(t, convkit.IsRegistered(time.Now()))

	type X struct{}
	assert.False(t, convkit.IsRegistered[X]())
	undo := convkit.Register[X](ImplT[X]{
		MarshalFunc: func(v X) ([]byte, error) {
			return []byte("X{}"), nil
		},
		UnmarshalFunc: func(data []byte, p *X) error {
			if string(data) != "X{}" {
				return fmt.Errorf("not X")
			}
			*p = X{}
			return nil
		},
	})
	assert.True(t, convkit.IsRegistered[X]())
	assert.True(t, convkit.IsRegistered(X{}))
	undo()
	assert.False(t, convkit.IsRegistered[X]())
	assert.False(t, convkit.IsRegistered(X{}))
}

func Test_listElemContainsSeparator(t *testing.T) {
	const input = "Hello\\, world!,foo,bar,baz"
	opts := convkit.Options{Separator: ","}

	vals, err := convkit.Parse[[]string](input, opts)
	assert.NoError(t, err)
	assert.Equal(t, []string{"Hello, world!", "foo", "bar", "baz"}, vals)

	formatted, err := convkit.Format[[]string](vals, opts)
	assert.NoError(t, err)
	assert.Equal(t, formatted, input)
}

func ExampleParse() {
	_, _ = convkit.Parse[string]("hello")
	_, _ = convkit.Parse[int]("42")
	_, _ = convkit.Parse[float64]("42.24")
	_, _ = convkit.Parse[[]string]("a,b,c", convkit.Options{Separator: ","})
	_, _ = convkit.Parse[[]string]([]byte(`["a","b","c"]`), convkit.Options{Separator: ","})
}

func TestParse(t *testing.T) {
	testParseForType[string](t, "foo-bar-baz", "foo-bar-baz", "")
	testParseForType[int](t, 42, "42", "abc")
	testParseForType[float32](t, 42.24, "42.24", "foo")
	testParseForType[float64](t, 24.42, "24.42", "bar")
	testParseForType[bool](t, true, "true", "baz")
}

func TestOptions(t *testing.T) {
	var (
		qux     = "1;23;4"
		refTime = rnd.Time()
		layout  = time.RFC3339
		quux    = refTime.Format(layout)
	)

	vals, err := convkit.Parse[[]int](qux,
		convkit.Options{Separator: ";"})
	assert.NoError(t, err)
	assert.Equal(t, []int{1, 23, 4}, vals)

	timeval, err := convkit.Parse[time.Time](quux, convkit.Options{TimeLayout: layout})
	assert.NoError(t, err)
	assert.True(t, timeval.Equal(refTime))
}

func TestParse_urlURL(t *testing.T) {
	const (
		invalidRequestURI = "/path/with?invalid=query&characters\\foo\n"
		invalidURI        = "http://example.com" + invalidRequestURI
	)
	t.Run("request uri", func(t *testing.T) {
		val := "/the/path"
		requestURI, err := url.ParseRequestURI(val)
		assert.NoError(t, err)
		testParseForType[url.URL](t, *requestURI, val, invalidURI)
		testParseForType[*url.URL](t, requestURI, val, invalidURI)
	})
	t.Run("url", func(t *testing.T) {
		val := "https://go.llib.dev/frameless"
		u, err := url.ParseRequestURI(val)
		assert.NoError(t, err)
		testParseForType[url.URL](t, *u, val, invalidURI)
		testParseForType[*url.URL](t, u, val, invalidURI)
	})
}

func TestParse_json(t *testing.T) {
	t.Run("array", func(t *testing.T) {
		gotList, err := convkit.Parse[[]string]([]byte(`["a","b","c"]`))
		assert.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, gotList)
	})
	t.Run("object", func(t *testing.T) {
		gotMap, err := convkit.Parse[map[string]int]([]byte(`{"a":42}`))
		assert.NoError(t, err)
		assert.Equal(t, map[string]int{"a": 42}, gotMap)
	})
}

func TestParse_timeTime(t *testing.T) {
	testParseForType[time.Duration](t, time.Minute+5*time.Second, "1m5s", "five minutes")

	t.Run("time.Time struct field", func(t *testing.T) {
		const TimeLayout = "2006-01-02T15"

		t.Run("valid value", func(t *testing.T) {
			refTime, err := time.Parse(TimeLayout, rnd.Time().Format(TimeLayout))
			assert.NoError(t, err)
			got, err := convkit.Parse[time.Time](refTime.Format(TimeLayout),
				convkit.Options{TimeLayout: TimeLayout})
			assert.NoError(t, err)
			assert.Equal(t, refTime, got)
		})

		t.Run("invalid value", func(t *testing.T) {
			_, err := convkit.Parse[time.Time]("2006-invalid-time",
				convkit.Options{TimeLayout: TimeLayout})
			assert.Error(t, err)
		})
	})
}

func testParseForType[T any](t *testing.T, expVal T, valid, invalid string) {
	t.Run(reflectkit.SymbolicName(expVal)+" value type", func(t *testing.T) {
		t.Run("valid value", func(t *testing.T) {
			out, err := convkit.Parse[T](valid)
			assert.NoError(t, err)
			assert.NotEmpty(t, out)
			assert.Equal(t, expVal, out)
		})
		if invalid != "" {
			t.Run("the value, but the value is incorrect", func(t *testing.T) {
				out, err := convkit.Parse[T](invalid)
				assert.Error(t, err)
				assert.Empty(t, out)
			})
		}
	})
	t.Run("*"+reflectkit.SymbolicName(expVal)+" struct field", func(t *testing.T) {
		t.Run("valid value", func(t *testing.T) {
			out, err := convkit.Parse[*T](valid)
			assert.NoError(t, err)
			assert.NotNil(t, out)
			assert.Equal(t, expVal, *out)
		})
		if invalid != "" {
			t.Run("the value, but the value is incorrect", func(t *testing.T) {
				out, err := convkit.Parse[*T](invalid)
				assert.Error(t, err)
				assert.Nil(t, out)
			})
		}
	})
	t.Run("[]"+reflectkit.SymbolicName(expVal)+"", func(t *testing.T) {
		t.Run("valid list value as json", func(t *testing.T) {
			var vs []T
			rnd.Repeat(3, 7, func() {
				vs = append(vs, expVal)
			})
			bs, err := json.Marshal(vs)
			assert.NoError(t, err)

			out, err := convkit.Parse[[]T](bs)
			assert.NoError(t, err)
			assert.NotEmpty(t, out)
			assert.Equal(t, vs, out)
		})
		t.Run("valid comma separated list value", func(t *testing.T) {
			var (
				vs           []T
				envVarValues []string
			)
			rnd.Repeat(3, 7, func() {
				vs = append(vs, expVal)
				envVarValues = append(envVarValues, valid)
			})
			out, err := convkit.Parse[[]T](strings.Join(envVarValues, ","),
				convkit.Options{Separator: ","})
			assert.NoError(t, err)
			assert.NotEmpty(t, out)
			assert.Equal(t, vs, out)
		})
		if invalid != "" {
			t.Run("has the value, but the value is incorrect", func(t *testing.T) {
				out, err := convkit.Parse[[]T](invalid, convkit.Options{Separator: ","})
				assert.Error(t, err)
				assert.Empty(t, out)
			})
		}
	})
}

func Test_spikeReflectSet(t *testing.T) {
	type Person struct {
		Name    string
		Age     int
		Address string
	}

	p := Person{Name: "John", Age: 30, Address: "123 Main St."}

	// Use reflect to set the value of the Age field to 40.
	field := reflect.ValueOf(&p).Elem().FieldByName("Age")
	if field.IsValid() && field.CanSet() {
		field.SetInt(42)
	}

	assert.Equal(t, 42, p.Age)
}

func TestParseWith(t *testing.T) {
	type Conf struct {
		Foo string
		Bar string
	}
	t.Run("happy", func(t *testing.T) {
		conf, err := convkit.Parse[Conf]("foo:bar",
			convkit.UnmarshalWith(func(v []byte, p *Conf) error {
				parts := strings.SplitN(string(v), ":", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid format")
				}
				p.Foo = parts[0]
				p.Bar = parts[1]
				return nil
			}))
		assert.Equal(t, conf, Conf{Foo: "foo", Bar: "bar"})
		assert.NoError(t, err)
	})
	t.Run("rainy", func(t *testing.T) {
		expErr := rnd.Error()
		conf, err := convkit.Parse[Conf]("whatever", convkit.UnmarshalWith(func(v []byte, p *Conf) error {
			return expErr
		}))
		assert.Empty(t, conf)
		assert.ErrorIs(t, err, expErr)
	})
}

func TestParseReflect_smoke(t *testing.T) {
	var (
		foo = "forty-two"
		bar = "42"
		baz = "42.42"
		qux = "1;23;4"

		refTime = rnd.Time()
		layout  = time.RFC3339
		quux    = refTime.Format(layout)
	)

	strval, err := convkit.ParseReflect(reflectkit.TypeOf[string](), "")
	assert.NoError(t, err)
	assert.Empty(t, strval.Interface().(string))

	strval, err = convkit.ParseReflect(reflectkit.TypeOf[string](), foo)
	assert.NoError(t, err)
	assert.Equal(t, "forty-two", strval.Interface().(string))

	_, err = convkit.ParseReflect(reflectkit.TypeOf[int](), foo)
	assert.Error(t, err)

	intval, err := convkit.ParseReflect(reflectkit.TypeOf[int](), bar)
	assert.NoError(t, err)
	assert.Equal(t, 42, intval.Interface().(int))

	fltval, err := convkit.ParseReflect(reflectkit.TypeOf[float64](), baz)
	assert.NoError(t, err)
	assert.Equal(t, 42.42, fltval.Interface().(float64))

	vals, err := convkit.ParseReflect(reflectkit.TypeOf[[]int](),
		qux, convkit.Options{Separator: ";"})

	assert.NoError(t, err)
	assert.Equal(t, []int{1, 23, 4}, vals.Interface().([]int))

	timeval, err := convkit.ParseReflect(reflectkit.TypeOf[time.Time](),
		quux, convkit.Options{TimeLayout: layout})
	assert.NoError(t, err)
	assert.True(t, timeval.Interface().(time.Time).Equal(refTime))

	uintval, err := convkit.ParseReflect(reflectkit.TypeOf[uint](), "42")
	assert.NoError(t, err)
	assert.Equal(t, reflect.Uint, uintval.Kind())
	assert.Equal(t, 42, uintval.Uint())
}

func TestOptions_jsonAsPaseFunc(t *testing.T) {
	var _ = convkit.Options{ParseFunc: json.Unmarshal}
}

func ExampleFormat() {
	formatted, err := convkit.Format(42.24)
	_, _ = formatted, err // "42.24", nil
}

func TestDuckParse(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		duck, err := convkit.DuckParse("")
		assert.NoError(t, err)
		assert.Equal[any](t, duck, "")
	})
	t.Run("string", func(t *testing.T) {
		var str = "forty-two"
		duck, err := convkit.DuckParse(str)
		assert.NoError(t, err)
		assert.Equal[any](t, str, duck)
	})
	t.Run("int", func(t *testing.T) {
		duck, err := convkit.DuckParse("42")
		assert.NoError(t, err)
		assert.Equal[any](t, int(42), duck)
	})
	t.Run("float", func(t *testing.T) {
		var baz = "42.42"
		duck, err := convkit.DuckParse(baz)
		assert.NoError(t, err)
		assert.Equal[any](t, float64(42.42), duck)
	})
	t.Run("slice of same type", func(t *testing.T) {
		var qux = "1;23;4"
		duck, err := convkit.DuckParse(qux, convkit.Options{Separator: ";"})
		assert.NoError(t, err)
		assert.Equal[any](t, []int{1, 23, 4}, duck)
	})
	t.Run("slice of various type", func(t *testing.T) {
		var qux = "1;foo;true"
		duck, err := convkit.DuckParse(qux, convkit.Options{Separator: ";"})
		assert.NoError(t, err)
		assert.Equal[any](t, []any{1, "foo", true}, duck)
	})
	t.Run("time.Time", func(t *testing.T) {
		var (
			refTime = rnd.Time()
			layout  = time.RFC3339
			quux    = refTime.Format(layout)
		)
		duck, err := convkit.DuckParse(quux, convkit.Options{TimeLayout: layout})
		assert.NoError(t, err)
		timeDuck, ok := duck.(time.Time)
		assert.True(t, ok, "expected to have time.Time result from DuckParse")
		assert.Equal[any](t, timeDuck, refTime)
	})
	t.Run("json", func(t *testing.T) {
		var corge = map[string]any{
			"a": "foo",
			"b": float64(42),
			"c": float64(42.24),
			"d": true,
		}
		corgeData, err := json.Marshal(corge)
		assert.NoError(t, err)
		duck, err := convkit.DuckParse(corgeData)
		assert.NoError(t, err)
		assert.Equal[any](t, duck, corge)
	})
	t.Run("time.Duration", func(t *testing.T) {
		duration := time.Duration(rnd.IntBetween(int(time.Second), int(time.Hour)))
		got, err := convkit.DuckParse(duration.String())
		assert.NoError(t, err)
		assert.Equal[any](t, duration, got)
	})
}

type ValueWithTextMarshaler struct{ Data []byte }

func (v ValueWithTextMarshaler) MarshalText() (text []byte, err error) {
	return v.Data, nil
}

func (v *ValueWithTextMarshaler) UnmarshalText(text []byte) error {
	v.Data = text
	return nil
}

type ValueWithTextMarshalerErr struct{ Err error }

func (v ValueWithTextMarshalerErr) MarshalText() (text []byte, err error) {
	return []byte{}, v.Err
}

func (v *ValueWithTextMarshalerErr) UnmarshalText(text []byte) error {
	return errors.New(string(text))
}

func Test_textMarshalerIntegration(t *testing.T) {
	s := testcase.NewSpec(t)

	s.Test("happy", func(t *testcase.T) {
		var v = ValueWithTextMarshaler{
			Data: []byte(rnd.HexN(5)),
		}

		data, err := convkit.Format(v)
		assert.NoError(t, err)
		assert.Equal(t, []byte(data), v.Data)

		got, err := convkit.Parse[ValueWithTextMarshaler](data)
		assert.NoError(t, err)
		assert.Equal(t, v, got)

		rgot, err := convkit.ParseReflect(reflectkit.TypeOf[ValueWithTextMarshaler](), data)
		assert.NoError(t, err)
		assert.Equal(t, v, rgot.Interface().(ValueWithTextMarshaler))
	})

	s.Test("rainy", func(t *testcase.T) {
		var v = ValueWithTextMarshalerErr{
			Err: t.Random.Error(),
		}

		_, err := convkit.Format(v)
		assert.ErrorIs(t, err, v.Err)

		var data = t.Random.HexN(8)
		_, err = convkit.Parse[ValueWithTextMarshalerErr](data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), data)

		_, err = convkit.ParseReflect(reflectkit.TypeOf[ValueWithTextMarshalerErr](), data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), data)
	})
}

func TestFormatReflect(t *testing.T) {
	var (
		foo = "forty-two"
		bar = "42"
		baz = "42.42"
		qux = "1;23;4"

		refTime = rnd.Time()
		layout  = time.RFC3339
		quux    = refTime.Format(layout)
	)

	// Test string type
	strval := reflect.ValueOf(foo)
	gotStr, err := convkit.FormatReflect(strval)
	assert.NoError(t, err)
	assert.Equal(t, foo, gotStr)

	// Test int type
	intval := reflect.ValueOf(42)
	gotInt, err := convkit.FormatReflect(intval)
	assert.NoError(t, err)
	assert.Equal(t, bar, gotInt)

	// Test float64 type
	fltval := reflect.ValueOf(42.42)
	gotFloat, err := convkit.FormatReflect(fltval)
	assert.NoError(t, err)
	assert.Equal(t, baz, gotFloat)

	// Test uint type
	uintval := reflect.ValueOf(uint(42))
	gotUint, err := convkit.FormatReflect(uintval)
	assert.NoError(t, err)
	assert.Equal(t, bar, gotUint)

	// Test []int type with separator
	vals := reflect.ValueOf([]int{1, 23, 4})
	gotVals, err := convkit.FormatReflect(vals, convkit.Options{Separator: ";"})
	assert.NoError(t, err)
	assert.Equal(t, qux, gotVals)

	// Test time.Time type with layout
	timeval := reflect.ValueOf(refTime)
	gotTimeval, err := convkit.FormatReflect(timeval, convkit.Options{TimeLayout: layout})
	assert.NoError(t, err)
	assert.Equal(t, quux, gotTimeval)

	// Test bool type
	boolval := reflect.ValueOf(true)
	gotBool, err := convkit.FormatReflect(boolval)
	assert.NoError(t, err)
	assert.Equal(t, "true", gotBool)

	// Test nil pointer (should return empty string)
	var nilPtr *string
	nilVal := reflect.ValueOf(nilPtr)
	gotNil, err := convkit.FormatReflect(nilVal)
	assert.NoError(t, err)
	assert.Empty(t, gotNil)

	// Test slice of different types (as interface{})
	var mixedSlice []interface{}
	mixedSlice = append(mixedSlice, 1, "foo", true)
	mixedVal := reflect.ValueOf(mixedSlice)
	gotMixed, err := convkit.FormatReflect(mixedVal, convkit.Options{Separator: ";"})
	assert.NoError(t, err)
	assert.Equal(t, "1;foo;true", gotMixed)

	// Test time.Duration
	duration := reflect.ValueOf(time.Minute + 5*time.Second)
	gotDuration, err := convkit.FormatReflect(duration)
	assert.NoError(t, err)
	assert.Equal(t, "1m5s", gotDuration)

	// Test map
	testMap := map[string]int{"a": 42, "b": 100}
	mapVal := reflect.ValueOf(testMap)
	gotMap, err := convkit.FormatReflect(mapVal)
	assert.NoError(t, err)
	// For map, Format should use JSON serialization since it's complex
	var expectedMap string
	expectedBytes, _ := json.Marshal(testMap)
	expectedMap = string(expectedBytes)
	assert.Equal(t, expectedMap, gotMap)

	// Test struct
	type Person struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	unreg := convkit.Register[Person](ImplT[Person]{
		MarshalFunc: func(v Person) ([]byte, error) {
			return json.Marshal(v)
		},
		UnmarshalFunc: func(data []byte, p *Person) error {
			return json.Unmarshal(data, p)
		},
	})
	t.Cleanup(unreg)

	person := Person{Name: "John", Age: 30}
	personVal := reflect.ValueOf(person)
	gotPerson, err := convkit.FormatReflect(personVal)
	assert.NoError(t, err)
	expectedPersonBytes, _ := json.Marshal(person)
	expectedPerson := string(expectedPersonBytes)
	assert.Equal(t, expectedPerson, gotPerson)

	// Test with custom text marshaler
	textMarshaler := ValueWithTextMarshaler{
		Data: []byte(rnd.HexN(5)),
	}
	marshalerVal := reflect.ValueOf(textMarshaler)
	gotText, err := convkit.FormatReflect(marshalerVal)
	assert.NoError(t, err)
	assert.Equal(t, string(textMarshaler.Data), gotText)

	// Test error case with custom text marshaler
	textMarshalerErr := ValueWithTextMarshalerErr{
		Err: errors.New("test error"),
	}
	marshalerErrVal := reflect.ValueOf(textMarshalerErr)
	_, err = convkit.FormatReflect(marshalerErrVal)
	assert.Error(t, err)
}

func TestMarshal(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		data, err := convkit.Marshal("")
		assert.NoError(t, err)
		assert.Equal(t, []byte(""), data)
	})

	t.Run("string value", func(t *testing.T) {
		data, err := convkit.Marshal("forty-two")
		assert.NoError(t, err)
		assert.Equal(t, []byte("forty-two"), data)
	})

	t.Run("int value", func(t *testing.T) {
		data, err := convkit.Marshal(42)
		assert.NoError(t, err)
		assert.Equal(t, []byte("42"), data)
	})

	t.Run("float64 value", func(t *testing.T) {
		data, err := convkit.Marshal(42.42)
		assert.NoError(t, err)
		assert.Equal(t, []byte("42.42"), data)
	})

	t.Run("bool value", func(t *testing.T) {
		data, err := convkit.Marshal(true)
		assert.NoError(t, err)
		assert.Equal(t, []byte("true"), data)
	})

	t.Run("uint value", func(t *testing.T) {
		data, err := convkit.Marshal(uint(42))
		assert.NoError(t, err)
		assert.Equal(t, []byte("42"), data)
	})

	t.Run("pointer to int", func(t *testing.T) {
		val := 42
		data, err := convkit.Marshal(&val)
		assert.NoError(t, err)
		assert.Equal(t, []byte("42"), data)
	})

	t.Run("nil pointer", func(t *testing.T) {
		var nilPtr *int
		data, err := convkit.Marshal(nilPtr)
		assert.NoError(t, err)
		assert.Nil(t, data)
	})

	t.Run("slice of ints with separator", func(t *testing.T) {
		vals := []int{1, 23, 4}
		data, err := convkit.Marshal(vals, convkit.Options{Separator: ";"})
		assert.NoError(t, err)
		assert.Equal(t, []byte("1;23;4"), data)
	})

	t.Run("slice of ints without separator (JSON)", func(t *testing.T) {
		vals := []int{1, 23, 4}
		data, err := convkit.Marshal(vals)
		assert.NoError(t, err)
		assert.Equal(t, []byte("[1,23,4]"), data)
	})

	t.Run("slice of strings with separator", func(t *testing.T) {
		vals := []string{"foo", "bar", "baz"}
		data, err := convkit.Marshal(vals, convkit.Options{Separator: ","})
		assert.NoError(t, err)
		assert.Equal(t, []byte("foo,bar,baz"), data)
	})

	t.Run("slice with escaped separator", func(t *testing.T) {
		vals := []string{"Hello, world!", "foo", "bar"}
		data, err := convkit.Marshal(vals, convkit.Options{Separator: ","})
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello\\, world!,foo,bar"), data)
	})

	t.Run("map value (JSON)", func(t *testing.T) {
		m := map[string]int{"a": 42, "b": 100}
		data, err := convkit.Marshal(m)
		assert.NoError(t, err)

		var expected map[string]int
		err = json.Unmarshal(data, &expected)
		assert.NoError(t, err)
		assert.Equal(t, m, expected)
	})

	t.Run("time.Time with layout", func(t *testing.T) {
		refTime := rnd.Time()
		layout := time.RFC3339
		data, err := convkit.Marshal(refTime, convkit.Options{TimeLayout: layout})
		assert.NoError(t, err)
		assert.Equal(t, []byte(refTime.Format(layout)), data)
	})

	// t.Run("time.Time without layout", func(t *testing.T) {
	// 	refTime := rnd.Time()
	// 	_, err := convkit.Marshal(refTime)
	// 	assert.ErrorIs(t, err, errMissingTimeLayout)
	// })

	t.Run("time.Duration", func(t *testing.T) {
		duration := time.Minute + 5*time.Second
		data, err := convkit.Marshal(duration)
		assert.NoError(t, err)
		assert.Equal(t, []byte("1m5s"), data)
	})

	t.Run("url.URL", func(t *testing.T) {
		u, _ := url.ParseRequestURI("https://go.llib.dev/frameless")
		data, err := convkit.Marshal(*u)
		assert.NoError(t, err)
		assert.Equal(t, []byte(u.String()), data)
	})

	t.Run("pointer to url.URL", func(t *testing.T) {
		u, _ := url.ParseRequestURI("https://go.llib.dev/frameless")
		data, err := convkit.Marshal(u)
		assert.NoError(t, err)
		assert.Equal(t, []byte(u.String()), data)
	})

	t.Run("custom registered type", func(t *testing.T) {
		type X struct{ Value string }
		unreg := convkit.Register[X](ImplT[X]{
			MarshalFunc: func(v X) ([]byte, error) {
				return []byte("X{" + v.Value + "}"), nil
			},
			UnmarshalFunc: func(data []byte, p *X) error {
				if !bytes.HasPrefix(data, []byte("X{")) || !bytes.HasSuffix(data, []byte("}")) {
					return fmt.Errorf("invalid format")
				}
				p.Value = string(data[2 : len(data)-1])
				return nil
			},
		})
		defer unreg()

		x := X{Value: "test"}
		data, err := convkit.Marshal(x)
		assert.NoError(t, err)
		assert.Equal(t, []byte("X{test}"), data)
	})

	t.Run("value with TextMarshaler", func(t *testing.T) {
		v := ValueWithTextMarshaler{
			Data: []byte("custom-data"),
		}
		data, err := convkit.Marshal(v)
		assert.NoError(t, err)
		assert.Equal(t, []byte("custom-data"), data)
	})

	t.Run("value with TextMarshaler error", func(t *testing.T) {
		v := ValueWithTextMarshalerErr{
			Err: errors.New("test error"),
		}
		_, err := convkit.Marshal(v)
		assert.ErrorIs(t, err, v.Err)
	})

	t.Run("unknown type", func(t *testing.T) {
		type Unknown struct{ Field string }
		_, err := convkit.Marshal(Unknown{Field: "test"})
		assert.Error(t, err)
	})
}

func TestMarshalReflect(t *testing.T) {
	t.Run("string value", func(t *testing.T) {
		val := reflect.ValueOf("forty-two")
		data, err := convkit.MarshalReflect(val)
		assert.NoError(t, err)
		assert.Equal(t, []byte("forty-two"), data)
	})

	t.Run("int value", func(t *testing.T) {
		val := reflect.ValueOf(42)
		data, err := convkit.MarshalReflect(val)
		assert.NoError(t, err)
		assert.Equal(t, []byte("42"), data)
	})

	t.Run("slice of ints with separator", func(t *testing.T) {
		vals := []int{1, 23, 4}
		val := reflect.ValueOf(vals)
		data, err := convkit.MarshalReflect(val, convkit.Options{Separator: ";"})
		assert.NoError(t, err)
		assert.Equal(t, []byte("1;23;4"), data)
	})

	t.Run("time.Time with layout", func(t *testing.T) {
		refTime := rnd.Time()
		layout := time.RFC3339
		val := reflect.ValueOf(refTime)
		data, err := convkit.MarshalReflect(val, convkit.Options{TimeLayout: layout})
		assert.NoError(t, err)
		assert.Equal(t, []byte(refTime.Format(layout)), data)
	})

	t.Run("nil pointer", func(t *testing.T) {
		var nilPtr *int
		val := reflect.ValueOf(nilPtr)
		data, err := convkit.MarshalReflect(val)
		assert.NoError(t, err)
		assert.Empty(t, data)
	})
}

func TestUnmarshal(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		var val string
		assert.NoError(t, convkit.Unmarshal([]byte(""), &val))
		assert.Empty(t, val)
	})

	t.Run("string value", func(t *testing.T) {
		var val string
		assert.NoError(t, convkit.Unmarshal([]byte("forty-two"), &val))
		assert.Equal(t, "forty-two", val)
	})

	t.Run("int value", func(t *testing.T) {
		var val int
		assert.NoError(t, convkit.Unmarshal([]byte("42"), &val))
		assert.Equal(t, 42, val)
	})

	t.Run("float64 value", func(t *testing.T) {
		var val float64
		assert.NoError(t, convkit.Unmarshal([]byte("42.42"), &val))
		assert.Equal(t, 42.42, val)
	})

	t.Run("bool value", func(t *testing.T) {
		var val bool
		assert.NoError(t, convkit.Unmarshal([]byte("true"), &val))
		assert.True(t, val)
	})

	t.Run("uint value", func(t *testing.T) {
		var val uint
		assert.NoError(t, convkit.Unmarshal([]byte("42"), &val))
		assert.Equal(t, uint(42), val)
	})

	t.Run("slice of ints with separator", func(t *testing.T) {
		var vals []int
		assert.NoError(t, convkit.Unmarshal([]byte("1;23;4"), &vals, convkit.Options{Separator: ";"}))
		assert.Equal(t, []int{1, 23, 4}, vals)
	})

	t.Run("slice of ints without separator (JSON)", func(t *testing.T) {
		var vals []int
		assert.NoError(t, convkit.Unmarshal([]byte("[1,23,4]"), &vals))
		assert.Equal(t, []int{1, 23, 4}, vals)
	})

	t.Run("slice of strings with separator", func(t *testing.T) {
		var vals []string
		assert.NoError(t, convkit.Unmarshal([]byte("foo,bar,baz"), &vals, convkit.Options{Separator: ","}))
		assert.Equal(t, []string{"foo", "bar", "baz"}, vals)
	})

	t.Run("slice with escaped separator", func(t *testing.T) {
		var vals []string
		assert.NoError(t, convkit.Unmarshal([]byte("Hello\\, world!,foo,bar"), &vals, convkit.Options{Separator: ","}))
		assert.Equal(t, []string{"Hello, world!", "foo", "bar"}, vals)
	})

	t.Run("map value (JSON)", func(t *testing.T) {
		var m map[string]int
		assert.NoError(t, convkit.Unmarshal([]byte(`{"a":42,"b":100}`), &m))
		assert.Equal(t, map[string]int{"a": 42, "b": 100}, m)
	})

	t.Run("time.Time with layout", func(t *testing.T) {
		var val time.Time
		refTime := rnd.Time()
		layout := time.RFC3339
		assert.NoError(t, convkit.Unmarshal([]byte(refTime.Format(layout)), &val, convkit.Options{TimeLayout: layout}))
		assert.True(t, val.Equal(refTime))
	})

	// t.Run("time.Time without layout", func(t *testing.T) {
	// 	var val time.Time
	// 	assert.ErrorIs(t, convkit.Unmarshal([]byte(time.Now().Format(time.RFC3339)), &val), errMissingTimeLayout)
	// })

	t.Run("time.Duration", func(t *testing.T) {
		var duration time.Duration
		assert.NoError(t, convkit.Unmarshal([]byte("1m5s"), &duration))
		assert.Equal(t, time.Minute+5*time.Second, duration)
	})

	t.Run("url.URL", func(t *testing.T) {
		var u url.URL
		assert.NoError(t, convkit.Unmarshal([]byte("https://go.llib.dev/frameless"), &u))
		assert.Equal(t, "https://go.llib.dev/frameless", u.String())
	})

	t.Run("pointer to url.URL", func(t *testing.T) {
		var u *url.URL
		assert.NoError(t, convkit.Unmarshal([]byte("https://go.llib.dev/frameless"), &u))
		assert.NotNil(t, u)
		assert.Equal(t, "https://go.llib.dev/frameless", u.String())
	})

	t.Run("custom registered type", func(t *testing.T) {
		type X struct{ Value string }
		unreg := convkit.Register[X](ImplT[X]{
			MarshalFunc: func(v X) ([]byte, error) {
				return []byte("X{" + v.Value + "}"), nil
			},
			UnmarshalFunc: func(data []byte, p *X) error {
				if !bytes.HasPrefix(data, []byte("X{")) || !bytes.HasSuffix(data, []byte("}")) {
					return fmt.Errorf("invalid format")
				}
				p.Value = string(data[2 : len(data)-1])
				return nil
			},
		})
		defer unreg()

		var x X
		assert.NoError(t, convkit.Unmarshal([]byte("X{test}"), &x))
		assert.Equal(t, "test", x.Value)
	})

	t.Run("value with TextUnmarshaler", func(t *testing.T) {
		var v ValueWithTextMarshaler
		assert.NoError(t, convkit.Unmarshal([]byte("custom-data"), &v))
		assert.Equal(t, []byte("custom-data"), v.Data)
	})

	t.Run("value with TextUnmarshaler error", func(t *testing.T) {
		var v ValueWithTextMarshalerErr
		assert.Error(t, convkit.Unmarshal([]byte("test-error"), &v))
	})

	t.Run("unknown type", func(t *testing.T) {
		type Unknown struct{ Field string }
		var u Unknown
		assert.Error(t, convkit.Unmarshal([]byte("test"), &u))
	})

	t.Run("invalid input for type", func(t *testing.T) {
		var val int
		assert.Error(t, convkit.Unmarshal([]byte("not-a-number"), &val))
	})
}

func TestUnmarshalReflect(t *testing.T) {
	t.Run("string value", func(t *testing.T) {
		typ := reflectkit.TypeOf[string]()
		ptr := reflect.New(typ)
		assert.NoError(t, convkit.UnmarshalReflect(typ, []byte("forty-two"), ptr))
		assert.Equal(t, "forty-two", ptr.Elem().Interface())
	})

	t.Run("int value", func(t *testing.T) {
		typ := reflectkit.TypeOf[int]()
		ptr := reflect.New(typ)
		assert.NoError(t, convkit.UnmarshalReflect(typ, []byte("42"), ptr))
		assert.Equal(t, 42, ptr.Elem().Interface())
	})

	t.Run("slice of ints with separator", func(t *testing.T) {
		typ := reflectkit.TypeOf[[]int]()
		ptr := reflect.New(typ)
		assert.NoError(t, convkit.UnmarshalReflect(typ, []byte("1;23;4"), ptr, convkit.Options{Separator: ";"}))
		assert.Equal(t, []int{1, 23, 4}, ptr.Elem().Interface().([]int))
	})

	t.Run("time.Time with layout", func(t *testing.T) {
		typ := reflectkit.TypeOf[time.Time]()
		ptr := reflect.New(typ)
		refTime := rnd.Time()
		layout := time.RFC3339
		assert.NoError(t, convkit.UnmarshalReflect(typ, []byte(refTime.Format(layout)), ptr, convkit.Options{TimeLayout: layout}))
		assert.True(t, ptr.Elem().Interface().(time.Time).Equal(refTime))
	})

	t.Run("invalid input for type", func(t *testing.T) {
		typ := reflectkit.TypeOf[int]()
		ptr := reflect.New(typ)
		assert.Error(t, convkit.UnmarshalReflect(typ, []byte("not-a-number"), ptr))
	})

	t.Run("empty interface type", func(t *testing.T) {
		typ := reflectkit.TypeOf[any]()
		ptr := reflect.New(typ)

		assert.NoError(t, convkit.UnmarshalReflect(typ, []byte("42"), ptr))
		assert.Equal[any](t, 42, ptr.Elem().Interface())

		assert.NoError(t, convkit.UnmarshalReflect(typ, []byte(`"hello"`), ptr))
		assert.Equal[any](t, "hello", ptr.Elem().Interface())
	})

	t.Run("empty interface but concrete ptr type", func(t *testing.T) {
		typ := reflectkit.TypeOf[any]()
		ptr := reflect.New(reflectkit.TypeOf[int]())

		assert.NoError(t, convkit.UnmarshalReflect(typ, []byte("42"), ptr))
		assert.Equal[any](t, 42, ptr.Elem().Interface())

		assert.Error(t, convkit.UnmarshalReflect(typ, []byte(`"hello"`), ptr))
	})

	t.Run("non-pointer type for ptr argument", func(t *testing.T) {
		typ := reflectkit.TypeOf[any]()
		ptr := reflect.ValueOf(0)

		assert.Error(t, convkit.UnmarshalReflect(typ, []byte("42"), ptr))
	})
}

func ExampleUnmarshal_basicType() {
	var num int
	convkit.Unmarshal([]byte("42"), &num)
	fmt.Println(num) // Output: 42
}

func ExampleUnmarshal_sliceWithSeparator() {
	var nums []int
	convkit.Unmarshal([]byte("1;2;3"), &nums, convkit.Options{Separator: ";"})
	fmt.Println(nums) // Output: [1 2 3]
}

func ExampleUnmarshal_json() {
	var data map[string]interface{}
	convkit.Unmarshal([]byte(`{"key":"value"}`), &data)
	fmt.Println(data["key"]) // Output: value
}

func ExampleUnmarshal_timeWithLayout() {
	var t time.Time
	layout := "2006-01-02"
	convkit.Unmarshal([]byte("2023-05-15"), &t, convkit.Options{TimeLayout: layout})
	fmt.Println(t.Format(layout)) // Output: 2023-05-15
}

func ExampleUnmarshal_url() {
	var url url.URL
	convkit.Unmarshal([]byte("https://example.com/path?query=value"), &url)
	fmt.Println(url.String()) // Output: https://example.com/path?query=value
}

func ExampleUnmarshal_customType() {
	type Config struct {
		Host string
		Port int
	}
	var cfg Config
	convkit.Unmarshal([]byte("host=localhost,port=8080"), &cfg,
		convkit.UnmarshalWith(func(data []byte, c *Config) error {
			parts := strings.SplitN(string(data), ",", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid format")
			}
			for _, part := range parts {
				kv := strings.SplitN(part, "=", 2)
				if len(kv) != 2 {
					continue
				}
				switch kv[0] {
				case "host":
					c.Host = kv[1]
				case "port":
					var err error
					c.Port, err = strconv.Atoi(kv[1])
					if err != nil {
						return fmt.Errorf("invalid port: %w", err)
					}
				}
			}
			return nil
		}))
	fmt.Printf("%s:%d\n", cfg.Host, cfg.Port) // Output: localhost:8080
}

func ExampleUnmarshalReflect() {
	typ := reflectkit.TypeOf[int]()
	ptr := reflect.New(typ)
	convkit.UnmarshalReflect(typ, []byte("42"), ptr)
	fmt.Println(ptr.Elem().Interface()) // Output: 42
}

func ExampleUnmarshalReflect_sliceWithSeparator() {
	typ := reflectkit.TypeOf[[]int]()
	ptr := reflect.New(typ)
	convkit.UnmarshalReflect(typ, []byte("1;2;3"), ptr, convkit.Options{Separator: ";"})
	fmt.Println(ptr.Elem().Interface()) // Output: [1 2 3]
}

func ExampleUnmarshalReflect_timeWithLayout() {
	typ := reflectkit.TypeOf[time.Time]()
	ptr := reflect.New(typ)
	layout := "2006-01-02"
	convkit.UnmarshalReflect(typ, []byte("2023-05-15"), ptr, convkit.Options{TimeLayout: layout})
	fmt.Println(ptr.Elem().Interface().(time.Time).Format(layout)) // Output: 2023-05-15
}
