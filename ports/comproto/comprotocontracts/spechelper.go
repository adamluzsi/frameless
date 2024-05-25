package comprotocontracts

import (
	"context"

	"go.llib.dev/frameless/pkg/zerokit"
	"go.llib.dev/frameless/ports/option"
)

type Option interface {
	option.Option[Config]
}

type Config struct {
	MakeContext func() context.Context
}

func (c *Config) Init() {
	c.MakeContext = context.Background
}

func (c *Config) Configure(oth *Config) {
	oth.MakeContext = zerokit.Coalesce(oth.MakeContext, c.MakeContext)
}
