package option_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/pointer"
	"go.llib.dev/frameless/ports/option"
	"go.llib.dev/testcase/assert"
)

var _ option.Option[any] = option.Func[any](nil)

type SampleConfig struct {
	Foo  string
	Bar  int
	Baz  float64
	Func func() string
}

func (c *SampleConfig) Init() {
	c.Foo = "foo"
	c.Bar = 42
	c.Baz = 42.24
	c.Func = func() string { return "value" }
}

func (c SampleConfig) Configure(t *SampleConfig) {
	option.Configure(c, t)
}

func FooTo(v string) option.Option[SampleConfig] {
	return option.Func[SampleConfig](func(c *SampleConfig) {
		c.Foo = v
	})
}

func BazTo(v float64) option.Option[SampleConfig] {
	return option.Func[SampleConfig](func(c *SampleConfig) {
		c.Baz = v
	})
}

func BarTo(v int) option.Option[SampleConfig] {
	return option.Func[SampleConfig](func(c *SampleConfig) {
		c.Bar = v
	})
}

func TestUse(t *testing.T) {
	t.Run("smoke", func(t *testing.T) {
		opts := []option.Option[SampleConfig]{
			BazTo(128.821),
			BarTo(128),
		}
		c := option.Use[SampleConfig](opts)
		assert.Equal(t, c.Foo, "foo", "value taken from Init")
		assert.Equal(t, c.Bar, 128, "bar option used")
		assert.Equal(t, c.Baz, 128.821, "baz option used")
	})
	t.Run("init", func(t *testing.T) {
		var exp SampleConfig
		exp.Init()
		got := option.Use[SampleConfig]([]option.Option[SampleConfig](nil))
		assert.Equal(t, exp.Foo, got.Foo)
		assert.Equal(t, exp.Bar, got.Bar)
		assert.Equal(t, exp.Baz, got.Baz)
		assert.NotEmpty(t, exp.Func)
		assert.NotEmpty(t, got.Func)
		assert.Equal(t, exp.Func(), got.Func())

	})
	t.Run("options used", func(t *testing.T) {
		opts := []option.Option[SampleConfig]{FooTo("OOF")}
		c := option.Use[SampleConfig](opts)
		assert.Equal(t, c.Foo, "OOF")
	})
	t.Run("nil option values are ignored", func(t *testing.T) {
		opts := []option.Option[SampleConfig]{nil, FooTo("OOF")}
		c := option.Use[SampleConfig](opts)
		assert.Equal(t, c.Foo, "OOF")
	})
}

func TestConfigure(t *testing.T) {
	t.Run("on zero", func(t *testing.T) {
		receiver := SampleConfig{
			Foo: "foo",
			Bar: 42,
			Baz: 42.24,
		}

		var target SampleConfig
		option.Configure(receiver, &target)

		assert.Equal(t, target, SampleConfig{
			Foo: "foo",
			Bar: 42,
			Baz: 42.24,
		})
	})
	t.Run("target value is overwritten on non zero value", func(t *testing.T) {
		receiver := SampleConfig{
			Foo: "foo",
			Bar: 42,
			Baz: 42.24,
		}

		var target = SampleConfig{
			Foo: "oof",
			Bar: 24,
			Baz: 24.42,
		}
		option.Configure(receiver, &target)

		assert.Equal(t, target, SampleConfig{
			Foo: "foo",
			Bar: 42,
			Baz: 42.24,
		})
	})
	t.Run("when receiver value has zero value, then it won't overwrite the target config's field", func(t *testing.T) {
		receiver := SampleConfig{
			Foo: "foo",
			Baz: 42.24,
		}

		var target = SampleConfig{
			Bar: 24,
		}
		option.Configure(receiver, &target)

		assert.Equal(t, target, SampleConfig{
			Foo: "foo",
			Baz: 42.24,
			Bar: 24,
		})
	})

	t.Run("on non struct config type argument", func(t *testing.T) {
		v := assert.Panic(t, func() {
			option.Configure[string]("", pointer.Of(""))
		})
		assert.NotContain(t, v, "reflect:")
	})

	t.Run("using Configure as part of the config type's receiver works as expected", func(t *testing.T) {
		sc := option.Use[SampleConfig]([]option.Option[SampleConfig]{SampleConfig{
			Func: nil,
			Foo:  "foo-42",
		}})

		assert.NotEmpty(t, sc.Func)
		assert.Equal(t, sc.Foo, "foo-42")
		assert.Equal(t, sc.Bar, 42)
	})

	t.Run("nested configurations are respected", func(t *testing.T) {
		nc := option.Use[NestedConfig]([]option.Option[NestedConfig]{NestedConfig{
			SampleConfig: SampleConfig{
				Foo: "val",
			},
		}})

		assert.NotEmpty(t, nc)
		assert.NotEmpty(t, nc.SampleConfig)
		assert.NotEmpty(t, nc.SampleConfig.Func)
		assert.Equal(t, nc.SampleConfig.Foo, "val")
		assert.Equal(t, nc.SampleConfig.Bar, 42)
	})
}

type NestedConfig struct {
	SampleConfig
	Val string
}

func (c *NestedConfig) Init() {
	c.Val = "the answer"
	c.SampleConfig.Init()
}

func (c NestedConfig) Configure(t *NestedConfig) {
	option.Configure(c, t)
}
