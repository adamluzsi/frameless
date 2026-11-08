package env_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.llib.dev/frameless/pkg/convkit"
	"go.llib.dev/frameless/pkg/enum"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/internal/osint"
	"go.llib.dev/frameless/pkg/must"
	"go.llib.dev/frameless/pkg/validate"

	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
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
		t.Run("os env has the value under the second key, then it takes precedence over the default", func(t *testing.T) {
			type Example struct {
				F string `env:"THE_ENV_KEY_1, THE_ENV_KEY_2" default:"the-default"`
			}
			testcase.UnsetEnv(t, "THE_ENV_KEY_1")
			testcase.SetEnv(t, "THE_ENV_KEY_2", "24")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Equal(t, "24", c.F)
		})
		t.Run("os env has the value under the second key, then it satisfies required", func(t *testing.T) {
			type Example struct {
				F string `env:"THE_ENV_KEY_1, THE_ENV_KEY_2" required:"true"`
			}
			testcase.UnsetEnv(t, "THE_ENV_KEY_1")
			testcase.SetEnv(t, "THE_ENV_KEY_2", "24")
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Equal(t, "24", c.F)
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

	t.Run("[]string struct field with a JSON array value", func(t *testing.T) {
		type Example struct {
			V []string `env:"THE_ENV_KEY"`
		}

		t.Run("then list elements can contain the separator", func(t *testing.T) {
			testcase.SetEnv(t, envKey, `["foo,bar","baz"]`)
			var c Example
			assert.NoError(t, env.Load(&c))
			assert.Equal(t, []string{"foo,bar", "baz"}, c.V)
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

	t.Run("time.Time struct field with the shared layout tag", func(t *testing.T) {
		const TimeLayout = "2006-01-02"
		type Example struct {
			V time.Time `env:"THE_ENV_KEY" layout:"2006-01-02"`
		}

		refTime, err := time.Parse(TimeLayout, rnd.Time().Format(TimeLayout))
		assert.NoError(t, err)
		testcase.SetEnv(t, envKey, refTime.Format(TimeLayout))
		var c Example
		assert.NoError(t, env.Load(&c))
		assert.Equal(t, refTime, c.V)
	})

	t.Run("time.Time struct field with both env-time-layout and layout tag", func(t *testing.T) {
		const TimeLayout = "2006-01-02T15"
		type Example struct {
			V time.Time `env:"THE_ENV_KEY" env-time-layout:"2006-01-02T15" layout:"2006-01-02"`
		}

		refTime, err := time.Parse(TimeLayout, rnd.Time().Format(TimeLayout))
		assert.NoError(t, err)
		testcase.SetEnv(t, envKey, refTime.Format(TimeLayout))
		var c Example
		assert.NoError(t, env.Load(&c), "env-time-layout takes precedence over layout")
		assert.Equal(t, refTime, c.V)
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
		t.Run("os env has a single list element as value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, envVal)
			var c ExampleConfig[[]T]
			assert.NoError(t, env.Load(&c))
			assert.Equal(t, []T{expVal}, c.V)
		})
		t.Run("os env has a single list element as value for a field with a separator tag", func(t *testing.T) {
			testcase.SetEnv(t, othEnvKey, envVal)
			var c ExampleConfig[[]T]
			assert.NoError(t, env.Load(&c))
			assert.Equal(t, []T{expVal}, c.O)
		})
		t.Run("os env has valid list value as json for a field with a separator tag", func(t *testing.T) {
			var vs []T
			rnd.Repeat(3, 7, func() {
				vs = append(vs, expVal)
			})
			bs, err := json.Marshal(vs)
			assert.NoError(t, err)
			testcase.SetEnv(t, othEnvKey, string(bs))
			var c ExampleConfig[[]T]
			assert.NoError(t, env.Load(&c))
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
		assert.NotContains(t, fmt.Sprintf("%v", val), "nil pointer dereference")
	})

	t.Run("nil value pointer is given", func(t *testing.T) {
		val := assert.Panic(t, func() {
			env.SetLookup[string](&env.Set{}, nil, "KEY")
		})
		assert.NotContains(t, fmt.Sprintf("%v", val), "nil pointer dereference")
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
		assert.Contains(t, string(out), env.ErrInvalidValue.Error())
	})

	s.Test("missing key", func(t *testcase.T) {
		t.UnsetEnv(envKey)

		assert.Panic(t, func() { env.InitGlobal[Config[string]]() })
		assert.Equal(t, lastExitCode.Get(t), 1)

		out := readSTDERR(t)
		assert.NotEmpty(t, out)
		assert.Contains(t, string(out), envKey)
		assert.Contains(t, string(out), env.ErrMissingEnvironmentVariable.Error())
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
	assert.ContainsExactly(t, names, []string{"X_A"})

	names, ok = env.LookupFieldEnvNames(must.OK(T.FieldByName("B")))
	assert.True(t, ok)
	assert.ContainsExactly(t, names, []string{"X_B", "XB"})

	names, ok = env.LookupFieldEnvNames(must.OK(T.FieldByName("C")))
	assert.False(t, ok)
	assert.Empty(t, names)

	names, ok = env.LookupFieldEnvNames(reflect.StructField{})
	assert.False(t, ok)
	assert.Empty(t, names)
}

func TestReflectTryLoadField_smoke(t *testing.T) {
	type Example struct {
		A string `env:"THE_ENV_KEY" enum:"foo,bar,baz,"`
		B int    `env:"OTH_ENV_KEY"`
	}
	t.Run("os env has the value", func(t *testing.T) {
		testcase.SetEnv(t, envKey, "foo")
		var c Example
		assert.NoError(t, env.ReflectTryLoad(reflect.ValueOf(&c)))
		assert.NotEmpty(t, c)
		assert.Equal(t, c.A, "foo")
	})
	t.Run("have fields which's env value is not loaded", func(t *testing.T) {
		testcase.UnsetEnv(t, envKey)
		var c Example
		assert.NoError(t, env.ReflectTryLoad(reflect.ValueOf(&c)))
		assert.Empty(t, c.A)
	})
	t.Run("have a field which env value is incorrect", func(t *testing.T) {
		t.Run("type", func(t *testing.T) {
			testcase.SetEnv(t, othEnvKey, "foo")
			var c Example
			ptr := reflect.ValueOf(&c)
			err := env.ReflectTryLoad(ptr)
			assert.ErrorIs(t, env.ErrInvalidValue, err)
			assert.Empty(t, c.B)
		})
		t.Run("enum", func(t *testing.T) {
			testcase.SetEnv(t, envKey, "fo")
			var c Example
			ptr := reflect.ValueOf(&c)
			err := env.ReflectTryLoad(ptr)
			assert.ErrorIs(t, enum.ErrInvalid, err)
		})
		t.Run("validate", func(t *testing.T) {
			type C struct {
				V StringTypeWithValidator `env:"VAL"`
			}
			const EnvVarName = "VAL"

			t.Run("value is in the env variables and valid", func(t *testing.T) {
				testcase.SetEnv(t, "VAL", "valid")

				var c C
				assert.NoError(t, env.ReflectTryLoad(reflect.ValueOf(&c)))
				assert.Equal(t, "valid", c.V)
			})

			t.Run("value in zero state is invalid, but because it is absent from the env vars, it is ignored", func(t *testing.T) {
				testcase.UnsetEnv(t, "VAL")

				var c C
				assert.NoError(t, env.ReflectTryLoad(reflect.ValueOf(&c)))
			})

			t.Run("value is in the env variables but invalid", func(t *testing.T) {
				testcase.SetEnv(t, "VAL", "invalid")

				var c C
				assert.ErrorIs(t, env.ReflectTryLoad(reflect.ValueOf(&c)), ErrStringTypeWithValidatorInvalid)
			})
		})
	})
}

func TestLoad_noEnum(t *testing.T) {
	type T struct {
		V string `env:"VAL"`
	}

	var v T

	assert.NoError(t, env.Load(&v))
}

type ValidatorStub struct {
	Error error
}

func (m ValidatorStub) Validate(context.Context) error {
	return m.Error
}

type StringTypeWithValidator string

const ErrStringTypeWithValidatorZero errorkit.Error = "zero value is considered incorrect"
const ErrStringTypeWithValidatorInvalid errorkit.Error = `"invalid" is considered as an invalid value`

func (str StringTypeWithValidator) Validate(context.Context) error {
	if len(str) == 0 {
		return ErrStringTypeWithValidatorZero
	}
	if str == "invalid" {
		return ErrStringTypeWithValidatorInvalid
	}
	return nil
}

func Test_TypeWithValidator(t *testing.T) {
	t.Log("tests are dependig on the logic written in the validation, so this test ensures that it is not accidentally changed")
	var validValue StringTypeWithValidator = random.Unique(func() StringTypeWithValidator {
		return StringTypeWithValidator(rnd.StringNC(rnd.IntBetween(3, 7), random.CharsetAlpha()))
	}, "invalid", "")
	assert.NoError(t, validate.Value(t.Context(), StringTypeWithValidator(validValue)))
	assert.Error(t, validate.Value(t.Context(), StringTypeWithValidator("")))
	assert.Error(t, validate.Value(t.Context(), StringTypeWithValidator("invalid")))
}

func TestLoad_typeWithValidator(t *testing.T) {
	testcase.UnsetEnv(t, "V")

	type TypeWithValidator struct {
		V StringTypeWithValidator `env:"V"`
	}

	t.Run("when value is found and validator pass", func(t *testing.T) {
		expV := rnd.StringNC(5, random.CharsetDigit())
		testcase.SetEnv(t, "V", expV)

		var c TypeWithValidator
		assert.NoError(t, env.Load(&c), "expected no issue")

		assert.Equal(t, expV, string(c.V))
	})

	t.Run("when value is set, but to an invalid value", func(t *testing.T) {
		testcase.SetEnv(t, "V", "invalid") // set but invalid
		var c TypeWithValidator
		assert.Error(t, env.Load(&c))
	})

	t.Run("struct field has a validation issue from a validate error", func(t *testing.T) {
		type C struct {
			V ValidatorStub
		}

		var c C
		assert.NoError(t, env.Load(&c))

		expErr := rnd.Error()
		c.V.Error = expErr

		assert.ErrorIs(t, env.Load(&c), expErr)
	})
}

func TestLoad_opaque(t *testing.T) {
	s := testcase.NewSpec(t)

	const (
		nameEnvKey  = "FL_PKG_ENV_OPAQUE_NAME"  // the env tag of Config.Name, and ForeignConfig.Name
		tokenEnvKey = "FL_PKG_ENV_OPAQUE_TOKEN" // the env tag of ForeignConfig.Token
		levelEnvKey = "FL_PKG_ENV_OPAQUE_LEVEL" // the env tag of Settings.Level
		jsonEnvKey  = "FL_PKG_ENV_OPAQUE_JSON"  // the env tag of Config.JSON
	)

	// ForeignConfig mimics a type which was designed with different principles than the env package,
	// such as the config type of a third-party library.
	// Its tags share their names with the env tags, but they don't follow the env conventions.
	type ForeignConfig struct {
		Name  string `env:"FL_PKG_ENV_OPAQUE_NAME" default:"foreign-default"` // the same env var as Config.Name
		Token string `env:"FL_PKG_ENV_OPAQUE_TOKEN" required:"true"`
	}
	// Settings follows the env conventions, so its tags are used when the env package looks into it.
	type Settings struct {
		Level string `env:"FL_PKG_ENV_OPAQUE_LEVEL" default:"info"`
	}
	type Config struct {
		Name string `env:"FL_PKG_ENV_OPAQUE_NAME"`

		ForeignConfig `opaque:""`
		Field         ForeignConfig `opaque:"true"`
		Alias         ForeignConfig `norecurse:""`

		// JSON is an input itself, which takes a JSON value, but the tags of its type are not used.
		JSON ForeignConfig `env:"FL_PKG_ENV_OPAQUE_JSON" opaque:""`

		// Settings is looked into as usual, since its opaque tag is turned off.
		Settings Settings `opaque:"false"`
	}

	var (
		config = let.Var(s, func(t *testcase.T) *Config {
			return &Config{}
		})
		name = let.StringNC(s, 8, random.CharsetAlpha())
	)
	act := func(t *testcase.T) error {
		return env.Load(config.Get(t))
	}

	s.Before(func(t *testcase.T) {
		t.SetEnv(nameEnvKey, name.Get(t))
		// ForeignConfig.Token is required, so it would fail the loading if it were used.
		t.UnsetEnv(tokenEnvKey)
		t.UnsetEnv(levelEnvKey)
		t.UnsetEnv(jsonEnvKey)
	})

	s.Then("the struct's own fields are loaded, even when an opaque field's type uses the same env var", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).Name, name.Get(t))
	})

	s.Then("the fields of an opaque field's type are not loaded, so their default tags are not applied, and their required tags are not enforced", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).ForeignConfig, ForeignConfig{})
		assert.Equal(t, config.Get(t).Field, ForeignConfig{})
		assert.Equal(t, config.Get(t).JSON, ForeignConfig{})
	})

	s.Then("the norecurse alias works the same as the opaque tag", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).Alias, ForeignConfig{})
	})

	s.Then(`a field tagged with opaque:"false" is looked into as usual`, func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).Settings.Level, "info")
	})

	s.When("the env var of an opaque field's type is set", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.SetEnv(tokenEnvKey, t.Random.StringNC(8, random.CharsetAlpha()))
		})

		s.Then("it is not read", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Empty(t, config.Get(t).ForeignConfig.Token)
			assert.Empty(t, config.Get(t).Field.Token)
			assert.Empty(t, config.Get(t).Alias.Token)
			assert.Empty(t, config.Get(t).JSON.Token)
		})
	})

	s.When("the opaque field is an input itself, and its env var holds a JSON value", func(s *testcase.Spec) {
		var (
			foreign = let.Var(s, func(t *testcase.T) ForeignConfig {
				return ForeignConfig{
					Name:  t.Random.StringNC(8, random.CharsetAlpha()),
					Token: t.Random.StringNC(8, random.CharsetAlpha()),
				}
			})
			foreignJSON = let.Var(s, func(t *testcase.T) string {
				data, err := json.Marshal(foreign.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
		)
		s.Before(func(t *testcase.T) {
			t.SetEnv(jsonEnvKey, foreignJSON.Get(t))
		})

		s.Then("the JSON value is parsed into the field", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, config.Get(t).JSON, foreign.Get(t))
		})

		s.And("the JSON value doesn't match the field's type", func(s *testcase.Spec) {
			foreignJSON.Let(s, func(t *testcase.T) string {
				return fmt.Sprintf(`{"Name":%d}`, t.Random.Int())
			})

			s.Then("it fails with an invalid value error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), env.ErrInvalidValue)
			})
		})
	})
}

func TestLoad_opaqueWithInvalidValue(t *testing.T) {
	s := testcase.NewSpec(t)

	type Settings struct {
		Level string `env:"FL_PKG_ENV_OPAQUE_LEVEL" default:"info"`
	}
	type Config struct {
		Settings Settings `opaque:"maybe"`
	}

	config := let.Var(s, func(t *testcase.T) *Config {
		return &Config{}
	})
	act := func(t *testcase.T) error {
		return env.Load(config.Get(t))
	}

	s.Then("it panics with the field, the tag and the invalid value, so a mistyped tag can't go unnoticed", func(t *testcase.T) {
		msg := fmt.Sprint(assert.Panic(t, func() { _ = act(t) }))
		assert.Contains(t, msg, "Settings")
		assert.Contains(t, msg, "opaque")
		assert.Contains(t, msg, "maybe")
	})
}

func TestLoad_structFieldWithEnvTag(t *testing.T) {
	s := testcase.NewSpec(t)

	const (
		retryEnvKey  = "FL_PKG_ENV_STRUCT_RETRY"   // the env tag of Config.Retry
		dbEnvKey     = "FL_PKG_ENV_STRUCT_DB"      // the env tag of Config.DB
		dbHostEnvKey = "FL_PKG_ENV_STRUCT_DB_HOST" // the env tag of DB.Host
		dbPortEnvKey = "FL_PKG_ENV_STRUCT_DB_PORT" // the env tag of DB.Port

		authEnvKey      = "FL_PKG_ENV_STRUCT_AUTH"       // the env tag of Config.Auth
		authTokenEnvKey = "FL_PKG_ENV_STRUCT_AUTH_TOKEN" // the env tag of Auth.Token
	)

	// RetryPolicy has no env tags of its own, so it can only be loaded as a whole, from a JSON value.
	type RetryPolicy struct {
		Max     int
		Backoff time.Duration
	}
	// DB has env tags of its own, and it can be loaded as a whole from a JSON value too.
	type DB struct {
		Host string `env:"FL_PKG_ENV_STRUCT_DB_HOST" default:"localhost"`
		Port int    `env:"FL_PKG_ENV_STRUCT_DB_PORT" default:"5432"`
	}
	// Auth has a required inner field, which can be provided either by its own env var or as part of a JSON value.
	type Auth struct {
		Token string `env:"FL_PKG_ENV_STRUCT_AUTH_TOKEN" required:"true"`
	}
	type Config struct {
		Retry RetryPolicy `env:"FL_PKG_ENV_STRUCT_RETRY"`
		DB    DB          `env:"FL_PKG_ENV_STRUCT_DB"`
		Auth  Auth        `env:"FL_PKG_ENV_STRUCT_AUTH"`
	}

	var (
		config = let.Var(s, func(t *testcase.T) *Config {
			return &Config{}
		})
		// innerToken is set as the env var of the required inner field, so the scenarios not about it stay valid.
		innerToken = let.StringNC(s, 8, random.CharsetAlpha())
	)
	act := func(t *testcase.T) error {
		return env.Load(config.Get(t))
	}

	s.Before(func(t *testcase.T) {
		t.UnsetEnv(retryEnvKey)
		t.UnsetEnv(dbEnvKey)
		t.UnsetEnv(dbHostEnvKey)
		t.UnsetEnv(dbPortEnvKey)
		t.UnsetEnv(authEnvKey)
		t.SetEnv(authTokenEnvKey, innerToken.Get(t))
	})

	s.Then("the inner fields are loaded from their own tags, like their defaults", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).DB, DB{Host: "localhost", Port: 5432})
		assert.Equal(t, config.Get(t).Retry, RetryPolicy{})
	})

	s.Then("a required inner field is loaded from its own env var", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).Auth, Auth{Token: innerToken.Get(t)})
	})

	s.When("the env var of a struct field without env tags of its own holds a JSON value", func(s *testcase.Spec) {
		var (
			retry = let.Var(s, func(t *testcase.T) RetryPolicy {
				return RetryPolicy{
					Max:     t.Random.IntBetween(1, 10),
					Backoff: time.Duration(t.Random.IntBetween(1, 60)) * time.Second,
				}
			})
			retryJSON = let.Var(s, func(t *testcase.T) string {
				data, err := json.Marshal(retry.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
		)
		s.Before(func(t *testcase.T) {
			t.SetEnv(retryEnvKey, retryJSON.Get(t))
		})

		s.Then("the JSON value is loaded into the field", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, config.Get(t).Retry, retry.Get(t))
		})

		s.And("the JSON value doesn't match the field's type", func(s *testcase.Spec) {
			retryJSON.Let(s, func(t *testcase.T) string {
				return fmt.Sprintf(`{"Max":%q}`, t.Random.StringNC(8, random.CharsetAlpha()))
			})

			s.Then("it fails with an invalid value error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), env.ErrInvalidValue)
			})
		})
	})

	s.When("the env vars of the inner fields are set", func(s *testcase.Spec) {
		var (
			innerHost = let.StringNC(s, 8, random.CharsetAlpha())
			innerPort = let.IntB(s, 1000, 1999)
		)
		s.Before(func(t *testcase.T) {
			t.SetEnv(dbHostEnvKey, innerHost.Get(t))
			t.SetEnv(dbPortEnvKey, strconv.Itoa(innerPort.Get(t)))
		})

		s.Then("the inner fields are loaded from them", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, config.Get(t).DB, DB{Host: innerHost.Get(t), Port: innerPort.Get(t)})
		})
	})

	s.When("the env var of a struct field with env tags of its own holds a JSON value", func(s *testcase.Spec) {
		var (
			db = let.Var(s, func(t *testcase.T) DB {
				return DB{
					Host: t.Random.StringNC(8, random.CharsetAlpha()),
					Port: t.Random.IntBetween(2000, 2999),
				}
			})
			dbJSON = let.Var(s, func(t *testcase.T) string {
				data, err := json.Marshal(db.Get(t))
				assert.Must(t).NoError(err)
				return string(data)
			})
		)
		s.Before(func(t *testcase.T) {
			t.SetEnv(dbEnvKey, dbJSON.Get(t))
		})

		s.Then("the JSON value is loaded onto the inner fields, overwriting their defaults", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, config.Get(t).DB, db.Get(t))
		})

		s.And("the env vars of the inner fields are set too", func(s *testcase.Spec) {
			var (
				innerHost = let.StringNC(s, 8, random.CharsetAlpha())
				innerPort = let.IntB(s, 1000, 1999)
			)
			s.Before(func(t *testcase.T) {
				t.SetEnv(dbHostEnvKey, innerHost.Get(t))
				t.SetEnv(dbPortEnvKey, strconv.Itoa(innerPort.Get(t)))
			})

			s.Then("the inner fields are loaded first, then the JSON value overwrites them", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, config.Get(t).DB, db.Get(t))
			})
		})

		s.And("the JSON value has only some of the fields", func(s *testcase.Spec) {
			dbJSON.Let(s, func(t *testcase.T) string {
				return fmt.Sprintf(`{"Host":%q}`, db.Get(t).Host)
			})

			s.Then("the JSON value is merged onto the inner fields, so the absent ones keep their loaded value", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, config.Get(t).DB, DB{Host: db.Get(t).Host, Port: 5432})
			})
		})

		s.And("the JSON value doesn't match the field's type", func(s *testcase.Spec) {
			dbJSON.Let(s, func(t *testcase.T) string {
				return fmt.Sprintf(`{"Port":%q}`, t.Random.StringNC(8, random.CharsetAlpha()))
			})

			s.Then("it fails with an invalid value error", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), env.ErrInvalidValue)
			})
		})
	})

	// required is checked once the whole loading is complete,
	// so a required inner field can be satisfied by the JSON value of its outer struct field too.
	s.When("the env var of a required inner field is not set", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.UnsetEnv(authTokenEnvKey)
		})

		s.Then("it fails with a missing environment variable error that names the env var", func(t *testcase.T) {
			err := act(t)

			assert.ErrorIs(t, err, env.ErrMissingEnvironmentVariable)
			assert.Contains(t, err.Error(), authTokenEnvKey)
		})

		s.And("the env var of the outer struct field holds a JSON value with it", func(s *testcase.Spec) {
			auth := let.Var(s, func(t *testcase.T) Auth {
				return Auth{Token: t.Random.StringNC(8, random.CharsetAlpha())}
			})
			s.Before(func(t *testcase.T) {
				data, err := json.Marshal(auth.Get(t))
				assert.Must(t).NoError(err)
				t.SetEnv(authEnvKey, string(data))
			})

			s.Then("required is satisfied by the JSON value", func(t *testcase.T) {
				assert.NoError(t, act(t))

				assert.Equal(t, config.Get(t).Auth, auth.Get(t))
			})
		})

		s.And("the env var of the outer struct field holds a JSON value without it", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				t.SetEnv(authEnvKey, `{}`)
			})

			s.Then("it still fails with a missing environment variable error that names the env var", func(t *testcase.T) {
				err := act(t)

				assert.ErrorIs(t, err, env.ErrMissingEnvironmentVariable)
				assert.Contains(t, err.Error(), authTokenEnvKey)
			})
		})
	})

	s.When("the env var of a required inner field is set to an empty value", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.SetEnv(authTokenEnvKey, "")
		})

		s.Then("required is satisfied, since the env var is present, even though the loaded value is empty", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, config.Get(t).Auth, Auth{})
		})
	})
}

func TestLoad_structFieldWithDefaultJSON(t *testing.T) {
	s := testcase.NewSpec(t)

	const (
		replicaEnvKey      = "FL_PKG_ENV_STRUCT_REPLICA"       // the env tag of Config.Replica
		replicaHostEnvKey  = "FL_PKG_ENV_STRUCT_REPLICA_HOST"  // the env tag of Replica.Host
		replicaPortEnvKey  = "FL_PKG_ENV_STRUCT_REPLICA_PORT"  // the env tag of Replica.Port
		replicaUserEnvKey  = "FL_PKG_ENV_STRUCT_REPLICA_USER"  // the env tag of Replica.User
		replicaTokenEnvKey = "FL_PKG_ENV_STRUCT_REPLICA_TOKEN" // the env tag of Replica.Token
	)
	// the values in the default tag of Config.Replica
	const (
		defaultHost  = "replica.internal"
		defaultToken = "from-default"
	)

	// Replica has inner defaults and required inner fields.
	type Replica struct {
		Host  string `env:"FL_PKG_ENV_STRUCT_REPLICA_HOST" default:"localhost"`
		Port  int    `env:"FL_PKG_ENV_STRUCT_REPLICA_PORT" default:"5432"`
		User  string `env:"FL_PKG_ENV_STRUCT_REPLICA_USER" required:"true"`
		Token string `env:"FL_PKG_ENV_STRUCT_REPLICA_TOKEN" required:"true"`
	}
	// Config puts a JSON value into a default tag. Horrifying, but possible.
	// The default JSON overwrites an inner default (Host), satisfies a required inner field (Token),
	// and lacks an inner field with a default (Port) and a required inner field (User).
	type Config struct {
		Replica Replica `env:"FL_PKG_ENV_STRUCT_REPLICA" default:"{\"Host\":\"replica.internal\",\"Token\":\"from-default\"}"`
	}

	var (
		config = let.Var(s, func(t *testcase.T) *Config {
			return &Config{}
		})
		// innerUser is set as the env var of the required inner field that the default JSON lacks,
		// so the scenarios not about it stay valid.
		innerUser = let.StringNC(s, 8, random.CharsetAlpha())
	)
	act := func(t *testcase.T) error {
		return env.Load(config.Get(t))
	}

	s.Before(func(t *testcase.T) {
		t.UnsetEnv(replicaEnvKey)
		t.UnsetEnv(replicaHostEnvKey)
		t.UnsetEnv(replicaPortEnvKey)
		t.UnsetEnv(replicaTokenEnvKey)
		t.SetEnv(replicaUserEnvKey, innerUser.Get(t))
	})

	s.Then("the default JSON is merged onto the inner fields, overwriting their defaults, while the absent ones keep theirs", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).Replica.Host, defaultHost)
		assert.Equal(t, config.Get(t).Replica.Port, 5432)
	})

	s.Then("the default JSON satisfies a required inner field that has no input", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.Equal(t, config.Get(t).Replica.Token, defaultToken)
	})

	s.When("the env vars of the inner fields are set", func(s *testcase.Spec) {
		var (
			innerHost  = let.StringNC(s, 8, random.CharsetAlpha())
			innerPort  = let.IntB(s, 1000, 1999)
			innerToken = let.StringNC(s, 8, random.CharsetAlpha())
		)
		s.Before(func(t *testcase.T) {
			t.SetEnv(replicaHostEnvKey, innerHost.Get(t))
			t.SetEnv(replicaPortEnvKey, strconv.Itoa(innerPort.Get(t)))
			t.SetEnv(replicaTokenEnvKey, innerToken.Get(t))
		})

		s.Then("they win over the default JSON, since a default never overwrites an input", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, config.Get(t).Replica, Replica{
				Host:  innerHost.Get(t),
				Port:  innerPort.Get(t),
				User:  innerUser.Get(t),
				Token: innerToken.Get(t),
			})
		})
	})

	s.When("the env var of the struct field holds a JSON value", func(s *testcase.Spec) {
		port := let.IntB(s, 2000, 2999)
		s.Before(func(t *testcase.T) {
			t.SetEnv(replicaEnvKey, fmt.Sprintf(`{"Port":%d}`, port.Get(t)))
		})

		s.Then("it is merged onto the value the field has without it, including its default JSON", func(t *testcase.T) {
			assert.NoError(t, act(t))

			assert.Equal(t, config.Get(t).Replica, Replica{
				Host:  defaultHost,
				Port:  port.Get(t),
				User:  innerUser.Get(t),
				Token: defaultToken,
			})
		})
	})

	s.When("the env var of a required inner field that the default JSON lacks is not set", func(s *testcase.Spec) {
		s.Before(func(t *testcase.T) {
			t.UnsetEnv(replicaUserEnvKey)
		})

		s.Then("it fails with a missing environment variable error that names the env var", func(t *testcase.T) {
			err := act(t)

			assert.ErrorIs(t, err, env.ErrMissingEnvironmentVariable)
			assert.Contains(t, err.Error(), replicaUserEnvKey)
		})
	})
}
