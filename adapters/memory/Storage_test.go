package memory_test

import (
	"context"
	"testing"

	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/adapters/memory"
	. "github.com/adamluzsi/frameless/contracts/asserts"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
)

var (
	_ frameless.MetaAccessor           = &memory.Memory{}
	_ frameless.OnePhaseCommitProtocol = &memory.Memory{}
)

func TestStorage(t *testing.T) {
	testcase.RunSuite(t, GetContracts[TestEntity, string](func(tb testing.TB) ContractSubject[TestEntity, string] {
		m := memory.NewMemory()
		s := memory.NewStorage[TestEntity, string](m)
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
	m := memory.NewMemory()
	s1 := memory.NewStorageWithNamespace[TestEntity, string](m, "TestEntity#A")
	s2 := memory.NewStorageWithNamespace[TestEntity, string](m, "TestEntity#B")
	ent := random.New(random.CryptoSeed{}).Make(TestEntity{}).(TestEntity)
	ent.ID = ""
	Create[TestEntity, string](t, s1, ctx, &ent)
	IsAbsent[TestEntity, string](t, s2, ctx, HasID[TestEntity, string](t, &ent))
}
