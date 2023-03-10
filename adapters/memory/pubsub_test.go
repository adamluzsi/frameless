package memory_test

import (
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"
	"github.com/adamluzsi/testcase"
	"testing"

	. "github.com/adamluzsi/frameless/spechelper/testent"
)

var _ interface {
	pubsub.Publisher[Foo]
	pubsub.Subscriber[Foo]
} = &memory.Queue[Foo]{}

func TestQueue(t *testing.T) {
	testcase.RunSuite(t,
		pubsubcontracts.FIFO[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory()}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeData:    makeTestEntity,
		},
		pubsubcontracts.LIFO[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory(), LIFO: true}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeData:    makeTestEntity,
		},
		pubsubcontracts.Buffered[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory()}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeData:    makeTestEntity,
		},
		pubsubcontracts.Volatile[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory(), Volatile: true}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeData:    makeTestEntity,
		},
		pubsubcontracts.Blocking[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory(), Blocking: true}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeData:    makeTestEntity,
		},
	)
}

var _ pubsub.Publisher[Foo] = &memory.FanOutExchange[Foo]{}

func TestFanOutExchange(t *testing.T) {
	testcase.RunSuite(t,
		pubsubcontracts.FanOut[Foo]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.FanOutSubject[Foo] {
				mm := memory.NewMemory()
				exchange := &memory.FanOutExchange[Foo]{Memory: mm}
				return pubsubcontracts.FanOutSubject[Foo]{
					Exchange: exchange,
					MakeQueue: func() pubsub.Subscriber[Foo] {
						return exchange.MakeQueue()
					},
				}
			},
			MakeContext: MakeContext,
			MakeData:    MakeFoo,
		},
		//pubsubcontracts.OnePhaseCommitProtocol
	)
}
