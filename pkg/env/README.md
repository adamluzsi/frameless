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
  Without a separator tag, list elements are separated by `,`.
  A list element can contain the separator when it is escaped (`a\,b,c`),
  or when the value is a JSON array (`["a,b","c"]`).
- Support for required fields using `required`/`require` or `env-required`/`env-require` tags.
- Support for configuring what layout we need to use to parse a time from the environment value
  with `env-time-layout`/`layout`
- Custom parsers for specific types can be registered using the RegisterParser function.
- Built-in support for loading string, int, float, boolean and time.Duration types.
- Nested structs are also visited and loaded with environment variables.
  - A struct field with an `env` tag of its own takes a JSON object too,
    which is merged onto what its inner fields loaded.
    See [Struct fields with a JSON value](#struct-fields-with-a-json-value).
  - Tag a struct field with `opaque:""` (alias `norecurse:""`) to stop the env package from looking inside it,
    e.g. when its type has `env` or `default` tags meant for something else.
    With an `env` tag of its own, the field is still loaded, from a JSON value.
  - `opaque:"false"` turns it off, and any other non-bool value panics.
  - The tag is shared with `frameless/pkg/cli`.
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
		logger.Fatal(nil, "failed to load application config", logging.ErrField(err))
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

## Struct fields with a JSON value

A struct field is loaded through its inner fields' tags.
When the struct field has an `env` tag of its own, its env var can hold a JSON object as well:

```go
type DB struct {
	Host  string `env:"DB_HOST" default:"localhost"`
	Port  int    `env:"DB_PORT" default:"5432"`
	Token string `env:"DB_TOKEN" required:"true"`
}

type Config struct {
	DB DB `env:"DB"` // DB='{"Host":"db.internal","Token":"secret"}'
}
```

The values are loaded in this order, and a later one overwrites an earlier one:

1. the defaults of the inner fields
2. the `default` tag of the struct field, when it holds a JSON object
3. the env vars of the inner fields
4. the env var of the struct field

So a default never overwrites an env var.
A JSON object is merged onto the field's value, so the inner fields it doesn't mention keep what they loaded.
With `DB='{"Port":6543}'` and `DB_HOST=db.internal`, the field is `{db.internal 6543 ...}`.
An invalid JSON value fails with `env.ErrInvalidValue`.

The `required` tags are checked once everything is loaded.
A required inner field without an env var of its own is satisfied
when the JSON object of its struct field, from the env var or the `default` tag, gives it a non-zero value.
An env var that is present satisfies required even when it is empty (`DB_TOKEN=""`).
The validation tags, like `enum`, are checked once everything is loaded as well.
