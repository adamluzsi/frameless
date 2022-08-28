package memory_test

import (
	"context"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/meta"
	. "github.com/adamluzsi/frameless/spechelper/frcasserts"
	"testing"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
)

var (
	_ meta.MetaAccessor               = &memory.Memory{}
	_ comproto.OnePhaseCommitProtocol = &memory.Memory{}
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
