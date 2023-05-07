package env_test

import (
	"github.com/adamluzsi/frameless/pkg/env"
	"github.com/adamluzsi/frameless/pkg/logger"
	"os"
	"strconv"
	"time"
)

func ExampleLoad() {
	type ExampleAppConfig struct {
		Foo string        `env:"FOO"`
		Bar time.Duration `env:"BAR" default:"1h5m"`
		Baz int           `env:"BAZ" required:"true"`
	}

	var c ExampleAppConfig
	if err := env.Load(&c); err != nil {
		logger.Fatal(nil, "failed to load application config", logger.ErrField(err))
		os.Exit(1)
	}
}

func ExampleLoad_enum() {
	type ExampleAppConfig struct {
		Foo string `env:"FOO" enum:"foo;bar;baz;" default:"foo"`
	}

	var c ExampleAppConfig
	if err := env.Load(&c); err != nil {
		logger.Fatal(nil, "failed to load application config", logger.ErrField(err))
		os.Exit(1)
	}
}

func ExampleRegisterParser() {
	type MyCustomInt int

	var _ = env.RegisterParser(func(envValue string) (MyCustomInt, error) {
		// try parse hex
		v, err := strconv.ParseInt(envValue, 16, 64)
		if err == nil {
			return MyCustomInt(v), nil
		}

		// then let's try parse it as base 10 int
		v, err = strconv.ParseInt(envValue, 10, 64)
		if err == nil {
			return MyCustomInt(v), nil
		}
		return 0, err
	})

	type ExampleAppConfig struct {
		Foo MyCustomInt `env:"FOO" required:"true"`
	}

	var c ExampleAppConfig
	_ = env.Load(&c) // handle error
}
