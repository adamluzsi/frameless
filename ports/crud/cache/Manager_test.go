package cache_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless/ports/comproto"
	cachecontracts "github.com/adamluzsi/frameless/ports/crud/cache/contracts"
	fc "github.com/adamluzsi/frameless/ports/crud/contracts"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/crud/cache"
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
		cachecontracts.Manager[TestEntity, string]{
			Subject: func(tb testing.TB) cachecontracts.ManagerSubject[TestEntity, string] {
				return NewManager(tb)
			},
			MakeCtx: makeCtx,
			MakeEnt: makeTestEntity,
		},
	)
}

func NewManager(tb testing.TB) cachecontracts.ManagerSubject[TestEntity, string] {
	eventLog := memory.NewEventLog()
	eventLog.Options.DisableAsyncSubscriptionHandling = true
	cacheHitRepository := memory.NewEventLogRepository[cache.Hit[string], string](eventLog)
	cacheEntityRepository := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, `TestEntity#CacheRepository`)
	sourceEntityRepository := memory.NewEventLogRepositoryWithNamespace[TestEntity, string](eventLog, `TestEntity#SourceRepository`)

	testCacheRepository := TestCacheRepository{
		Hits:                   cacheHitRepository,
		Entities:               cacheEntityRepository,
		OnePhaseCommitProtocol: eventLog,
	}
	manager, err := cache.NewManager[TestEntity, string](testCacheRepository, sourceEntityRepository)
	assert.Must(tb).Nil(err)
	tb.Cleanup(func() { _ = manager.Close() })
	return cachecontracts.ManagerSubject[TestEntity, string]{
		Cache:         manager,
		Source:        sourceEntityRepository,
		CommitManager: eventLog,
	}
}

type TestCacheRepository struct {
	Hits     cache.HitRepository[string]
	Entities cache.EntityRepository[TestEntity, string]
	comproto.OnePhaseCommitProtocol
}

func (s TestCacheRepository) CacheEntity(ctx context.Context) cache.EntityRepository[TestEntity, string] {
	return s.Entities
}

func (s TestCacheRepository) CacheHit(ctx context.Context) cache.HitRepository[string] {
	return s.Hits
}
