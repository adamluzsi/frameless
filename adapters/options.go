package adapters

type Option interface {
	Configure(options)
}

func setupOptions(os []Option) *options {
	return &options{}
}

type options struct {
}
