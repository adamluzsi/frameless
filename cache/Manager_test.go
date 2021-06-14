package cache_test

import (
	"context"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/cache"
	"github.com/adamluzsi/frameless/cache/contracts"
	fc "github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/testcase"
	"github.com/stretchr/testify/require"
	"testing"
)

type TestEntity struct {
	ID    string `ext:"id"`
	Value string
}

func TestManager_creator(t *testing.T) {
	testcase.RunContract(t, fc.Creator{
		T: TestEntity{},
		Subject: func(tb testing.TB) fc.CRD {
			manager, _, _ := NewManager(tb)
			return manager
		},
		FixtureFactory: fixtures.FixtureFactory{},
	})
}

func TestManager(t *testing.T) {
	testcase.RunContract(t,
		contracts.Manager{
			T: TestEntity{},
			Subject: func(tb testing.TB) (contracts.Cache, cache.Source, frameless.OnePhaseCommitProtocol) {
				return NewManager(tb)
			},
			FixtureFactory: fixtures.FixtureFactory{},
		},
	)
}

func NewManager(tb testing.TB) (*cache.Manager, cache.Source, frameless.OnePhaseCommitProtocol) {
	eventLog := inmemory.NewEventLog()
	eventLog.Options.DisableAsyncSubscriptionHandling = true
	source := inmemory.NewEventLogStorage(TestEntity{}, eventLog)
	storage := TestCacheStorage{
		Hits:                   inmemory.NewEventLogStorage(cache.Hit{}, eventLog),
		Entities:               inmemory.NewEventLogStorage(TestEntity{}, eventLog),
		OnePhaseCommitProtocol: eventLog,
	}
	manager, err := cache.NewManager(TestEntity{}, storage, source)
	require.Nil(tb, err)
	tb.Cleanup(func() { _ = manager.Close() })
	return manager, source, eventLog
}

type TestCacheStorage struct {
	Hits     cache.HitStorage
	Entities cache.EntityStorage
	frameless.OnePhaseCommitProtocol
}

func (s TestCacheStorage) CacheEntity(ctx context.Context) cache.EntityStorage { return s.Entities }
func (s TestCacheStorage) CacheHit(ctx context.Context) cache.HitStorage       { return s.Hits }
