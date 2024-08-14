package memory_test

import (
	"sort"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontracts"
	"go.llib.dev/testcase"

	. "go.llib.dev/frameless/spechelper/testent"
)

var _ interface {
	pubsub.Publisher[Foo]
	pubsub.Subscriber[Foo]
} = &memory.Queue[Foo]{}

func TestFIFO(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Memory: memory.NewMemory(),
	}

	pubsubcontracts.FIFO[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestLIFO(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Memory: memory.NewMemory(),

		LIFO: true,
	}

	testcase.RunSuite(t, pubsubcontracts.LIFO[TestEntity](q, q, pubsubConfig))
}

func TestBuffered(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Memory: memory.NewMemory(),
	}

	pubsubcontracts.Buffered[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestVolatile(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Memory: memory.NewMemory(),

		Volatile: true,
	}

	testcase.RunSuite(t, pubsubcontracts.Volatile[TestEntity](q, q, pubsubConfig))
}

func TestBlocking(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		// SupportPublishContextCancellation: true,// TODO: fixme in memory queue

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Memory: memory.NewMemory(),

		Blocking: true,
	}

	pubsubcontracts.Blocking[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestOrdering(t *testing.T) {
	pubsubConfig := pubsubcontracts.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Memory: memory.NewMemory(),

		SortLessFunc: func(i TestEntity, j TestEntity) bool {
			return i.Data < j.Data
		},
	}

	sorting := func(entities []TestEntity) {
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].Data < entities[j].Data
		})
	}

	pubsubcontracts.Ordering[TestEntity](q, q, sorting, pubsubConfig).Test(t)
}

var _ pubsub.Publisher[Foo] = &memory.FanOutExchange[Foo]{}

func TestFanOutExchange(t *testing.T) {
	mm := memory.NewMemory()
	exchange := &memory.FanOutExchange[Foo]{Memory: mm}

	var MakeQueue = func(tb testing.TB) pubsub.Subscriber[Foo] {
		return exchange.MakeQueue()
	}

	testcase.RunSuite(t,
		pubsubcontracts.FanOut[Foo](exchange, MakeQueue),
		//pubsubcontracts.OnePhaseCommitProtocol
	)
}

var _ pubsub.Publisher[Foo] = &memory.FanOutExchange[Foo]{}
