package resource

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/crud"
	crudcontracts "github.com/adamluzsi/frameless/ports/crud/crudcontracts"
	"github.com/adamluzsi/frameless/ports/meta"
	frmetacontracts "github.com/adamluzsi/frameless/ports/meta/metacontracts"
	"github.com/adamluzsi/frameless/ports/pubsub"
	pubsubcontracts "github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"

	"github.com/adamluzsi/testcase"
)

type Contract[Entity, ID, V any] struct {
	MakeSubject func(testing.TB) ContractSubject[Entity, ID]
	MakeContext func(testing.TB) context.Context
	MakeEntity  func(testing.TB) Entity
	MakeV       func(testing.TB) V
}

type ContractSubject[Entity, ID any] struct {
	Resource interface {
		crud.Creator[Entity]
		crud.Finder[Entity, ID]
		crud.Updater[Entity]
		crud.Deleter[ID]
		pubsub.CreatorPublisher[Entity]
		pubsub.UpdaterPublisher[Entity]
		pubsub.DeleterPublisher[ID]
	}
	MetaAccessor  meta.MetaAccessor
	CommitManager comproto.OnePhaseCommitProtocol
}

type Subscriber[Entity, ID any] interface {
	pubsub.CreatorSubscriber[Entity]
	pubsub.UpdaterSubscriber[Entity]
	pubsub.DeleterSubscriber[ID]
}

func (c Contract[Entity, ID, V]) Test(t *testing.T) {
	c.Spec(testcase.NewSpec(t))
}

func (c Contract[Entity, ID, V]) Benchmark(b *testing.B) {
	c.Spec(testcase.NewSpec(b))
}

func (c Contract[Entity, ID, V]) Spec(s *testcase.Spec) {
	testcase.RunSuite(s,
		crudcontracts.Creator[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.CreatorSubject[Entity, ID] {
				return c.MakeSubject(tb).Resource
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,

			SupportIDReuse: true,
		},
		crudcontracts.Finder[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.FinderSubject[Entity, ID] {
				return c.MakeSubject(tb).Resource
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
		crudcontracts.Deleter[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.DeleterSubject[Entity, ID] {
				return c.MakeSubject(tb).Resource
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
		crudcontracts.Updater[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.UpdaterSubject[Entity, ID] {
				return c.MakeSubject(tb).Resource
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
		pubsubcontracts.Publisher[Entity, ID]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PublisherSubject[Entity, ID] {
				return c.MakeSubject(tb).Resource
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
		frmetacontracts.MetaAccessorBasic[bool]{
			MakeSubject: func(tb testing.TB) meta.MetaAccessor {
				return c.MakeSubject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) bool {
				t := tb.(*testcase.T)
				return t.Random.Bool()
			},
		},
		frmetacontracts.MetaAccessorBasic[string]{
			MakeSubject: func(tb testing.TB) meta.MetaAccessor {
				return c.MakeSubject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) string {
				t := tb.(*testcase.T)
				return t.Random.String()
			},
		},
		frmetacontracts.MetaAccessorBasic[int]{
			MakeSubject: func(tb testing.TB) meta.MetaAccessor {
				return c.MakeSubject(tb).MetaAccessor
			},
			MakeV: func(tb testing.TB) int {
				t := tb.(*testcase.T)
				return t.Random.Int()
			},
		},
		frmetacontracts.MetaAccessor[Entity, ID, V]{
			MakeSubject: func(tb testing.TB) frmetacontracts.MetaAccessorSubject[Entity, ID, V] {
				subject := c.MakeSubject(tb)
				return frmetacontracts.MetaAccessorSubject[Entity, ID, V]{
					MetaAccessor: subject.MetaAccessor,
					Resource:     subject.Resource,
					Publisher:    subject.Resource,
				}
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
			MakeV:       c.MakeV,
		},
		crudcontracts.OnePhaseCommitProtocol[Entity, ID]{
			MakeSubject: func(tb testing.TB) crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID] {
				subject := c.MakeSubject(tb)
				return crudcontracts.OnePhaseCommitProtocolSubject[Entity, ID]{
					Resource:      subject.Resource,
					CommitManager: subject.CommitManager,
				}
			},
			MakeContext: c.MakeContext,
			MakeEntity:  c.MakeEntity,
		},
	)
}
