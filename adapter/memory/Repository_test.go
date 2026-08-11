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

// TestRepository_concurrentUse_namespaceLazyInitIsRaceFree proves that a
// Repository created without an explicit Namespace can be used concurrently
// without a data race on the lazy initialisation of that Namespace field.
//
// Every Repository method resolves its namespace on entry (getNamespaceFor),
// and for a Repository built via NewRepository the Namespace starts empty:
// the first call derives it from the entity type and writes it back to the
// Namespace field. If that one-time write is left unguarded, concurrent calls
// race on it. This surfaces whenever a single Repository is shared across
// goroutines (for example, a workflow Scheduler being scheduled concurrently).
//
// Read-only operations are enough to trigger the race because they too resolve
// the namespace on entry; using only reads keeps the test focused on the
// Namespace field and free of any other shared mutable state. Run with -race
// to catch a regression.
func TestRepository_concurrentUse_namespaceLazyInitIsRaceFree(t *testing.T) {
	m := memory.NewMemory()
	// NewRepository leaves Namespace empty on purpose, so the lazy-init code
	// path (the one that must be race-free) is the code path under test.
	repo := memory.NewRepository[TestEntity, string](m)
	ctx := context.Background()

	assert.Empty(t, repo.Namespace,
		assert.Message("precondition: the repository must start without a namespace so the lazy-init path is exercised"))

	testcase.Race(
		func() { _, _, _ = repo.FindByID(ctx, "1") },
		func() { _, _, _ = repo.FindByID(ctx, "2") },
		func() { _, _, _ = repo.FindByID(ctx, "3") },
		func() { _, _, _ = repo.FindByID(ctx, "4") },
	)

	// The lazy init must have resolved the namespace exactly once, to a single
	// stable value that every caller shares.
	assert.NotEmpty(t, repo.Namespace)
}

// TestRepository_concurrentUse_lazyInitStaysConsistent is the functional
// counterpart of the race test above: it proves that the once-only namespace
// initialisation yields a single, consistent namespace even when the very
// first operations on a fresh Repository run concurrently.
//
// Each goroutine creates its own distinct entity (so the only state shared
// between them is the Repository's lazily-initialised Namespace). Afterwards
// every entity must be retrievable, which can only hold if all the concurrent
// callers resolved and wrote to the same namespace. Run with -race.
func TestRepository_concurrentUse_lazyInitStaysConsistent(t *testing.T) {
	m := memory.NewMemory()
	repo := memory.NewRepository[TestEntity, string](m)
	ctx := context.Background()

	ents := []*TestEntity{
		{ID: "1", Data: "one"},
		{ID: "2", Data: "two"},
		{ID: "3", Data: "three"},
		{ID: "4", Data: "four"},
	}

	var creates []func()
	for _, ent := range ents {
		creates = append(creates, func() {
			assert.NoError(t, repo.Create(ctx, ent))
		})
	}

	testcase.Race(creates...)

	// If the concurrent lazy init had produced diverging namespaces, some of
	// these entities would be stored under a different namespace and thus be
	// unreachable here.
	for _, ent := range ents {
		got, found, err := repo.FindByID(ctx, ent.ID)
		assert.NoError(t, err)
		assert.True(t, found, assert.MessageF("expected entity %q to be found after concurrent Create", ent.ID))
		assert.Equal(t, *ent, got)
	}
}
