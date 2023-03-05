package cache_test

import (
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/frameless/pkg/cache/cachecontracts"
	sh "github.com/adamluzsi/frameless/spechelper"
	"github.com/adamluzsi/frameless/spechelper/testent"
	"github.com/adamluzsi/testcase"
	"testing"
)

var _ cache.Interface[testent.Foo, testent.FooID] = &cache.Cache[testent.Foo, testent.FooID]{}

func TestCache(t *testing.T) {
	testcase.RunSuite(t, cachecontracts.Cache[testent.Foo, testent.FooID]{
		MakeSubject: func(tb testing.TB) cachecontracts.CacheSubject[testent.Foo, testent.FooID] {
			m := memory.NewMemory()
			return cachecontracts.CacheSubject[testent.Foo, testent.FooID]{
				Source:     memory.NewRepository[testent.Foo, testent.FooID](m),
				Repository: memory.NewCacheRepository[testent.Foo, testent.FooID](m),
			}
		},
		MakeContext: sh.MakeContext,
		MakeEntity:  testent.MakeFoo,
	})
}

// EventStream: &memory.PubSub[cache.Event[string]]{
//					Memory:    m,
//					Namespace: "cache.Event[TestEntityID]",
//					Blocking:  testcase.ToT(&tb).Random.Bool(),
//				},
