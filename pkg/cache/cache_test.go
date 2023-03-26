package cache_test

import (
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/pkg/cache"
	"github.com/adamluzsi/frameless/pkg/cache/cachecontracts"
	"github.com/adamluzsi/frameless/spechelper/testent"
	"github.com/adamluzsi/testcase"
	"testing"
)

var _ cache.Interface[testent.Foo, testent.FooID] = &cache.Cache[testent.Foo, testent.FooID]{}

func TestCache(t *testing.T) {
	testcase.RunSuite(t, cachecontracts.Cache[testent.Foo, testent.FooID](func(tb testing.TB) cachecontracts.CacheSubject[testent.Foo, testent.FooID] {
		m := memory.NewMemory()
		source := memory.NewRepository[testent.Foo, testent.FooID](m)
		cacheRepository := memory.NewCacheRepository[testent.Foo, testent.FooID](m)
		return cachecontracts.CacheSubject[testent.Foo, testent.FooID]{
			Cache:        cache.New[testent.Foo, testent.FooID](source, cacheRepository),
			Source:       source,
			Repository:   cacheRepository,
			MakeContext:  testent.MakeContextFunc(tb),
			MakeEntity:   testent.MakeFooFunc(tb),
			ChangeEntity: nil,
		}
	}))
}
