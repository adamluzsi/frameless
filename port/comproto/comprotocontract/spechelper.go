package comprotocontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/port/option"
)

type Option interface {
	option.Option[Config]
}

type Config struct {
	MakeContext func(testing.TB) context.Context
}

func (c *Config) Init() {
	c.MakeContext = func(t testing.TB) context.Context {
		return context.Background()
	}
}

func (c *Config) Configure(oth *Config) {
	oth.MakeContext = zerokit.Coalesce(oth.MakeContext, c.MakeContext)
}
