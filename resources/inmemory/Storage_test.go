package inmemory_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	. "github.com/adamluzsi/frameless/contracts/asserts"
	"github.com/adamluzsi/frameless/resources/inmemory"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
)

var (
	_ frameless.MetaAccessor           = &inmemory.Memory{}
	_ frameless.OnePhaseCommitProtocol = &inmemory.Memory{}
)

func TestStorage(t *testing.T) {
	testcase.RunContract(t, GetContracts[TestEntity, string](func(tb testing.TB) ContractSubject[TestEntity, string] {
		m := inmemory.NewMemory()
		s := inmemory.NewStorage[TestEntity, string](m)
		return ContractSubject[TestEntity, string]{
			Resource:      s,
			EntityStorage: s,
			CommitManager: m,
			MetaAccessor:  m,
		}
	}, makeContext, makeTestEntity)...)
}

func TestStorage_multipleStorageForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	memory := inmemory.NewMemory()
	s1 := inmemory.NewStorageWithNamespace[TestEntity, string](memory, "TestEntity#A")
	s2 := inmemory.NewStorageWithNamespace[TestEntity, string](memory, "TestEntity#B")
	ent := random.New(random.CryptoSeed{}).Make(TestEntity{}).(TestEntity)
	ent.ID = ""
	Create[TestEntity, string](t, s1, ctx, &ent)
	IsAbsent[TestEntity, string](t, s2, ctx, HasID[TestEntity, string](t, &ent))
}
