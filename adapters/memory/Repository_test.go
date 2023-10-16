package memory_test

import (
	"context"
	"go.llib.dev/frameless/spechelper/resource"
	"testing"

	. "go.llib.dev/frameless/ports/crud/crudtest"

	"go.llib.dev/frameless/adapters/memory"
	"go.llib.dev/frameless/ports/comproto"
	"go.llib.dev/frameless/ports/meta"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

var (
	_ meta.MetaAccessor               = &memory.Memory{}
	_ comproto.OnePhaseCommitProtocol = &memory.Memory{}
)

func TestRepository(t *testing.T) {
	testcase.RunSuite(t, resource.Contract[TestEntity, string](func(tb testing.TB) resource.ContractSubject[TestEntity, string] {
		m := memory.NewMemory()
		s := memory.NewRepository[TestEntity, string](m)
		return resource.ContractSubject[TestEntity, string]{
			Resource:      s,
			MetaAccessor:  m,
			CommitManager: m,
			MakeContext:   func() context.Context { return makeContext(tb) },
			MakeEntity:    func() TestEntity { return makeTestEntity(tb) },
		}
	}))
}

func TestRepository_multipleRepositoryForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	m := memory.NewMemory()
	s1 := memory.NewRepositoryWithNamespace[TestEntity, string](m, "TestEntity#A")
	s2 := memory.NewRepositoryWithNamespace[TestEntity, string](m, "TestEntity#B")
	ent := random.New(random.CryptoSeed{}).Make(TestEntity{}).(TestEntity)
	ent.ID = ""
	Create[TestEntity, string](t, s1, ctx, &ent)
	IsAbsent[TestEntity, string](t, s2, ctx, HasID[TestEntity, string](t, ent))
}
