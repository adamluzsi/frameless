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

type ContractSubject[Entity any, ID comparable] interface {
	crud.Creator[Entity]
	crud.Finder[Entity, ID]
	crud.Updater[Entity]
	crud.Deleter[ID]
}

type Option[Entity any, ID comparable] interface {
	option.Option[Config[Entity, ID]]
}

type Config[Entity any, ID comparable] struct {
	CRUD          crudcontracts.Config[Entity, ID]
	MetaAccessor  meta.MetaAccessor
	CommitManager comproto.OnePhaseCommitProtocol
}

func (c *Config[Entity, ID]) Init() {
	c.CRUD.Init()
}

func (c Config[Entity, ID]) Configure(t *Config[Entity, ID]) {
	option.Configure(c, t)
}

func Contract[Entity any, ID comparable](subject ContractSubject[Entity, ID], opts ...Option[Entity, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.Use[Config[Entity, ID]](opts)
	c.CRUD.SupportIDReuse = true
	c.CRUD.SupportRecreate = true

	if cm, ok := subject.(comproto.OnePhaseCommitProtocol); ok && c.CommitManager == nil {
		c.CommitManager = cm
	}

	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID](subject, c.CRUD),
		crudcontracts.Finder[Entity, ID](subject, c.CRUD),
		crudcontracts.Deleter[Entity, ID](subject, c.CRUD),
		crudcontracts.Updater[Entity, ID](subject, c.CRUD),
	)

	if c.CommitManager != nil {
		testcase.RunSuite(s,
			crudcontracts.OnePhaseCommitProtocol[Entity, ID](subject, c.CommitManager, c.CRUD),
			comprotocontracts.OnePhaseCommitProtocol(c.CommitManager, &comprotocontracts.Config{
				MakeContext: c.CRUD.MakeContext,
			}),
		)
	}

	if entrep, ok := subject.(cache.EntityRepository[Entity, ID]); ok && c.CommitManager != nil {
		testcase.RunSuite(s,
			cachecontracts.EntityRepository[Entity, ID](entrep, c.CommitManager,
				cachecontracts.Config[Entity, ID]{CRUD: c.CRUD}),
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
