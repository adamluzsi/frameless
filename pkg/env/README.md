# Package `env`

Working with OS environment variables is beneficial because they provide a simple,
uniform, and platform-agnostic way to manage application configurations.
By storing configuration settings in the environment, developers can separate code from configuration,
promoting codebase portability and reducing the risk of sensitive data leakage.

The `env` package adheres to the principles of the 12-factor app principle's configuration section
and helps making configurations more accessible in your application
and providing a structured way to load these configuration values into your application.

## Key Features

- Load environment variables into configuration structure based on its env tags.
- typesafe environment variable Lookup
- Support for default values using `default` or `env-default` tags.
- configurable list separator using `separator` or `env-separator` tags.
- Support for required fields using `required`/`require` or `env-required`/`env-require` tags.
- Support for configuring what layout we need to use to parse a time from the environment value
  with `env-time-layout`/`layout`
- Custom parsers for specific types can be registered using the RegisterParser function.
- Built-in support for loading string, int, float, boolean and time.Duration types.
- Nested structs are also visited and loaded with environment variables.
- integrates with `frameless/pkg/enum` package

## Examples

`Lookup[T any]`: typesafe environment variable Lookup.

```go
package main

func main() {
	val, ok, err := env.Lookup[string]("FOO", env.DefaultValue("foo"))
	_, _, _ = val, ok, err
}

```

`Load[T any](ptr *T) error`: Loads environment variables into the struct fields based on the field tags.

```go
package main

import "net/url"

type ExampleAppConfig struct {
	Foo     string        `env:"FOO"`
	Bar     time.Duration `env:"BAR" default:"1h5m"`
	Baz     int           `env:"BAZ" enum:"1;2;3;""`
	Qux     float64       `env:"QUX" required:"true"`
	Quux    MyCustomInt   `env:"QUUX"`
	Quuz    time.Time     `env:"QUUZ" layout:"2006-01-02"`
	Corge   *string       `env:"CORGE" required:"false"`
	BaseURL *url.URL      `env:"BASE_URL"`
	Port    int           `env:"PORT" default:"8080"`
}

func main() {
	var c ExampleAppConfig
	if err := env.Load(&c); err != nil {
		logger.Fatal(nil, "failed to load application config", logger.ErrField(err))
		os.Exit(1)
	}
}

```

`RegisterParser[T any]`: Registers custom parsers for specific types.

```go
package main

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
```
