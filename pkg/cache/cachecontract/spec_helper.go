package cachecontract

import (
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
)

type Option[ENT any, ID comparable] interface {
	option.Option[Config[ENT, ID]]
}

type Config[ENT any, ID comparable] struct {
	// MakeID will help  create a valid ENT.ID.
	// In the cache, we work with entities which suppose to be already stored somewhere else
	// so the default use-case for testing the cache.EntityRepository is that entities already have a populated ID.
	// If a randomly generated value not good, you can overwrite this function.
	MakeID func(tb testing.TB) ID
	// CRUD [optional] contracts related configuration.
	CRUD crudcontract.Config[ENT, ID]
}

func (c *Config[E, ID]) Init() {
	c.CRUD.Init()

	if c.MakeID == nil {
		c.MakeID = func(tb testing.TB) ID {
			tc := testcase.ToT(&tb)
			return tc.Random.Make(reflectkit.TypeOf[ID]()).(ID)
		}
	}
}

func (c Config[E, ID]) Configure(t *Config[E, ID]) {
	*t = reflectkit.MergeStruct(*t, c)
}

func CRUDOption[E any, ID comparable](opts ...crudcontract.Option[E, ID]) Option[E, ID] {
	return option.Func[Config[E, ID]](func(c *Config[E, ID]) {
		t := option.ToConfig[crudcontract.Config[E, ID]](opts)
		c.CRUD.Configure(&t)
		c.CRUD = t
	})
}
