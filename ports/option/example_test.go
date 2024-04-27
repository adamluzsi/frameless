package option_test

import (
	"fmt"

	"go.llib.dev/frameless/ports/option"
)

type config struct {
	Foo int
}

func (c *config) Init() {
	c.Foo = 42 // default value for Foo config
}

type Option interface {
	option.Option[config]
}

func FooIs(foo int) Option {
	return option.Func[config](func(c *config) {
		c.Foo = foo
	})
}

func FuncWithOptionalConfigurationInput(arg1 string, opts ...Option) string {
	conf := option.Use[config](opts)

	return fmt.Sprintf("Hello %s. (foo=%d)", arg1, conf.Foo)
}

func Example() {
	fmt.Println(
		FuncWithOptionalConfigurationInput("argument", FooIs(42)),
	)
}
