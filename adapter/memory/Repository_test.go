package memory_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/internal/spechelper/resource"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/testing/testent"

	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/crud/crudtest"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/meta"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
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
		CRUD: crudcontract.Config[TestEntity, string]{
			MakeEntity: makeTestEntity,
		},
	}))
}

func TestRepository_implementsOnePhaseCommitProtocol(t *testing.T) {
	m := memory.NewMemory()
	repo := memory.NewRepository[TestEntity, string](m)
	testcase.RunSuite(t, resource.Contract[TestEntity, string](repo, resource.Config[TestEntity, string]{
		MetaAccessor:  m,
		CommitManager: repo,
		CRUD: crudcontract.Config[TestEntity, string]{
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
	crudtest.Create[TestEntity, string](t, s1, ctx, &ent)
	crudtest.IsAbsent[TestEntity, string](t, s2, ctx, ent.ID)
}

func TestRepository_Create_expectID(t *testing.T) {
	m := memory.NewMemory()
	r := memory.NewRepository[TestEntity, string](m)
	r.ExpectID = true

	ctx := context.Background()
	assert.Error(t, r.Create(ctx, &TestEntity{Data: "boom"}))
	assert.NoError(t, r.Create(ctx, &TestEntity{ID: "1", Data: "boom"}))
	assert.Error(t, r.Save(ctx, &TestEntity{Data: "boom"}))
	assert.NoError(t, r.Save(ctx, &TestEntity{ID: "1", Data: "boom"}))
}

func TestRepository_query(t *testing.T) {
	m := memory.NewMemory()
	r := memory.NewRepository[testent.Foo, testent.FooID](m)

	ctx := context.Background()
	ent1 := testent.MakeFoo(t)
	ent2 := testent.MakeFoo(t)
	ent3 := testent.MakeFoo(t)

	crudtest.Create[testent.Foo, testent.FooID](t, r, ctx, &ent1)
	crudtest.Create[testent.Foo, testent.FooID](t, r, ctx, &ent2)
	crudtest.Create[testent.Foo, testent.FooID](t, r, ctx, &ent3)

	got1, found, err := r.QueryOne(ctx, func(v testent.Foo) bool { return ent1.ID == v.ID })
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, ent1, got1)

	iter, err := r.QueryMany(ctx, func(v testent.Foo) bool {
		return v.ID == ent1.ID || v.ID == ent3.ID
	})
	assert.NoError(t, err)
	vs, err := iterkit.CollectE(iter)
	assert.NoError(t, err)
	assert.ContainsExactly(t, vs, []testent.Foo{ent1, ent3})
}

func TestRepository_Batcher_crudBatch(t *testing.T) {
	m := memory.NewMemory()
	r := memory.NewRepository[testent.Foo, testent.FooID](m)
	crudcontract.Batcher[testent.Foo, testent.FooID](r).Test(t)
}
