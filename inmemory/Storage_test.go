package inmemory_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/contracts"
	"github.com/adamluzsi/frameless/fixtures"
	"github.com/adamluzsi/frameless/inmemory"
	"github.com/adamluzsi/testcase"
)

var (
	_ frameless.MetaAccessor           = &inmemory.Memory{}
	_ frameless.OnePhaseCommitProtocol = &inmemory.Memory{}
)

func TestStorage(t *testing.T) {
	testcase.RunContract(t, GetContracts(TestEntity{}, func(tb testing.TB) ContractSubject {
		m := inmemory.NewMemory()
		s := inmemory.NewStorage(TestEntity{}, m)
		return ContractSubject{
			Creator:                s,
			Finder:                 s,
			Updater:                s,
			Deleter:                s,
			EntityStorage:          s,
			OnePhaseCommitProtocol: m,
			MetaAccessor:           m,
		}
	})...)
}

func TestStorage_multipleStorageForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	memory := inmemory.NewMemory()
	s1 := inmemory.NewStorageWithNamespace(TestEntity{}, memory, "TestEntity#A")
	s2 := inmemory.NewStorageWithNamespace(TestEntity{}, memory, "TestEntity#B")
	ent := fixtures.NewFactory(t).Create(TestEntity{}).(TestEntity)
	contracts.CreateEntity(t, s1, ctx, &ent)
	contracts.IsAbsent(t, TestEntity{}, s2, ctx, contracts.HasID(t, ent))
}
