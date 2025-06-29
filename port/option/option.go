package option

type Option[Config any] interface {
	// Configure will configure an option.
	Configure(*Config)
}

// Func (option.Func[Config]) is a default implementation for creating options.
type Func[Config any] func(*Config)

func (fn Func[Config]) Configure(c *Config) { fn(c) }

// func ToConfig[Config any, Opt Option[Config]](opts ...Opt) Config {
func ToConfig[Config any, Opt Option[Config]](opts []Opt) Config {
	var c Config
	if init, ok := any(&c).(initer); ok {
		init.Init()
	}
	for _, opt := range opts {
		opt.Configure(&c)
	}
	return c
}

type initer interface {
	Init()
}
