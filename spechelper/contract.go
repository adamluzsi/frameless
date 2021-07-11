package spechelper

import (
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

type Contract struct {
	T, V           frameless.T
	Subject        func(testing.TB) ContractSubject
	FixtureFactory func(testing.TB) contracts.FixtureFactory
}

type ContractSubject struct {
	frameless.MetaAccessor
	frameless.OnePhaseCommitProtocol
	CRUD interface {
		frameless.Creator
		frameless.Finder
		frameless.Updater
		frameless.Deleter
		frameless.Publisher
	}
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
		},
		contracts.Finder{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.Deleter{T: c.T,
			Subject: func(tb testing.TB) contracts.CRD {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.Updater{T: c.T,
			Subject: func(tb testing.TB) contracts.UpdaterSubject {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.Publisher{T: c.T,
			Subject: func(tb testing.TB) contracts.PublisherSubject {
				return c.Subject(tb).CRUD
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.MetaAccessor{T: c.T, V: c.V,
			Subject: func(tb testing.TB) contracts.MetaAccessorSubject {
				subject := c.Subject(tb)
				return contracts.MetaAccessorSubject{
					MetaAccessor: subject.MetaAccessor,
					CRD:          subject.CRUD,
					Publisher:    subject.CRUD,
				}
			},
			FixtureFactory: c.FixtureFactory,
		},
		contracts.OnePhaseCommitProtocol{T: c.T,
			Subject: func(tb testing.TB) (frameless.OnePhaseCommitProtocol, contracts.CRD) {
				subject := c.Subject(tb)
				return subject.OnePhaseCommitProtocol, subject.CRUD
			},
			FixtureFactory: c.FixtureFactory,
		},
	)
}
