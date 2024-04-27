package option

import (
	"reflect"

	"go.llib.dev/frameless/pkg/reflectkit"
)

type Option[Config any] interface {
	Configure(*Config)
}

type Func[Config any] func(*Config)

func (fn Func[Config]) Configure(c *Config) { fn(c) }

type initializable interface {
	Init()
}

func Use[Config any, OPT Option[Config]](opts []OPT) Config {
	var c Config
	if ic, ok := any(&c).(initializable); ok {
		ic.Init()
	}
	for _, opt := range opts {
		if any(opt) == nil {
			continue
		}
		opt.Configure(&c)
	}
	return c
}

// Configure is a default implementation that can be used to implement the Option interface' Configure method.
func Configure[Config any](receiver Config, target *Config) {
	if target == nil {
		return
	}

	typ := reflectkit.TypeOf[Config]()
	self := reflect.ValueOf(receiver)
	oth := reflect.ValueOf(target)

	if typ.Kind() != reflect.Struct {
		panic("option.Configure currently only support Struct types")
	}

	for i, fnum := 0, typ.NumField(); i < fnum; i++ {
		sval := self.Field(i)
		oval := oth.Elem().Field(i)

		zero := reflect.New(typ.Field(i).Type).Elem()
		if !reflectkit.Equal(sval.Interface(), zero.Interface()) { // if target field is not zero
			oval.Set(sval)
		}
	}
}
