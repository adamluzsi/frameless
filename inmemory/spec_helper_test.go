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

type Resource interface {
	frameless.Creator
	frameless.Finder
	frameless.Updater
	frameless.Deleter
	//frameless.CreatorPublisher
	//frameless.UpdaterPublisher
	//frameless.DeleterPublisher
	cache.EntityStorage
}

func GetContracts(T frameless.T, subject func(testing.TB) (Resource, frameless.OnePhaseCommitProtocol)) []testcase.Contract {
	ff := fixtures.Factory
	return []testcase.Contract{
		contracts.Creator{T: T,
			Subject: func(tb testing.TB) contracts.CRD {
				resource, _ := subject(tb)
				return resource
			},
			FixtureFactory: ff,
		},
		contracts.Finder{T: T,
			Subject: func(tb testing.TB) contracts.CRD {
				resource, _ := subject(tb)
				return resource
			},
			FixtureFactory: ff,
		},
		contracts.Updater{T: T,
			Subject: func(tb testing.TB) contracts.UpdaterSubject {
				resource, _ := subject(tb)
				return resource
			},
			FixtureFactory: ff,
		},
		contracts.Deleter{T: T,
			Subject: func(tb testing.TB) contracts.CRD {
				resource, _ := subject(tb)
				return resource
			},
			FixtureFactory: ff,
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
				resource, cpm := subject(tb)
				return cpm, resource
			},
			FixtureFactory: ff,
		},
		cachecontracts.EntityStorage{T: T,
			Subject: func(tb testing.TB) (cache.EntityStorage, frameless.OnePhaseCommitProtocol) {
				resource, cpm := subject(tb)
				return resource, cpm
			}, FixtureFactory: ff,
		},
	}
}
