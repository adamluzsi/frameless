package cachecontracts

import (
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/option"
)

type Option[E any, ID comparable] interface {
	option.Option[Config[E, ID]]
}

type Config[E any, ID comparable] struct {
	CRUD crudcontracts.Config[E, ID]

	MakeCache func(cache.Source[E, ID], cache.Repository[E, ID]) CacheSubject[E, ID]
}

func (c *Config[E, ID]) Init() {
	c.CRUD.Init()
}

func (c Config[E, ID]) Configure(t *Config[E, ID]) {
	option.Configure(c, t)
}

func CRUDOption[E any, ID comparable](opts ...crudcontracts.Option[E, ID]) Option[E, ID] {
	return option.Func[Config[E, ID]](func(c *Config[E, ID]) {
		t := option.Use[crudcontracts.Config[E, ID]](opts)
		c.CRUD.Configure(&t)
		c.CRUD = t
	})
}
