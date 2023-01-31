package memory_test

import (
	"context"
	"testing"

	. "github.com/adamluzsi/frameless/ports/crud/crudtest"

	"github.com/adamluzsi/frameless/adapters/memory"
	"github.com/adamluzsi/frameless/ports/comproto"
	"github.com/adamluzsi/frameless/ports/meta"
	"github.com/adamluzsi/testcase"
	"github.com/adamluzsi/testcase/random"
)

var (
	_ meta.MetaAccessor               = &memory.Memory{}
	_ comproto.OnePhaseCommitProtocol = &memory.Memory{}
)

func TestRepository(t *testing.T) {
	testcase.RunSuite(t, GetContracts[TestEntity, string](func(tb testing.TB) ContractSubject[TestEntity, string] {
		m := memory.NewMemory()
		s := memory.NewRepository[TestEntity, string](m)
		return ContractSubject[TestEntity, string]{
			Resource:         s,
			EntityRepository: s,
			CommitManager:    m,
			MetaAccessor:     m,
		}
	}, makeContext, makeTestEntity)...)
}

func TestRepository_multipleRepositoryForSameEntityUnderDifferentNamespace(t *testing.T) {
	ctx := context.Background()
	m := memory.NewMemory()
	s1 := memory.NewRepositoryWithNamespace[TestEntity, string](m, "TestEntity#A")
	s2 := memory.NewRepositoryWithNamespace[TestEntity, string](m, "TestEntity#B")
	ent := random.New(random.CryptoSeed{}).Make(TestEntity{}).(TestEntity)
	ent.ID = ""
	Create[TestEntity, string](t, s1, ctx, &ent)
	IsAbsent[TestEntity, string](t, s2, ctx, HasID[TestEntity, string](t, &ent))
}
