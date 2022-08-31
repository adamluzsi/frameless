package memory_test

import (
	"context"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/meta"
	"github.com/adamluzsi/testcase/assert"
	"testing"
	"time"

	"github.com/adamluzsi/frameless/ports/crud/cache"
	"github.com/adamluzsi/frameless/ports/crud/cache/contracts"
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

type ContractSubject[Ent, ID any] struct {
	Resource interface {
		crud.Creator[Ent]
		crud.Finder[Ent, ID]
		crud.Updater[Ent]
		crud.Deleter[ID]
	}
	//frameless.CreatorPublisher
	//frameless.UpdaterPublisher
	//frameless.DeleterPublisher
	EntityStorage cache.EntityStorage[Ent, ID]
	CommitManager comproto.OnePhaseCommitProtocol
	meta.MetaAccessor
}

func GetContracts[Ent, ID any](
	subject func(testing.TB) ContractSubject[Ent, ID],
	makeCtx func(testing.TB) context.Context,
	makeEnt func(testing.TB) Ent,
) []testcase.Suite {
	return []testcase.Suite{
		crudcontracts.Creator[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.CreatorSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		crudcontracts.Finder[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.FinderSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		crudcontracts.Updater[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.UpdaterSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		crudcontracts.Deleter[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.DeleterSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		//resource.CreatorPublisher{T: T,
		//	Subject: func(tb testing.TB) resource.CreatorPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		//resource.UpdaterPublisher{T: T,
		//	Subject: func(tb testing.TB) resource.UpdaterPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		//resource.DeleterPublisher{T: T,
		//	Subject: func(tb testing.TB) resource.DeleterPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		crudcontracts.OnePhaseCommitProtocol[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Ent, ID] {
				s := subject(tb)
				return crudcontracts.OnePhaseCommitProtocolSubject[Ent, ID]{
					Resource:      s.Resource,
					CommitManager: s.CommitManager,
				}
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		cachecontracts.EntityStorage[Ent, ID]{
			Subject: func(tb testing.TB) (cache.EntityStorage[Ent, ID], comproto.OnePhaseCommitProtocol) {
				s := subject(tb)
				return s.EntityStorage, s.CommitManager
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		//resource.MetaAccessor{T: T, V: "string",
		//	Subject: func(tb testing.TB) resource.MetaAccessorSubject {
		//		s := subject(tb)
		//		return resource.MetaAccessorSubject{
		//			MetaAccessor: s.MetaAccessor,
		//			CRD:          s,
		//			Publisher:    s.Publisher,
		//		}
		//	},
		//	FixtureFactory: ff,
		//	Context:        cf,
		//},
	}
}
