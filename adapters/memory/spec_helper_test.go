package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	cachecontracts "github.com/adamluzsi/frameless/ports/crud/cache/contracts"
	"github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/meta"
	"github.com/adamluzsi/testcase/assert"

	"github.com/adamluzsi/frameless/ports/crud/cache"
	"github.com/adamluzsi/testcase"
)

var (
	waiter = assert.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:      3 * time.Second,
	}
	eventually = assert.Eventually{
		RetryStrategy: waiter,
	}
)

// Entity is an example entity that can be used for testing
type TestEntity struct {
	ID   string `ext:"id"`
	Data string
	List []string
}

func makeTestEntity(tb testing.TB) TestEntity {
	t := tb.(*testcase.T)
	var list []string
	n := t.Random.IntBetween(1, 3)
	for i := 0; i < n; i++ {
		list = append(list, t.Random.String())
	}
	return TestEntity{
		Data: t.Random.String(),
		List: list,
	}
}

func makeContext(tb testing.TB) context.Context {
	return context.Background()
}

type ContractSubject[Entity, ID any] struct {
	Resource interface {
		crud.Creator[Entity]
		crud.Finder[Entity, ID]
		crud.Updater[Entity]
		crud.Deleter[ID]
	}
	//frameless.CreatorPublisher
	//frameless.UpdaterPublisher
	//frameless.DeleterPublisher
	EntityRepository cache.EntityRepository[Entity, ID]
	CommitManager    comproto.OnePhaseCommitProtocol
	meta.MetaAccessor
}

func GetContracts[Entity, ID any](
	subject func(testing.TB) ContractSubject[Entity, ID],
	MakeContext func(testing.TB) context.Context,
	MakeEntity func(testing.TB) Entity,
) []testcase.Suite {
	return []testcase.Suite{
		crudcontracts.Creator[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
				return subject(tb).Resource
			},
			MakeContext: MakeContext,
			MakeEntity:  MakeEntity,

			SupportIDReuse: true,
		},
		crudcontracts.Finder[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
				return subject(tb).Resource.(crudcontracts.FinderSubject[Entity, ID])
			},
			MakeContext: MakeContext,
			MakeEntity:  MakeEntity,
		},
		crudcontracts.Updater[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
				return subject(tb).Resource
			},
			MakeContext: MakeContext,
			MakeEntity:  MakeEntity,
		},
		crudcontracts.Deleter[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
				return subject(tb).Resource
			},
			MakeContext: MakeContext,
			MakeEntity:  MakeEntity,
		},
		//resource.CreatorPublisher{T: T,
		//	MakeSubject: func(tb testing.TB) resource.CreatorPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		//resource.UpdaterPublisher{T: T,
		//	MakeSubject: func(tb testing.TB) resource.UpdaterPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		//resource.DeleterPublisher{T: T,
		//	MakeSubject: func(tb testing.TB) resource.DeleterPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		crudcontracts.OnePhaseCommitProtocol[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
				s := subject(tb)
				return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
					Resource:      s.Resource,
					CommitManager: s.CommitManager,
				}
			},
			MakeContext: MakeContext,
			MakeEntity:  MakeEntity,
		},
		cachecontracts.EntityRepository[Entity, ID]{
			MakeSubject: func(tb testing.TB) (cache.EntityRepository[Entity, ID], comproto.OnePhaseCommitProtocol) {
				s := subject(tb)
				return s.EntityRepository, s.CommitManager
			},
			MakeContext: MakeContext,
			MakeEntity:  MakeEntity,
		},
		//resource.MetaAccessor{T: T, V: "string",
		//	MakeSubject: func(tb testing.TB) resource.MetaAccessorSubject {
		//		s := subject(tb)
		//		return resource.MetaAccessorSubject{
		//			MetaAccessor: s.MetaAccessor,
		//			CRD:          s,
		//			Publisher:    s.Publisher,
		//		}
		//	},
		//	FixtureFactory: ff,
		//	MakeContext:        cf,
		//},
	}
}
