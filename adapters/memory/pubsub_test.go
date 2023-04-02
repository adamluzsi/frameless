package memory_test

import (
	"context"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/pubsub"
	"github.com/adamluzsi/frameless/ports/pubsub/pubsubcontracts"
	"github.com/adamluzsi/testcase"
	"sort"
	"testing"

	. "github.com/adamluzsi/frameless/spechelper/testent"
)

var _ interface {
	pubsub.Publisher[Foo]
	pubsub.Subscriber[Foo]
} = &memory.Queue[Foo]{}

func TestQueue(t *testing.T) {
	testcase.RunSuite(t,
		pubsubcontracts.FIFO[TestEntity](func(tb testing.TB) pubsubcontracts.FIFOSubject[TestEntity] {
			q := &memory.Queue[TestEntity]{
				Memory: memory.NewMemory(),
			}
			return pubsubcontracts.FIFOSubject[TestEntity]{
				PubSub:      pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q},
				MakeContext: context.Background,
				MakeData:    makeTestEntityFunc(tb),
			}
		}),
		pubsubcontracts.LIFO[TestEntity](func(tb testing.TB) pubsubcontracts.LIFOSubject[TestEntity] {
			q := &memory.Queue[TestEntity]{
				Memory: memory.NewMemory(),
				LIFO:   true,
			}
			return pubsubcontracts.LIFOSubject[TestEntity]{
				PubSub:      pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q},
				MakeContext: context.Background,
				MakeData:    makeTestEntityFunc(tb),
			}
		}),
		pubsubcontracts.Buffered[TestEntity](func(tb testing.TB) pubsubcontracts.BufferedSubject[TestEntity] {
			q := &memory.Queue[TestEntity]{
				Memory: memory.NewMemory(),
			}
			return pubsubcontracts.BufferedSubject[TestEntity]{
				PubSub:      pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q},
				MakeContext: context.Background,
				MakeData:    makeTestEntityFunc(tb),
			}
		}),
		pubsubcontracts.Volatile[TestEntity](func(tb testing.TB) pubsubcontracts.VolatileSubject[TestEntity] {
			q := &memory.Queue[TestEntity]{
				Memory:   memory.NewMemory(),
				Volatile: true,
			}
			return pubsubcontracts.VolatileSubject[TestEntity]{
				PubSub:      pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q},
				MakeContext: context.Background,
				MakeData:    makeTestEntityFunc(tb),
			}
		}),
		pubsubcontracts.Blocking[TestEntity](func(tb testing.TB) pubsubcontracts.BlockingSubject[TestEntity] {
			q := &memory.Queue[TestEntity]{
				Memory:   memory.NewMemory(),
				Blocking: true,
			}
			return pubsubcontracts.BlockingSubject[TestEntity]{
				PubSub:      pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q},
				MakeContext: context.Background,
				MakeData:    makeTestEntityFunc(tb),
			}
		}),
		pubsubcontracts.Ordering[TestEntity](func(tb testing.TB) pubsubcontracts.OrderingSubject[TestEntity] {
			t := testcase.ToT(&tb)
			q := &memory.Queue[TestEntity]{
				Memory: memory.NewMemory(),
				SortLessFunc: func(i TestEntity, j TestEntity) bool {
					return i.Data < j.Data
				},
			}
			return pubsubcontracts.OrderingSubject[TestEntity]{
				PubSub: pubsubcontracts.PubSub[TestEntity]{Publisher: q, Subscriber: q},
				Sort: func(entities []TestEntity) {
					sort.Slice(entities, func(i, j int) bool {
						return entities[i].Data < entities[j].Data
					})
				},
				MakeContext: context.Background,
				MakeData: func() TestEntity {
					v := makeTestEntity(tb)
					v.Data = t.Random.UUID()
					return v
				},
			}
		}),
	)
}

var _ pubsub.Publisher[Foo] = &memory.FanOutExchange[Foo]{}

func TestFanOutExchange(t *testing.T) {
	testcase.RunSuite(t,
		pubsubcontracts.FanOut[Foo](func(tb testing.TB) pubsubcontracts.FanOutSubject[Foo] {
			mm := memory.NewMemory()
			exchange := &memory.FanOutExchange[Foo]{Memory: mm}
			return pubsubcontracts.FanOutSubject[Foo]{
				Exchange: exchange,
				MakeQueue: func() pubsub.Subscriber[Foo] {
					return exchange.MakeQueue()
				},
				MakeContext: context.Background,
				MakeData:    MakeFooFunc(tb),
			}
		}),
		//pubsubcontracts.OnePhaseCommitProtocol
	)
}
