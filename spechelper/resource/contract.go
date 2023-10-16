package resource

import (
	"context"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/ports/comproto/comprotocontracts"
	"testing"

	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/crud"
	"go.llib.dev/frameless/ports/crud/crudcontracts"
	"go.llib.dev/frameless/ports/meta"
	"go.llib.dev/frameless/ports/meta/metacontracts"
	"go.llib.dev/testcase"
)

type Contract[Entity, ID any] func(testing.TB) ContractSubject[Entity, ID]

type ContractSubject[Entity, ID any] struct {
	Resource interface {
		crud.Creator[Entity]
		crud.Finder[Entity, ID]
		crud.Updater[Entity]
		crud.Deleter[ID]
	}
	MetaAccessor  meta.MetaAccessor
	CommitManager comproto.OnePhaseCommitProtocol

	MakeContext func() context.Context
	MakeEntity  func() Entity
}

func (c Contract[Entity, ID]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Contract[Entity, ID]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Contract[Entity, ID]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID](func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
			sub := c(tb)
			return crudcontracts.CreatorSubject[Entity, ID]{
				Resource:        sub.Resource,
				MakeContext:     sub.MakeContext,
				MakeEntity:      sub.MakeEntity,
				SupportIDReuse:  true,
				SupportRecreate: true,
			}
		}),
		crudcontracts.Finder[Entity, ID](func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
			sub := c(tb)
			return crudcontracts.FinderSubject[Entity, ID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}),
		crudcontracts.Deleter[Entity, ID](func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
			sub := c(tb)
			return crudcontracts.DeleterSubject[Entity, ID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}),
		crudcontracts.Updater[Entity, ID](func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
			sub := c(tb)
			return crudcontracts.UpdaterSubject[Entity, ID]{
				Resource:    sub.Resource,
				MakeContext: sub.MakeContext,
				MakeEntity:  sub.MakeEntity,
			}
		}),
		metacontracts.MetaAccessor[bool](func(tb testing.TB) metacontracts.MetaAccessorSubject[bool] {
			sub := c(tb)
			return metacontracts.MetaAccessorSubject[bool]{
				MetaAccessor: sub.MetaAccessor,
				MakeContext:  sub.MakeContext,
				MakeV:        testcase.ToT(&tb).Random.Bool,
			}
		}),
		metacontracts.MetaAccessor[string](func(tb testing.TB) metacontracts.MetaAccessorSubject[string] {
			sub := c(tb)
			return metacontracts.MetaAccessorSubject[string]{
				MetaAccessor: sub.MetaAccessor,
				MakeContext:  sub.MakeContext,
				MakeV:        testcase.ToT(&tb).Random.String,
			}
		}),
		metacontracts.MetaAccessor[int](func(tb testing.TB) metacontracts.MetaAccessorSubject[int] {
			sub := c(tb)
			return metacontracts.MetaAccessorSubject[int]{
				MetaAccessor: sub.MetaAccessor,
				MakeContext:  sub.MakeContext,
				MakeV:        testcase.ToT(&tb).Random.Int,
			}
		}),
		crudcontracts.OnePhaseCommitProtocol[Entity, ID](func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
			sub := c(tb)
			return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
				Resource:      sub.Resource,
				CommitManager: sub.CommitManager,
				MakeContext:   sub.MakeContext,
				MakeEntity:    sub.MakeEntity,
			}
		}),
		comprotocontracts.OnePhaseCommitProtocol(func(tb testing.TB) comprotocontracts.OnePhaseCommitProtocolSubject {
			sub := c(tb)
			return comprotocontracts.OnePhaseCommitProtocolSubject{
				CommitManager: sub.CommitManager,
				MakeContext:   sub.MakeContext,
			}
		}),
		cachecontracts.EntityRepository[Entity, ID](func(tb testing.TB) cachecontracts.EntityRepositorySubject[Entity, ID] {
			sub := c(tb)
			res, ok := sub.Resource.(cache.EntityRepository[Entity, ID])
			if !ok {
				tb.Skip()
			}
			return cachecontracts.EntityRepositorySubject[Entity, ID]{
				EntityRepository: res,
				CommitManager:    sub.CommitManager,
				MakeContext:      sub.MakeContext,
				MakeEntity:       sub.MakeEntity,
				ChangeEntity:     nil,
			}
		}),
	)
}
