package cache_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/cache"
	"github.com/adamluzsi/frameless/cache/contracts"
	fc "github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/assert"
)

type TestEntity struct {
	ID    string `ext:"id"`
	Value string
}

func makeTestEntity(tb testing.TB) TestEntity {
	t := tb.(*testcase.T)
	return TestEntity{Value: t.Random.String()}
}

func makeCtx(tb testing.TB) context.Context {
	return context.Background()
}

func TestManager_creator(t *testing.T) {
	testcase.RunSuite(t, fc.Creator[TestEntity, string]{
		Subject: func(tb testing.TB) fc.CreatorSubject[TestEntity, string] {
			return NewManager(tb).Cache
		},
		MakeEnt: makeTestEntity,
		MakeCtx: makeCtx,
	})
}

func TestManager(t *testing.T) {
	testcase.RunSuite(t,
		contracts.Manager[TestEntity, string]{
			Subject: func(tb testing.TB) contracts.ManagerSubject[TestEntity, string] {
				return NewManager(tb)
			},
			MakeCtx: makeCtx,
			MakeEnt: makeTestEntity,
		},
	)
}

func NewManager(tb testing.TB) contracts.ManagerSubject[TestEntity, string] {
	eventLog := memory.NewEventLog()
	eventLog.Options.DisableAsyncSubscriptionHandling = true
	cacheHitStorage := memory.NewEventLogStorage[cache.Hit[string], string](eventLog)
	cacheEntityStorage := memory.NewEventLogStorageWithNamespace[TestEntity, string](eventLog, `TestEntity#CacheStorage`)
	sourceEntityStorage := memory.NewEventLogStorageWithNamespace[TestEntity, string](eventLog, `TestEntity#SourceStorage`)

	storage := TestCacheStorage{
		Hits:                   cacheHitStorage,
		Entities:               cacheEntityStorage,
		OnePhaseCommitProtocol: eventLog,
	}
	manager, err := cache.NewManager[TestEntity, string](storage, sourceEntityStorage)
	assert.Must(tb).Nil(err)
	tb.Cleanup(func() { _ = manager.Close() })
	return contracts.ManagerSubject[TestEntity, string]{
		Cache:         manager,
		Source:        sourceEntityStorage,
		CommitManager: eventLog,
	}
}

type TestCacheStorage struct {
	Hits     cache.HitStorage[string]
	Entities cache.EntityStorage[TestEntity, string]
	frameless.OnePhaseCommitProtocol
}

func (s TestCacheStorage) CacheEntity(ctx context.Context) cache.EntityStorage[TestEntity, string] {
	return s.Entities
}

func (s TestCacheStorage) CacheHit(ctx context.Context) cache.HitStorage[string] {
	return s.Hits
}
