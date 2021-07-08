package inmemory_test

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/cache"
	cachecontracts "github.com/adamluzsi/frameless/cache/contracts"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"testing"
	"time"

	"github.com/adamluzsi/testcase"
)

var (
	waiter = testcase.Waiter{
		WaitDuration: time.Millisecond,
		WaitTimeout:  3 * time.Second,
	}
	retry = testcase.Retry{
		Strategy: waiter,
	}
)

// Entity is an example entity that can be used for testing
type TestEntity struct {
	ID   string `ext:"id"`
	Data string
	List []string
}

type ContractSubject struct {
	frameless.Creator
	frameless.Finder
	frameless.Updater
	frameless.Deleter
	//frameless.CreatorPublisher
	//frameless.UpdaterPublisher
	//frameless.DeleterPublisher
	EntityStorage cache.EntityStorage
	frameless.OnePhaseCommitProtocol
	frameless.MetaAccessor
}

func GetContracts(T frameless.T, subject func(testing.TB) ContractSubject) []testcase.Contract {
	fff := func(tb testing.TB) contracts.FixtureFactory {
		return fixtures.NewFactory(tb)
	}
	return []testcase.Contract{
		contracts.Creator{T: T,
			Subject: func(tb testing.TB) contracts.CRD {
				return subject(tb)
			},
			FixtureFactory: fff,
		},
		contracts.Finder{T: T,
			Subject: func(tb testing.TB) contracts.CRD {
				return subject(tb)
			},
			FixtureFactory: fff,
		},
		contracts.Updater{T: T,
			Subject: func(tb testing.TB) contracts.UpdaterSubject {
				return subject(tb)
			},
			FixtureFactory: fff,
		},
		contracts.Deleter{T: T,
			Subject: func(tb testing.TB) contracts.CRD {
				return subject(tb)
			},
			FixtureFactory: fff,
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
		contracts.OnePhaseCommitProtocol{T: T,
			Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
				s := subject(tb)
				return s.OnePhaseCommitProtocol, s
			},
			FixtureFactory: fff,
		},
		cachecontracts.EntityStorage{T: T,
			Subject: func(tb testing.TB) (cache.EntityStorage, frameless.OnePhaseCommitProtocol) {
				s := subject(tb)
				return s.EntityStorage, s.OnePhaseCommitProtocol
			}, FixtureFactory: fff,
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
		//},
	}
}
