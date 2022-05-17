package inmemory_test

import (
	"context"
	"testing"
	"time"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/cache"
	cachecontracts "github.com/adamluzsi/frameless/cache/contracts"
	"github.com/adamluzsi/frameless/contracts"

	"github.com/adamluzsi/testcase"
)

var (
	waiter = testcase.Waiter{
		WaitDuration: time.Millisecond,
		Timeout:  3 * time.Second,
	}
	eventually = testcase.Eventually{
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
		frameless.Creator[Ent]
		frameless.Finder[Ent, ID]
		frameless.Updater[Ent]
		frameless.Deleter[ID]
	}
	//frameless.CreatorPublisher
	//frameless.UpdaterPublisher
	//frameless.DeleterPublisher
	EntityStorage cache.EntityStorage[Ent, ID]
	CommitManager frameless.OnePhaseCommitProtocol
	frameless.MetaAccessor
}

func GetContracts[Ent, ID any](
	subject func(testing.TB) ContractSubject[Ent, ID],
	makeCtx func(testing.TB) context.Context,
	makeEnt func(testing.TB) Ent,
) []testcase.Contract {
	return []testcase.Contract{
		contracts.Creator[Ent, ID]{
			Subject: func(tb testing.TB) contracts.CreatorSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		contracts.Finder[Ent, ID]{
			Subject: func(tb testing.TB) contracts.FinderSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		contracts.Updater[Ent, ID]{
			Subject: func(tb testing.TB) contracts.UpdaterSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		contracts.Deleter[Ent, ID]{
			Subject: func(tb testing.TB) contracts.DeleterSubject[Ent, ID] {
				return subject(tb).Resource
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		//contracts.CreatorPublisher{T: T,
		//	Subject: func(tb testing.TB) contracts.CreatorPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		//contracts.UpdaterPublisher{T: T,
		//	Subject: func(tb testing.TB) contracts.UpdaterPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		//contracts.DeleterPublisher{T: T,
		//	Subject: func(tb testing.TB) contracts.DeleterPublisherSubject {
		//		resource, _ := ns(tb)
		//		return resource
		//	},
		//	FixtureFactory: ff,
		//},
		contracts.OnePhaseCommitProtocol[Ent, ID]{
			Subject: func(tb testing.TB) contracts.OnePhaseCommitProtocolSubject[Ent, ID] {
				s := subject(tb)
				return contracts.OnePhaseCommitProtocolSubject[Ent, ID]{
					Resource:      s.Resource,
					CommitManager: s.CommitManager,
				}
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		cachecontracts.EntityStorage[Ent, ID]{
			Subject: func(tb testing.TB) (cache.EntityStorage[Ent, ID], frameless.OnePhaseCommitProtocol) {
				s := subject(tb)
				return s.EntityStorage, s.CommitManager
			},
			MakeCtx: makeCtx,
			MakeEnt: makeEnt,
		},
		//contracts.MetaAccessor{T: T, V: "string",
		//	Subject: func(tb testing.TB) contracts.MetaAccessorSubject {
		//		s := subject(tb)
		//		return contracts.MetaAccessorSubject{
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
