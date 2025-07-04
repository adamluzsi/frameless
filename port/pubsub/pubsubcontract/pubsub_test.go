package pubsubcontract_test

import (
	"sort"
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/pubsub"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/testcase"

	. "go.llib.dev/frameless/testing/testent"
)

func TestFIFO(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontract.FIFO[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestLIFO(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		LIFO: true,
	}

	testcase.RunSuite(t, pubsubcontract.LIFO[TestEntity](q, q, pubsubConfig))
}

func TestBuffered(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{}

	pubsubcontract.Buffered[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestVolatile(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Volatile: true,
	}

	testcase.RunSuite(t, pubsubcontract.Volatile[TestEntity](q, q, pubsubConfig))
}

func TestBlocking(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		// SupportPublishContextCancellation: true,// TODO: fixme in memory queue

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		Blocking: true,
	}

	pubsubcontract.Blocking[TestEntity](q, q, pubsubConfig).Test(t)
}

func TestOrdering(t *testing.T) {
	pubsubConfig := pubsubcontract.Config[TestEntity]{
		SupportPublishContextCancellation: true,

		MakeData: func(tb testing.TB) TestEntity {
			v := makeTestEntity(tb)
			v.Data = testcase.ToT(&tb).Random.UUID()
			return v
		},
	}

	q := &memory.Queue[TestEntity]{
		SortLessFunc: func(i TestEntity, j TestEntity) bool {
			return i.Data < j.Data
		},
	}

	sorting := func(entities []TestEntity) {
		sort.Slice(entities, func(i, j int) bool {
			return entities[i].Data < entities[j].Data
		})
	}

	pubsubcontract.Ordering[TestEntity](q, q, sorting, pubsubConfig).Test(t)
}

var _ pubsub.Publisher[Foo] = &memory.FanOutExchange[Foo]{}

func TestFanOutExchange(t *testing.T) {
	exchange := &memory.FanOutExchange[Foo]{}

	var MakeQueue = func(tb testing.TB) pubsub.Subscriber[Foo] {
		return exchange.MakeQueue()
	}

	testcase.RunSuite(t,
		pubsubcontract.FanOut[Foo](exchange, MakeQueue),
		//pubsubcontracts.OnePhaseCommitProtocol
	)
}

// Entity is an example entity that can be used for testing
type TestEntity struct {
	ID   string `ext:"id"`
	Data string
	List []string
}

func makeTestEntity(tb testing.TB) TestEntity {
	t := tb.(*testcase.T)
	var list []string
	n := t.Random.IntBetween(1, 3)
	for i := 0; i < n; i++ {
		list = append(list, t.Random.String())
	}
	return TestEntity{
		Data: t.Random.String(),
		List: list,
	}
}
