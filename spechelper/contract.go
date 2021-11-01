package spechelper

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/testcase"
)

type Contract struct {
	T, V           frameless.T
	Subject        func(testing.TB) ContractSubject
	Context        func(testing.TB) context.Context
	FixtureFactory func(testing.TB) frameless.FixtureFactory
}

type ContractSubject struct {
	CRUD interface {
		CRUD
		Publisher
	}
	frameless.MetaAccessor
	frameless.OnePhaseCommitProtocol
}

type CRUD interface {
	frameless.Creator
	frameless.Finder
	frameless.Updater
	frameless.Deleter
}

type Publisher interface {
	frameless.CreatorPublisher
	frameless.UpdaterPublisher
	frameless.DeleterPublisher
}

type Subscriber interface {
	frameless.CreatorSubscriber
	frameless.UpdaterSubscriber
	frameless.DeleterSubscriber
}

func (c Contract) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Contract) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Contract) Spec(s *testcase.Spec) {
	testcase.RunContract(s,
		contracts.Creator{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
			Context:        c.Context,
		},
		contracts.Finder{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
			Context:        c.Context,
		},
		contracts.Deleter{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
			Context:        c.Context,
		},
		contracts.Updater{T: c.T,
			Subject: func(tb testing.TB) contracts.UpdaterSubject {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
			Context:        c.Context,
		},
		contracts.Publisher{T: c.T,
			Subject: func(tb testing.TB) contracts.PublisherSubject {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
			Context:        c.Context,
		},
		contracts.MetaAccessor{T: c.T, V: c.V,
			Subject: func(tb testing.TB) contracts.MetaAccessorSubject {
				subject := c.Subject(tb)
				return contracts.MetaAccessorSubject{
					MetaAccessor: subject.MetaAccessor,
					Resource:     subject.CRUD,
					Publisher:    subject.CRUD,
				}
			},
			FixtureFactory: c.FixtureFactory,
			Context:        c.Context,
		},
		contracts.OnePhaseCommitProtocol{T: c.T,
			Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
				subject := c.Subject(tb)
				return subject.OnePhaseCommitProtocol, subject.CRUD
			},
			FixtureFactory: c.FixtureFactory,
			Context:        c.Context,
		},
	)
}
