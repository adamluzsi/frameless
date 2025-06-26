package resource

import (
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontract"
	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/comproto/comprotocontract"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/meta"
	"go.llib.dev/frameless/port/meta/metacontract"
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
	CRUD          crudcontract.Config[ENT, ID]
	MetaAccessor  meta.MetaAccessor
	CommitManager comproto.OnePhaseCommitProtocol
}

func (c *Config[ENT, ID]) Init() {
	c.CRUD.Init()
}

func (c Config[ENT, ID]) Configure(t *Config[ENT, ID]) {
	*t = reflectkit.MergeStruct(*t, c)
}

func Contract[ENT any, ID comparable](subject ContractSubject[ENT, ID], opts ...Option[ENT, ID]) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config[ENT, ID]](opts)
	c.CRUD.SupportIDReuse = true
	c.CRUD.SupportRecreate = true

	if cm, ok := subject.(comproto.OnePhaseCommitProtocol); ok && c.CommitManager == nil {
		c.CommitManager = cm
	}

	testcase.RunSuite(s,
		crudcontract.Creator[ENT, ID](subject, c.CRUD),
		crudcontract.Finder[ENT, ID](subject, c.CRUD),
		crudcontract.Deleter[ENT, ID](subject, c.CRUD),
		crudcontract.Updater[ENT, ID](subject, c.CRUD),
		crudcontract.Saver[ENT, ID](subject, c.CRUD),
	)

	if c.CommitManager != nil {
		testcase.RunSuite(s,
			crudcontract.OnePhaseCommitProtocol[ENT, ID](subject, c.CommitManager, c.CRUD),
			comprotocontract.OnePhaseCommitProtocol(c.CommitManager, &comprotocontract.Config{
				MakeContext: c.CRUD.MakeContext,
			}),
		)
	}

	if entrep, ok := subject.(cache.EntityRepository[ENT, ID]); ok && c.CommitManager != nil {
		testcase.RunSuite(s,
			cachecontract.EntityRepository[ENT, ID](entrep, c.CommitManager,
				cachecontract.Config[ENT, ID]{CRUD: c.CRUD}),
		)
	}

	if c.MetaAccessor != nil {
		testcase.RunSuite(s,
			metacontract.MetaAccessor[bool](c.MetaAccessor),
			metacontract.MetaAccessor[string](c.MetaAccessor),
			metacontract.MetaAccessor[int](c.MetaAccessor),
		)
	}

	return s.AsSuite("Contract")
}
