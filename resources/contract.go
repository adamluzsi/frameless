package resources

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/testcase"
)

type Contract[Ent, ID, V any] struct {
	Subject func(testing.TB) ContractSubject[Ent, ID]
	MakeCtx func(testing.TB) context.Context
	MakeEnt func(testing.TB) Ent
	MakeV   func(testing.TB) V
}

type ContractSubject[Ent, ID any] struct {
	Resource interface {
		frameless.Creator[Ent]
		frameless.Finder[Ent, ID]
		frameless.Updater[Ent]
		frameless.Deleter[ID]
		frameless.CreatorPublisher[Ent]
		frameless.UpdaterPublisher[Ent]
		frameless.DeleterPublisher[ID]
	}
	MetaAccessor  frameless.MetaAccessor
	CommitManager frameless.OnePhaseCommitProtocol
}

type Subscriber[Ent, ID any] interface {
	frameless.CreatorSubscriber[Ent]
	frameless.UpdaterSubscriber[Ent]
	frameless.DeleterSubscriber[ID]
}

func (c Contract[Ent, ID, V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Contract[Ent, ID, V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Contract[Ent, ID, V]) Spec(s *testcase.Spec) {
	testcase.RunContract(s,
		contracts.Creator[Ent, ID]{
			Subject: func(tb testing.TB) contracts.CreatorSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		contracts.Finder[Ent, ID]{
			Subject: func(tb testing.TB) contracts.FinderSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		contracts.Deleter[Ent, ID]{
			Subject: func(tb testing.TB) contracts.DeleterSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		contracts.Updater[Ent, ID]{
			Subject: func(tb testing.TB) contracts.UpdaterSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		contracts.Publisher[Ent, ID]{
			Subject: func(tb testing.TB) contracts.PublisherSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		contracts.MetaAccessorBasic[bool]{
			Subject: func(tb testing.TB) frameless.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) bool {
				t := tb.(*testcase.T)
				return t.Random.Bool()
			},
		},
		contracts.MetaAccessorBasic[string]{
			Subject: func(tb testing.TB) frameless.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) string {
				t := tb.(*testcase.T)
				return t.Random.String()
			},
		},
		contracts.MetaAccessorBasic[int]{
			Subject: func(tb testing.TB) frameless.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) int {
				t := tb.(*testcase.T)
				return t.Random.Int()
			},
		},
		contracts.MetaAccessor[Ent, ID, V]{
			Subject: func(tb testing.TB) contracts.MetaAccessorSubject[Ent, ID, V] {
				subject := c.Subject(tb)
				return contracts.MetaAccessorSubject[Ent, ID, V]{
					MetaAccessor: subject.MetaAccessor,
					Resource:     subject.Resource,
					Publisher:    subject.Resource,
				}
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
			MakeV:   c.MakeV,
		},
		contracts.OnePhaseCommitProtocol[Ent, ID]{
			Subject: func(tb testing.TB) contracts.OnePhaseCommitProtocolSubject[Ent, ID] {
				subject := c.Subject(tb)
				return contracts.OnePhaseCommitProtocolSubject[Ent, ID]{
					Resource:      subject.Resource,
					CommitManager: subject.CommitManager,
				}
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
	)
}
