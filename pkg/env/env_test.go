package env_test

import (
	"github.com/adamluzsi/frameless/pkg/enum"
	"reflect"
	"testing"

	"github.com/adamluzsi/frameless/pkg/env"
	"github.com/adamluzsi/frameless/pkg/reflectkit"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
	"github.com/adamluzsi/testcase/random"
)

const envKey = "THE_ENV_KEY"

func TestLoad(t *testing.T) {
	t.Run("on nil value", func(t *testing.T) {
		type Example struct{}
		assert.Error(t, env.Load[Example](nil))
	})

	t.Run("on non-struct type", func(t *testing.T) {
		var c string
		assert.Error(t, env.Load(&c))
	})

	t.Run("struct fields without env tag are ignored", func(t *testing.T) {
		type Example struct{ V string }
		var c Example
		assert.NoError(t, env.Load(&c))
		assert.Empty(t, c)
	})

	t.Run("string field", func(t *testing.T) {
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

	loadTestCase[int](t, 42, "42")
	type intType int
	loadTestCase[intType](t, 42, "42")

	loadTestCase[int8](t, 42, "42")
	type int8Type int8
	loadTestCase[int8Type](t, 42, "42")

	loadTestCase[int16](t, 42, "42")
	type int16Type int16
	loadTestCase[int16Type](t, 42, "42")

	loadTestCase[int32](t, 42, "42")
	type int32Type int32
	loadTestCase[int32Type](t, 42, "42")

	loadTestCase[int64](t, 42, "42")
	type int64Type int64
	loadTestCase[int64Type](t, 42, "42")

	loadTestCase[float32](t, 42.42, "42.42")
	type float32Type float32
	loadTestCase[float32Type](t, 42.42, "42.42")

	loadTestCase[float64](t, 42.42, "42.42")
	type float64Type float64
	loadTestCase[float64Type](t, 42.42, "42.42")

	loadTestCase[bool](t, true, "true")
	type boolType bool
	loadTestCase[boolType](t, true, "t")

	t.Run("struct fields will be visited", func(t *testing.T) {
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
}

func loadTestCase[T any](t *testing.T, expVal T, envVal string) {
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
