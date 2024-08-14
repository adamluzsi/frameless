package memory_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/spechelper/resource"

	"go.llib.dev/frameless/port/crud/crudcontracts"
	. "go.llib.dev/frameless/port/crud/crudtest"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/meta"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/random"
)

var (
	_ meta.MetaAccessor               = &memory.Memory{}
	_ comproto.OnePhaseCommitProtocol = &memory.Memory{}
)

func TestRepository(t *testing.T) {
	m := memory.NewMemory()
	repo := memory.NewRepository[TestEntity, string](m)
	testcase.RunSuite(t, resource.Contract[TestEntity, string](repo, resource.Config[TestEntity, string]{
		MetaAccessor:  m,
		CommitManager: m,
		CRUD: crudcontracts.Config[TestEntity, string]{
			MakeEntity: makeTestEntity,
		},
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
