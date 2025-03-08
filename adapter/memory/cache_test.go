package memory_test

import (
	"testing"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/cache"
	"go.llib.dev/frameless/pkg/cache/cachecontracts"
	"go.llib.dev/frameless/spechelper/testent"
)

var _ cache.Interface[testent.Foo, testent.FooID] = &cache.Cache[testent.Foo, testent.FooID]{}

func TestCacheRepository(t *testing.T) {
	t.Run("var cr memory.CacheRepository", func(t *testing.T) {
		cacheRepository := &memory.CacheRepository[testent.Foo, testent.FooID]{}
		cachecontracts.Cache(cacheRepository).Test(t)
	})
	t.Run("memory.NewCacheRepository", func(t *testing.T) {
		cacheRepository := memory.NewCacheRepository[testent.Foo, testent.FooID](memory.NewMemory())
		cachecontracts.Cache(cacheRepository).Test(t)
	})
}
