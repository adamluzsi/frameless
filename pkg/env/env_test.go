package env_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/must"

	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/random"
)

var rnd = random.New(random.CryptoSeed{})

const (
	envKey    = "THE_ENV_KEY"
	othEnvKey = "OTH_ENV_KEY"
)

func TestLoad(t *testing.T) {
	t.Run("on nil value", func(t *testing.T) {
		type Example struct{}
		assert.Error(t, env.Load[Example](nil))
	})

	t.Run("on non-struct type", func(t *testing.T) {
		var c string
		assert.Error(t, env.Load(&c))
	})

	t.Run("struct struct fields without env tag are ignored", func(t *testing.T) {
		type Example struct{ V string }
		var c Example
		assert.NoError(t, env.Load(&c))
		assert.Empty(t, c)
	})

	t.Run("string struct field", func(t *testing.T) {
		type Example struct {
			V string `env:"THE_ENV_KEY"`
		}
		t.Run("os env has the value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, "42")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "42", c.V)
		})
		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})
	})

	loadStructFieldTypeTestCase[int](t, 42, "42", "forty-two")
	type intType int
	loadStructFieldTypeTestCase[intType](t, 42, "42", "forty-two")

	loadStructFieldTypeTestCase[int8](t, 42, "42", "forty-two")
	type int8Type int8
	loadStructFieldTypeTestCase[int8Type](t, 42, "42", "forty-two")

	loadStructFieldTypeTestCase[int16](t, 42, "42", "forty-two")
	type int16Type int16
	loadStructFieldTypeTestCase[int16Type](t, 42, "42", "forty-two")

	loadStructFieldTypeTestCase[int32](t, 42, "42", "forty-two")
	type int32Type int32
	loadStructFieldTypeTestCase[int32Type](t, 42, "42", "forty-two")

	loadStructFieldTypeTestCase[int64](t, 42, "42", "forty-two")
	type int64Type int64
	loadStructFieldTypeTestCase[int64Type](t, 42, "42", "forty-two")

	loadStructFieldTypeTestCase[float32](t, 42.42, "42.42", "forty-two")
	type float32Type float32
	loadStructFieldTypeTestCase[float32Type](t, 42.42, "42.42", "forty-two")

	loadStructFieldTypeTestCase[float64](t, 42.42, "42.42", "forty-two")
	type float64Type float64
	loadStructFieldTypeTestCase[float64Type](t, 42.42, "42.42", "forty-two")

	loadStructFieldTypeTestCase[bool](t, true, "true", "sure")
	type boolType bool
	loadStructFieldTypeTestCase[boolType](t, true, "t", "sure")

	t.Run("url package integration", testLoadUrlPackageIntegration)

	t.Run("time package integration", testLoadTimePackageIntegration)

	t.Run("a struct field without tag will be visited", func(t *testing.T) {
		type Example struct {
			V struct {
				F string `env:"THE_ENV_KEY"`
			}
		}
		t.Run("os env has the value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, "42")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "42", c.V.F)
		})
		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})
	})

	t.Run("a field with env tag that specifies multiple env key", func(t *testing.T) {
		type Example struct {
			F string `env:"THE_ENV_KEY_1, THE_ENV_KEY_2"`
		}
		t.Run("os env has the value", func(t *testing.T) {
			testcase.SetEnv(t, "THE_ENV_KEY_1", "42")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "42", c.F)
		})
		t.Run("os env has the value under the second key", func(t *testing.T) {
			testcase.SetEnv(t, "THE_ENV_KEY_2", "24")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "24", c.F)
		})
		t.Run("os env has the value under both keys then first is prioritised", func(t *testing.T) {
			testcase.SetEnv(t, "THE_ENV_KEY_1", "42")
			testcase.SetEnv(t, "THE_ENV_KEY_2", "24")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "42", c.F)
		})
		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})
	})

	t.Run("unexported fields are ignored in a struct field", func(t *testing.T) {
		type MyStruct struct {
			unexported *int
			Exported   string
		}
		type Example struct {
			V MyStruct
		}
		var c Example
		assert.NoError(t, env.Load(&c))
		assert.Empty(t, c)
		assert.Nil(t, c.V.unexported)
	})

	t.Run("map struct field", func(t *testing.T) {
		type Example struct {
			V map[string]int `env:"THE_ENV_KEY"`
		}
		t.Run("os env has the value", func(t *testing.T) {
			ref := map[string]int{"the answer is": 42}
			bs, err := json.Marshal(ref)
			assert.NoError(t, err)
			testcase.SetEnv(t, envKey, string(bs))
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, ref, c.V)
		})

		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})
	})

	t.Run("when default tag is supplied, its value is used in case of the absence of a env variable", func(t *testing.T) {
		type Example struct {
			V string `env:"THE_ENV_KEY" default:"the default is 42"`
			O string `env:"OTH_ENV_KEY" env-default:"oth default value" default:"this is ignored when prefixed tag key is present"`
		}
		t.Run("os env has the value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, "42")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "42", c.V)
		})
		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "the default is 42", c.V)
			assert.Equal(t, "oth default value", c.O)
		})
	})

	t.Run("when required tag is supplied, its value is used in case of the absence of a env variable", func(t *testing.T) {
		type Example struct {
			V string `env:"V_KEY" required:"true"`
			B string `env:"B_KEY" required:"false"`
			N string `env:"N_KEY" required:"true" default:"fallback value"`
			M string `env:"M_KEY" env-required:"true" required:"false"` // in case of prefix, other value is ignored
		}

		t.Run("os env has the value", func(t *testing.T) {
			testcase.SetEnv(t, "V_KEY", "V")
			testcase.SetEnv(t, "B_KEY", "B")
			testcase.SetEnv(t, "N_KEY", "N")
			testcase.SetEnv(t, "M_KEY", "M")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "V", c.V)
			assert.Equal(t, "B", c.B)
			assert.Equal(t, "N", c.N)
			assert.Equal(t, "M", c.M)
		})
		t.Run("os env has the value for the must required fields only", func(t *testing.T) {
			testcase.SetEnv(t, "V_KEY", "V")
			testcase.UnsetEnv(t, "B_KEY")
			testcase.UnsetEnv(t, "N_KEY")
			testcase.UnsetEnv(t, "N_KEY")
			testcase.SetEnv(t, "M_KEY", "M")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "V", c.V)
			assert.Empty(t, c.B)
			assert.Equal(t, "fallback value", c.N)
			assert.Equal(t, "M", c.M)
		})
		t.Run("os env doesn't have the required env variable values", func(t *testing.T) {
			testcase.SetEnv(t, "V_KEY", "V")
			testcase.UnsetEnv(t, "B_KEY")
			testcase.UnsetEnv(t, "N_KEY")
			testcase.UnsetEnv(t, "N_KEY")
			testcase.SetEnv(t, "M_KEY", "M")
			if random.New(random.CryptoSeed{}).Bool() {
				testcase.UnsetEnv(t, "V_KEY")
			} else {
				testcase.UnsetEnv(t, "M_KEY")
			}
			var c Example
			assert.Error(t, env.Load(&c))
		})

		t.Run("with defined `require`/`env-require` tag name", func(t *testing.T) {
			type Example struct {
				V string `env:"V_KEY" require:"true"`
				B string `env:"B_KEY" require:"false"`
				N string `env:"N_KEY" require:"true" default:"fallback value"`
				M string `env:"M_KEY" env-require:"true" require:"false"` // in case of prefix, other value is ignored
			}

			t.Run("os env has the value", func(t *testing.T) {
				testcase.SetEnv(t, "V_KEY", "V")
				testcase.SetEnv(t, "B_KEY", "B")
				testcase.SetEnv(t, "N_KEY", "N")
				testcase.SetEnv(t, "M_KEY", "M")
				var c Example
				assert.NoError(t, env.Load(&c))
				assert.NotEmpty(t, c)
				assert.Equal(t, "V", c.V)
				assert.Equal(t, "B", c.B)
				assert.Equal(t, "N", c.N)
				assert.Equal(t, "M", c.M)
			})
			t.Run("os env has the value for the must required fields only", func(t *testing.T) {
				testcase.SetEnv(t, "V_KEY", "V")
				testcase.UnsetEnv(t, "B_KEY")
				testcase.UnsetEnv(t, "N_KEY")
				testcase.UnsetEnv(t, "N_KEY")
				testcase.SetEnv(t, "M_KEY", "M")
				var c Example
				assert.NoError(t, env.Load(&c))
				assert.NotEmpty(t, c)
				assert.Equal(t, "V", c.V)
				assert.Empty(t, c.B)
				assert.Equal(t, "fallback value", c.N)
				assert.Equal(t, "M", c.M)
			})
			t.Run("os env doesn't have the required env variable values", func(t *testing.T) {
				testcase.SetEnv(t, "V_KEY", "V")
				testcase.UnsetEnv(t, "B_KEY")
				testcase.UnsetEnv(t, "N_KEY")
				testcase.UnsetEnv(t, "N_KEY")
				testcase.SetEnv(t, "M_KEY", "M")
				if random.New(random.CryptoSeed{}).Bool() {
					testcase.UnsetEnv(t, "V_KEY")
				} else {
					testcase.UnsetEnv(t, "M_KEY")
				}
				var c Example
				assert.Error(t, env.Load(&c))
			})
		})
	})

	t.Run("when env-separator tag is supplied", func(t *testing.T) {
		type Example struct {
			V []int `env:"V_KEY" env-separator:":"`
			B []int `env:"B_KEY"`
		}

		t.Run("os env has the value", func(t *testing.T) {
			testcase.SetEnv(t, "V_KEY", "1:2:4:3")
			testcase.SetEnv(t, "B_KEY", "3,2,1")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, []int{1, 2, 4, 3}, c.V)
			assert.Equal(t, []int{3, 2, 1}, c.B)
		})

		t.Run("os env doesn't have the values", func(t *testing.T) {
			testcase.UnsetEnv(t, "V_KEY")
			testcase.UnsetEnv(t, "B_KEY")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})

		t.Run("with both `separator` and `env-separator` are defined", func(t *testing.T) {
			type Example struct {
				V []string `env:"V_KEY" env-separator:"|" separator:";"`
			}

			t.Run("then the prefixed tag is preferred over the non-prefixed variant", func(t *testing.T) {
				testcase.SetEnv(t, "V_KEY", "A|B|C|D")
				var c Example
				assert.NoError(t, env.Load(&c))
				assert.NotEmpty(t, c)
				assert.Equal(t, []string{"A", "B", "C", "D"}, c.V)
			})
		})
	})

	t.Run("integrates with enum package", func(t *testing.T) {
		type Example struct {
			V string `env:"THE_ENV_KEY" enum:"A;B;C;"`
			B string `env:"B" enum:"foo;bar;baz;" default:"bar"`
		}
		t.Run("os env has a valid value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, "A")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "A", c.V)
			assert.Equal(t, "bar", c.B)
		})
		t.Run("os env has an invalid value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, "D")
			var c Example
			assert.ErrorIs(t, enum.ErrInvalid, env.Load(&c))
		})
		t.Run("os env doesn't have the value but the config object already had it", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			c.V = "B"
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, "B", c.V)
		})
		t.Run("os env doesn't have the value and the config object doesn't have it", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			assert.ErrorIs(t, enum.ErrInvalid, env.Load(&c))
		})
	})
}

func testLoadUrlPackageIntegration(t *testing.T) {
	const (
		invalidRequestURI = "/path/with?invalid=query&characters\\foo\n"
		invalidURI        = "http://example.com" + invalidRequestURI
	)
	t.Run("request uri", func(t *testing.T) {
		val := "/the/path"
		requestURI, err := url.ParseRequestURI(val)
		assert.NoError(t, err)
		loadStructFieldTypeTestCase[url.URL](t, *requestURI, val, invalidURI)
		loadStructFieldTypeTestCase[*url.URL](t, requestURI, val, invalidURI)
	})
	t.Run("url", func(t *testing.T) {
		val := "https://go.llib.dev/frameless"
		u, err := url.ParseRequestURI(val)
		assert.NoError(t, err)
		loadStructFieldTypeTestCase[url.URL](t, *u, val, invalidURI)
		loadStructFieldTypeTestCase[*url.URL](t, u, val, invalidURI)
	})
}

func testLoadTimePackageIntegration(t *testing.T) {
	loadStructFieldTypeTestCase[time.Duration](t, time.Minute+5*time.Second, "1m5s", "five minutes")

	t.Run("time.Time struct field", func(t *testing.T) {
		const TimeLayout = "2006-01-02T15"
		type Example struct {
			V time.Time `env:"THE_ENV_KEY" env-time-layout:"2006-01-02T15"`
		}

		t.Run("os env has the value", func(t *testing.T) {
			refTime, err := time.Parse(TimeLayout, rnd.Time().Format(TimeLayout))
			assert.NoError(t, err)
			testcase.SetEnv(t, envKey, refTime.Format(TimeLayout))
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Equal(t, refTime, c.V)
		})

		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})
	})
}

type ExampleConfig[T any] struct {
	V T `env:"THE_ENV_KEY" seperator:","`
	O T `env:"OTH_ENV_KEY" separator:"|"`
}

func loadStructFieldTypeTestCase[T any](t *testing.T, expVal T, envVal, envInvVal string) {
	t.Run(reflectkit.SymbolicName(expVal)+" struct field", func(t *testing.T) {
		t.Run("os env has valid value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, envVal)
			var c ExampleConfig[T]
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, expVal, c.V)
		})
		t.Run("os env has the value, but the value is incorrect", func(t *testing.T) {
			testcase.SetEnv(t, envKey, envInvVal)
			var c ExampleConfig[T]
			assert.Error(t, env.Load(&c))
			assert.Empty(t, c)
		})
		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c ExampleConfig[T]
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})
	})
	t.Run("*"+reflectkit.SymbolicName(expVal)+" struct field", func(t *testing.T) {
		t.Run("os env has valid value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, envVal)
			var c ExampleConfig[*T]
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.NotNil(t, c.V)
			assert.Equal(t, expVal, *c.V)
		})
		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c ExampleConfig[*T]
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
			assert.Nil(t, c.V)
		})
		t.Run("os env has the value, but the value is incorrect", func(t *testing.T) {
			testcase.SetEnv(t, envKey, envInvVal)
			var c ExampleConfig[*T]
			assert.Error(t, env.Load(&c))
		})
	})
	t.Run("[]"+reflectkit.SymbolicName(expVal)+" struct field", func(t *testing.T) {
		t.Run("os env has valid list value as json", func(t *testing.T) {
			var vs []T
			rnd.Repeat(3, 7, func() {
				vs = append(vs, expVal)
			})
			bs, err := json.Marshal(vs)
			assert.NoError(t, err)
			testcase.SetEnv(t, envKey, string(bs))
			var c ExampleConfig[[]T]
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, vs, c.V)
		})
		t.Run("os env has valid comma separated list value", func(t *testing.T) {
			var (
				vs           []T
				envVarValues []string
			)
			rnd.Repeat(3, 7, func() {
				vs = append(vs, expVal)
				envVarValues = append(envVarValues, envVal)
			})
			testcase.SetEnv(t, envKey, strings.Join(envVarValues, ","))
			var c ExampleConfig[[]T]
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, vs, c.V)
		})
		t.Run("os env has valid value separated by the separator defined in the tag", func(t *testing.T) {
			var (
				vs           []T
				envVarValues []string
			)
			rnd.Repeat(3, 7, func() {
				vs = append(vs, expVal)
				envVarValues = append(envVarValues, envVal)
			})
			testcase.SetEnv(t, othEnvKey, strings.Join(envVarValues, "|"))
			var c ExampleConfig[[]T]
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, vs, c.O)
		})
		t.Run("os env has the value, but the value is incorrect", func(t *testing.T) {
			testcase.SetEnv(t, envKey, envInvVal)
			var c ExampleConfig[T]
			assert.Error(t, env.Load(&c))
			assert.Empty(t, c)
		})
		t.Run("os env doesn't have the value", func(t *testing.T) {
			testcase.UnsetEnv(t, envKey)
			var c ExampleConfig[T]
			assert.NoError(t, env.Load(&c))
			assert.Empty(t, c)
		})
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

func TestLookup_smoke(t *testing.T) {
	testcase.UnsetEnv(t, "UNK")
	testcase.SetEnv(t, "FOO", "forty-two")
	testcase.SetEnv(t, "BAR", "42")
	testcase.SetEnv(t, "BAZ", "42.42")
	testcase.SetEnv(t, "QUX", "1;23;4")

	_, ok, err := env.Lookup[string]("UNK")
	assert.NoError(t, err)
	assert.False(t, ok)

	strval, ok, err := env.Lookup[string]("FOO")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "forty-two", strval)

	_, _, err = env.Lookup[int]("FOO")
	assert.Error(t, err)

	intval, ok, err := env.Lookup[int]("BAR")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 42, intval)

	fltval, ok, err := env.Lookup[float64]("BAZ")
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 42.42, fltval)

	vals, ok, err := env.Lookup[[]int]("QUX", env.ListSeparator(';'))
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []int{1, 23, 4}, vals)

	strval, ok, err = env.Lookup[string]("UNK", env.DefaultValue("defval"))
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "defval", strval)

	_, _, err = env.Lookup[string]("UNK", env.Required())
	assert.Error(t, err)

	refTime := rnd.Time()
	layout := time.RFC3339
	testcase.SetEnv(t, "QUUX", refTime.Format(layout))

	timeval, ok, err := env.Lookup[time.Time]("QUUX", env.TimeLayout(layout))
	assert.NoError(t, err)
	assert.True(t, ok)
	assert.True(t, timeval.Equal(refTime))
}

func ExampleSet() {
	var (
		A bool
		B int
		C string
		D []string
	)

	es := &env.Set{}
	env.SetLookup(es, &A, "A")
	env.SetLookup(es, &B, "B")
	env.SetLookup(es, &C, "C", env.DefaultValue("c-default-value"))
	env.SetLookup(es, &D, "D", env.ListSeparator(","))
	if err := es.Parse(); err != nil {
		panic(err)
	}
}

func TestSet(t *testing.T) {
	t.Run("happy", func(t *testing.T) {
		var (
			A bool
			B int
			C string
			D []string
		)

		testcase.SetEnv(t, "A", "true")
		testcase.SetEnv(t, "B", "42")
		testcase.SetEnv(t, "D", "foo,bar,baz")

		es := &env.Set{}
		env.SetLookup(es, &A, "A")
		env.SetLookup(es, &B, "B")
		env.SetLookup(es, &C, "C", env.DefaultValue("c-default-value"))
		env.SetLookup(es, &D, "D", env.ListSeparator(","))
		assert.NoError(t, es.Parse())

		assert.Equal(t, A, true)
		assert.Equal(t, B, 42)
		assert.Equal(t, C, "c-default-value")
		assert.Equal(t, D, []string{"foo", "bar", "baz"})
	})

	t.Run("at least one missing env value", func(t *testing.T) {
		var (
			A bool
			B int
			C string
			D []string
		)

		testcase.SetEnv(t, "A", "true")
		testcase.SetEnv(t, "B", "42")
		testcase.SetEnv(t, "D", "foo,bar,baz")

		random.Pick[func()](rnd,
			func() { testcase.UnsetEnv(t, "A") },
			func() { testcase.UnsetEnv(t, "B") },
			func() { testcase.UnsetEnv(t, "D") },
		)()

		es := &env.Set{}
		env.SetLookup(es, &A, "A")
		env.SetLookup(es, &B, "B")
		env.SetLookup(es, &C, "C", env.DefaultValue("c-default-value"))
		env.SetLookup(es, &D, "D", env.ListSeparator(","))
		assert.Error(t, es.Parse())
	})

	t.Run("at least one env value has an error", func(t *testing.T) {
		var (
			A bool
			B int
		)

		testcase.SetEnv(t, "A", "true")
		testcase.SetEnv(t, "B", "42")

		random.Pick[func()](rnd,
			func() { testcase.SetEnv(t, "A", "that's true") },
			func() { testcase.SetEnv(t, "B", "fourty-two") },
		)()

		es := &env.Set{}
		env.SetLookup(es, &A, "A")
		env.SetLookup(es, &B, "B")
		assert.Error(t, es.Parse())
	})

	t.Run("nil pointer is given for env.Set", func(t *testing.T) {
		val := assert.Panic(t, func() {
			var str string
			env.SetLookup(nil, &str, "KEY")
		})
		assert.NotContain(t, fmt.Sprintf("%v", val), "nil pointer dereference")
	})

	t.Run("nil value pointer is given", func(t *testing.T) {
		val := assert.Panic(t, func() {
			env.SetLookup[string](&env.Set{}, nil, "KEY")
		})
		assert.NotContain(t, fmt.Sprintf("%v", val), "nil pointer dereference")
	})
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
	conf, ok, err := env.Lookup[Conf]("FOO", env.ParseWith(parserFunc))
	_, _, _ = conf, ok, err
}

func TestParseWith(t *testing.T) {
	type Conf struct {
		Foo string
		Bar string
	}
	t.Run("happy", func(t *testing.T) {
		testcase.SetEnv(t, "FOO", "foo:bar")

		conf, ok, err := env.Lookup[Conf]("FOO", env.ParseWith(func(v string) (Conf, error) {
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
		assert.Equal(t, ok, true)
		assert.NoError(t, err)
	})
	t.Run("rainy", func(t *testing.T) {
		testcase.SetEnv(t, "FOO", "whatever")
		expErr := rnd.Error()
		conf, ok, err := env.Lookup[Conf]("FOO", env.ParseWith(func(v string) (Conf, error) {
			return Conf{}, expErr
		}))
		assert.Empty(t, conf)
		assert.False(t, ok)
		assert.ErrorIs(t, err, expErr)
	})
}

func ExampleInit() {
	type MyConfig struct {
		MyRequirement bool `env:"ENV_KEY_1" required:"true"`
	}

	var conf, err = env.Init[MyConfig]()
	_, _ = conf, err
}

func TestInit_smoke(t *testing.T) {
	const envKey = "FL_PKG_ENV_INIT_VAL"

	type Config[T any] struct {
		V T `env:"FL_PKG_ENV_INIT_VAL"`
	}

	testcase.UnsetEnv(t, envKey)

	t.Run("int", func(t *testing.T) {
		exp := rnd.Int()
		setEnv(t, envKey, exp)
		v, err := env.Init[Config[int]]()
		assert.NoError(t, err)
		assert.Equal(t, v.V, exp)
	})

	t.Run("float", func(t *testing.T) {
		exp := rnd.Float32()
		setEnv(t, envKey, exp)
		v, err := env.Init[Config[float32]]()
		assert.NoError(t, err)
		assert.Equal(t, v.V, exp)
	})

	t.Run("bool", func(t *testing.T) {
		exp := rnd.Bool()
		setEnv(t, envKey, exp)
		v, err := env.Init[Config[bool]]()
		assert.NoError(t, err)
		assert.Equal(t, v.V, exp)
	})

	t.Run("string", func(t *testing.T) {
		exp := rnd.String()
		setEnv(t, envKey, exp)
		v, err := env.Init[Config[string]]()
		assert.NoError(t, err)
		assert.Equal(t, v.V, exp)
	})

	t.Run("on error", func(t *testing.T) {
		setEnv(t, envKey, "foo")
		_, err := env.Init[Config[int]]()
		assert.Error(t, err)
	})
}

func setEnv[T any](tb testing.TB, envKey string, v T) {
	tb.Helper()
	got, err := convkit.Format(v)
	assert.NoError(tb, err)
	testcase.SetEnv(tb, envKey, got)
}

func ExampleInitGlobal() {
	type MyConfig struct {
		MyRequirement bool `env:"ENV_KEY_1" required:"true"`
	}

	var conf = env.InitGlobal[MyConfig]()
	_ = conf
}

func TestInitGlobal(t *testing.T) {
	s := testcase.NewSpec(t)

	const envKey = "FL_PKG_ENV_INIT_VAL"

	type Config[T any] struct {
		V T `env:"FL_PKG_ENV_INIT_VAL" required:"true"`
	}

	s.Before(func(t *testcase.T) {
		t.UnsetEnv(envKey)
	})

	const exitCodeNotCalled = -1
	var (
		lastExitCode = testcase.LetValue[int](s, exitCodeNotCalled)
		stderr       = testcase.Let(s, func(t *testcase.T) *os.File {
			dir := t.TempDir()
			t.Cleanup(func() { _ = os.RemoveAll(dir) })
			f, err := os.CreateTemp(dir, "stderr")
			assert.NoError(t, err)
			return f
		})
	)

	s.Before(func(t *testcase.T) {
		osint.StubExit(t, func(code int) {
			lastExitCode.Set(t, code)
		})
		osint.StubStderr(t, stderr.Get(t))
	})

	var readSTDERR = func(t *testcase.T) []byte {
		f := stderr.Get(t)
		f.Seek(0, io.SeekStart)
		out, err := io.ReadAll(f)
		assert.NoError(t, err)
		return out
	}

	s.Test("happy", func(t *testcase.T) {
		exp := rnd.String()
		setEnv(t, envKey, exp)

		var got Config[string]
		assert.NotPanic(t, func() { got = env.InitGlobal[Config[string]]() })
		assert.Equal(t, got.V, exp)
		assert.Equal(t, lastExitCode.Get(t), exitCodeNotCalled)
		assert.Empty(t, readSTDERR(t))
	})

	s.Test("invalid value", func(t *testcase.T) {
		setEnv(t, envKey, "foo")

		assert.Panic(t, func() { env.InitGlobal[Config[int]]() })
		assert.Equal(t, lastExitCode.Get(t), 1)

		out := readSTDERR(t)
		assert.NotEmpty(t, out)
		assert.Contain(t, string(out), env.ErrInvalidValue.Error())
	})

	s.Test("missing key", func(t *testcase.T) {
		t.UnsetEnv(envKey)

		assert.Panic(t, func() { env.InitGlobal[Config[string]]() })
		assert.Equal(t, lastExitCode.Get(t), 1)

		out := readSTDERR(t)
		assert.NotEmpty(t, out)
		assert.Contain(t, string(out), envKey)
		assert.Contain(t, string(out), env.ErrMissingEnvironmentVariable.Error())
	})
}

func TestLookupFieldEnvNames(t *testing.T) {
	type X struct {
		A string `env:"X_A"`
		B string `env:"X_B,XB"`
		C string
	}

	T := reflect.TypeOf((*X)(nil)).Elem()

	names, ok := env.LookupFieldEnvNames(must.OK(T.FieldByName("A")))
	assert.True(t, ok)
	assert.ContainExactly(t, names, []string{"X_A"})

	names, ok = env.LookupFieldEnvNames(must.OK(T.FieldByName("B")))
	assert.True(t, ok)
	assert.ContainExactly(t, names, []string{"X_B", "XB"})

	names, ok = env.LookupFieldEnvNames(must.OK(T.FieldByName("C")))
	assert.False(t, ok)
	assert.Empty(t, names)

	names, ok = env.LookupFieldEnvNames(reflect.StructField{})
	assert.False(t, ok)
	assert.Empty(t, names)
}

func TestReflectLoad_smoke(t *testing.T) {
	type Example struct {
		V string `env:"THE_ENV_KEY"`
	}
	t.Run("os env has the value", func(t *testing.T) {
		testcase.SetEnv(t, envKey, "42")
		var c Example
		assert.NoError(t, env.ReflectLoad(reflect.ValueOf(&c)))
		assert.NotEmpty(t, c)
		assert.Equal(t, "42", c.V)
	})
	t.Run("os env doesn't have the value", func(t *testing.T) {
		testcase.UnsetEnv(t, envKey)
		var c Example
		assert.NoError(t, env.ReflectLoad(reflect.ValueOf(&c)))
		assert.Empty(t, c)
	})
}

func TestReflectLoadField_smoke(t *testing.T) {
	type Example struct {
		V string `env:"THE_ENV_KEY"`
	}
	t.Run("os env has the value", func(t *testing.T) {
		testcase.SetEnv(t, envKey, "42")
		var c Example
		strucT := reflect.ValueOf(&c).Elem()
		assert.NoError(t, env.ReflectLoadField(strucT, strucT.Type().Field(0)))
		assert.NotEmpty(t, c)
		assert.Equal(t, c.V, "42")
	})
	t.Run("os env doesn't have the value", func(t *testing.T) {
		testcase.UnsetEnv(t, envKey)
		var c Example
		strucT := reflect.ValueOf(&c).Elem()
		assert.NoError(t, env.ReflectLoadField(strucT, strucT.Type().Field(0)))
		assert.Empty(t, c.V)
	})
}
