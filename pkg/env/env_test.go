package env_test

import (
	"encoding/json"
	"github.com/adamluzsi/frameless/pkg/enum"
	"reflect"
	"strings"
	"testing"

	"github.com/adamluzsi/frameless/pkg/env"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

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

	loadStructFieldTypeTestCase[int](t, 42, "42")
	type intType int
	loadStructFieldTypeTestCase[intType](t, 42, "42")

	loadStructFieldTypeTestCase[int8](t, 42, "42")
	type int8Type int8
	loadStructFieldTypeTestCase[int8Type](t, 42, "42")

	loadStructFieldTypeTestCase[int16](t, 42, "42")
	type int16Type int16
	loadStructFieldTypeTestCase[int16Type](t, 42, "42")

	loadStructFieldTypeTestCase[int32](t, 42, "42")
	type int32Type int32
	loadStructFieldTypeTestCase[int32Type](t, 42, "42")

	loadStructFieldTypeTestCase[int64](t, 42, "42")
	type int64Type int64
	loadStructFieldTypeTestCase[int64Type](t, 42, "42")

	loadStructFieldTypeTestCase[float32](t, 42.42, "42.42")
	type float32Type float32
	loadStructFieldTypeTestCase[float32Type](t, 42.42, "42.42")

	loadStructFieldTypeTestCase[float64](t, 42.42, "42.42")
	type float64Type float64
	loadStructFieldTypeTestCase[float64Type](t, 42.42, "42.42")

	loadStructFieldTypeTestCase[bool](t, true, "true")
	type boolType bool
	loadStructFieldTypeTestCase[boolType](t, true, "t")

	t.Run("time struct field", func(t *testing.T) {
		type Example struct {
			V string `env:"THE_ENV_KEY" time-layout:""`
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

	t.Run("struct field without tag will be visited", func(t *testing.T) {
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

		t.Run("when `separator` tag is defined for a non-slice type", func(t *testing.T) {
			type Example struct {
				V string `env:"V_KEY" separator:";"`
			}

			t.Run("then it yields an error", func(t *testing.T) {
				testcase.SetEnv(t, "V_KEY", "A;B;C;D")
				var c Example
				assert.Error(t, env.Load(&c))
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

type ExampleConfig[T any] struct {
	V T `env:"THE_ENV_KEY"`
	O T `env:"OTH_ENV_KEY" separator:"|"`
}

func loadStructFieldTypeTestCase[T any](t *testing.T, expVal T, envVal string) {
	t.Run(reflectkit.SymbolicName(expVal)+" struct field", func(t *testing.T) {
		t.Run("os env has valid value", func(t *testing.T) {
			testcase.SetEnv(t, envKey, envVal)
			var c ExampleConfig[T]
			assert.NoError(t, env.Load(&c))
			assert.NotEmpty(t, c)
			assert.Equal(t, expVal, c.V)
		})
		t.Run("os env has the value, but the value is incorrect", func(t *testing.T) {
			testcase.SetEnv(t, envKey, "forty-two")
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
	t.Run("[]"+reflectkit.SymbolicName(expVal)+" struct field", func(t *testing.T) {
		rnd := random.New(random.CryptoSeed{})
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
			testcase.SetEnv(t, envKey, "forty-two")
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
}
