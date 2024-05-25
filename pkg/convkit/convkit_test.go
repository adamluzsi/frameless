package convkit_test

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/reflectkit"
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

func TestIsRegistered(t *testing.T) {
	assert.False(t, convkit.IsRegistered[string]())
	assert.True(t, convkit.IsRegistered[time.Time]())
	assert.True(t, convkit.IsRegistered[*time.Time]())
	assert.True(t, convkit.IsRegistered(time.Now()))

	type X struct{}
	assert.False(t, convkit.IsRegistered[X]())
	undo := convkit.Register[X](func(data string) (X, error) {
		if data != "X{}" {
			return X{}, fmt.Errorf("not X")
		}
		return X{}, nil
	}, func(x X) (string, error) {
		return "X{}", nil
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

func ExampleParseWith() {
	// export FOO=foo:baz
	type Conf struct {
		Foo string
		Bar string
	}
	parserFunc := func(v string) (Conf, error) {
		parts := strings.SplitN(v, ":", 1)
		if len(parts) != 2 {
			return Conf{}, fmt.Errorf("invalid format")
		}
		return Conf{
			Foo: parts[0],
			Bar: parts[1],
		}, nil
	}
	conf, err := convkit.Parse[Conf]("foo:bar", convkit.ParseWith(parserFunc))
	_, _ = conf, err
}

func TestParseWith(t *testing.T) {
	type Conf struct {
		Foo string
		Bar string
	}
	t.Run("happy", func(t *testing.T) {
		conf, err := convkit.Parse[Conf]("foo:bar",
			convkit.ParseWith(func(v string) (Conf, error) {
				var c Conf
				parts := strings.SplitN(v, ":", 2)
				if len(parts) != 2 {
					return c, fmt.Errorf("invalid format")
				}
				c.Foo = parts[0]
				c.Bar = parts[1]
				return c, nil
			}))
		assert.Equal(t, conf, Conf{Foo: "foo", Bar: "bar"})
		assert.NoError(t, err)
	})
	t.Run("rainy", func(t *testing.T) {
		expErr := rnd.Error()
		conf, err := convkit.Parse[Conf]("whatever", convkit.ParseWith(func(v string) (Conf, error) {
			return Conf{}, expErr
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
