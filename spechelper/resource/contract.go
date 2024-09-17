package resource

import (
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/comproto/comprotocontracts"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontracts"
	"go.llib.dev/frameless/port/meta"
	"go.llib.dev/frameless/port/meta/metacontracts"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
)

type ContractSubject[ENT any, ID comparable] interface {
	crud.Creator[ENT]
	crud.Finder[ENT, ID]
	crud.Updater[ENT]
	crud.Deleter[ID]
	crud.Saver[ENT]
}

type Option[ENT any, ID comparable] interface {
	option.Option[Config[ENT, ID]]
}

type Config[ENT any, ID comparable] struct {
	CRUD          crudcontracts.Config[ENT, ID]
	MetaAccessor  meta.MetaAccessor
	CommitManager comproto.OnePhaseCommitProtocol
}

func (c *Config[ENT, ID]) Init() {
	c.CRUD.Init()
}

func (c Config[ENT, ID]) Configure(t *Config[ENT, ID]) {
	option.Configure(c, t)
}

func Contract[ENT any, ID comparable](subject ContractSubject[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[ENT, ID]](opts)
	c.CRUD.SupportIDReuse = true
	c.CRUD.SupportRecreate = true

	if cm, ok := subject.(comproto.OnePhaseCommitProtocol); ok && c.CommitManager == nil {
		c.CommitManager = cm
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[ENT, ID](subject, c.CRUD),
		crudcontracts.Finder[ENT, ID](subject, c.CRUD),
		crudcontracts.Deleter[ENT, ID](subject, c.CRUD),
		crudcontracts.Updater[ENT, ID](subject, c.CRUD),
		crudcontracts.Saver[ENT, ID](subject, c.CRUD),
	)

	if c.CommitManager != nil {
		testcase.RunSuite(s,
			crudcontracts.OnePhaseCommitProtocol[ENT, ID](subject, c.CommitManager, c.CRUD),
			comprotocontracts.OnePhaseCommitProtocol(c.CommitManager, &comprotocontracts.Config{
				MakeContext: c.CRUD.MakeContext,
			}),
		)
	}

	if entrep, ok := subject.(cache.EntityRepository[ENT, ID]); ok && c.CommitManager != nil {
		testcase.RunSuite(s,
			cachecontracts.EntityRepository[ENT, ID](entrep, c.CommitManager,
				cachecontracts.Config[ENT, ID]{CRUD: c.CRUD}),
		)
	}

	if c.MetaAccessor != nil {
		testcase.RunSuite(s,
			metacontracts.MetaAccessor[bool](c.MetaAccessor),
			metacontracts.MetaAccessor[string](c.MetaAccessor),
			metacontracts.MetaAccessor[int](c.MetaAccessor),
		)
	}

	return s.AsSuite("Contract")
}
