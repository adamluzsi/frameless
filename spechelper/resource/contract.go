package resource

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/contracts"
	"github.com/adamluzsi/frameless/ports/meta"
	frmetacontracts "github.com/adamluzsi/frameless/ports/meta/contracts"
	"github.com/adamluzsi/frameless/ports/pubsub"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/contracts"

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
		crud.Creator[Ent]
		crud.Finder[Ent, ID]
		crud.Updater[Ent]
		crud.Deleter[ID]
		pubsub.CreatorPublisher[Ent]
		pubsub.UpdaterPublisher[Ent]
		pubsub.DeleterPublisher[ID]
	}
	MetaAccessor  meta.MetaAccessor
	CommitManager comproto.OnePhaseCommitProtocol
}

type Subscriber[Ent, ID any] interface {
	pubsub.CreatorSubscriber[Ent]
	pubsub.UpdaterSubscriber[Ent]
	pubsub.DeleterSubscriber[ID]
}

func (c Contract[Ent, ID, V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Contract[Ent, ID, V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Contract[Ent, ID, V]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.CreatorSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		crudcontracts.Finder[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.FinderSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		crudcontracts.Deleter[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.DeleterSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		crudcontracts.Updater[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.UpdaterSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		pubsubcontracts.Publisher[Ent, ID]{
			Subject: func(tb testing.TB) pubsubcontracts.PublisherSubject[Ent, ID] {
				return c.Subject(tb).Resource
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
		frmetacontracts.MetaAccessorBasic[bool]{
			Subject: func(tb testing.TB) meta.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) bool {
				t := tb.(*testcase.T)
				return t.Random.Bool()
			},
		},
		frmetacontracts.MetaAccessorBasic[string]{
			Subject: func(tb testing.TB) meta.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) string {
				t := tb.(*testcase.T)
				return t.Random.String()
			},
		},
		frmetacontracts.MetaAccessorBasic[int]{
			Subject: func(tb testing.TB) meta.MetaAccessor {
				return c.Subject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) int {
				t := tb.(*testcase.T)
				return t.Random.Int()
			},
		},
		frmetacontracts.MetaAccessor[Ent, ID, V]{
			Subject: func(tb testing.TB) frmetacontracts.MetaAccessorSubject[Ent, ID, V] {
				subject := c.Subject(tb)
				return frmetacontracts.MetaAccessorSubject[Ent, ID, V]{
					MetaAccessor: subject.MetaAccessor,
					Resource:     subject.Resource,
					Publisher:    subject.Resource,
				}
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
			MakeV:   c.MakeV,
		},
		crudcontracts.OnePhaseCommitProtocol[Ent, ID]{
			Subject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Ent, ID] {
				subject := c.Subject(tb)
				return crudcontracts.OnePhaseCommitProtocolSubject[Ent, ID]{
					Resource:      subject.Resource,
					CommitManager: subject.CommitManager,
				}
			},
			MakeCtx: c.MakeCtx,
			MakeEnt: c.MakeEnt,
		},
	)
}
