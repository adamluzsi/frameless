package memory_test

import (
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"
	"github.com/adamluzsi/testcase"
	"testing"
)

func TestPubSub(t *testing.T) {
	testcase.RunSuite(t,
		pubsubcontracts.FIFO[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				return &memory.PubSub[TestEntity]{Memory: memory.NewMemory()}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		//pubsubcontracts.LIFO[TestEntity]{
		//	MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
		//		return &memory.PubSub[TestEntity]{Memory: memory.NewMemory(),LIFO: true}
		//	},
		//	MakeContext: makeContext,
		//	MakeV:       makeTestEntity,
		//},
		pubsubcontracts.Buffered[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				return &memory.PubSub[TestEntity]{Memory: memory.NewMemory()}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		pubsubcontracts.Volatile[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				return &memory.PubSub[TestEntity]{Memory: memory.NewMemory(), Volatile: true}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		pubsubcontracts.Blocking[TestEntity]{
			MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
				return &memory.PubSub[TestEntity]{Memory: memory.NewMemory(), Blocking: true}
			},
			MakeContext: makeContext,
			MakeV:       makeTestEntity,
		},
		//pubsubcontracts.Broadcast[TestEntity]{
		//	MakeSubject: func(tb testing.TB) pubsubcontracts.PubSub[TestEntity] {
		//		return &memory.PubSub[TestEntity]{Broadcast: true}
		//	},
		//	MakeContext: makeContext,
		//	MakeV:       makeTestEntity,
		//},
	)
}
