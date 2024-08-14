package env_test

import (
	"time"

	"go.llib.dev/frameless/pkg/env"
)

func ExampleLoad() {
	type ExampleAppConfig struct {
		Foo string        `env:"FOO"`
		Bar time.Duration `env:"BAR" default:"1h5m"`
		Baz int           `env:"BAZ" required:"true"`
	}

	var c ExampleAppConfig
	if err := env.Load(&c); err != nil {
		_ = err
	}
}

func ExampleLoad_withEnvKeyBackwardCompatibility() {
	type ExampleAppConfig struct {
		// Foo supports both FOO env key and also OLDFOO env key
		Foo string `env:"FOO,OLDFOO"`
	}

	var c ExampleAppConfig
	if err := env.Load(&c); err != nil {
		_ = err
	}
}

func ExampleLoad_enum() {
	type ExampleAppConfig struct {
		Foo string `env:"FOO" enum:"foo;bar;baz;" default:"foo"`
	}

	var c ExampleAppConfig
	if err := env.Load(&c); err != nil {
		_ = err
	}
}

func ExampleLookup() {
	val, ok, err := env.Lookup[string]("FOO", env.DefaultValue("foo"))
	_, _, _ = val, ok, err
}

func ExampleLoad_withDefaultValue() {
	type ExampleAppConfig struct {
		Foo string `env:"FOO" default:"foo"`
	}
}

func ExampleLoad_withTimeLayout() {
	type ExampleAppConfig struct {
		Foo time.Time `env:"FOO" layout:"2006-01-02"`
	}
}
