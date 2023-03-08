package memory_test

import (
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestQueue(t *testing.T) {
	testcase.RunSuite(t,
		pubsubcontracts.FIFO[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory()}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		pubsubcontracts.LIFO[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory(), LIFO: true}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		pubsubcontracts.Buffered[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory()}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		pubsubcontracts.Volatile[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory(), Volatile: true}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		pubsubcontracts.Blocking[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				q := &memory.Queue[TestEntity]{Memory: memory.NewMemory(), Blocking: true}
				return pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
	)
}
