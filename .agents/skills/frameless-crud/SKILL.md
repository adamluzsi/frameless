---
name: frameless-crud
description: Design idiomatic repository (entity storage) role interfaces with go.llib.dev/frameless/port/crud, and make the domain own their behaviour through contracts built on port/crud/crudcontract. Use when a component needs to store, find, update or delete entities (repository pattern, DAO, entity store, REST resource client); when writing a `<pkg>contract` package so every adapter (SQL, in-memory fake, HTTP client, cache) inherits premade test coverage, including OnePhaseCommitProtocol transactions, ctx cancellation, races and eventual consistency; when implementing or testing an adapter against such a contract; when adding custom QueryOne/QueryMany methods; or when changing port/crud, crudcontract, crudtest or extid. Applies to any Go project that imports go.llib.dev/frameless.
compatibility: Go projects importing go.llib.dev/frameless/port/crud; contracts also use go.llib.dev/testcase.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: port/crud, port/crud/crudcontract
  scope: Global
---

# Frameless CRUD

`go.llib.dev/frameless/port/crud` is a shared vocabulary for entity storage (the repository pattern). `port/crud/crudcontract` gives every word in it an executable meaning. Use them together so the **consumer owns both halves of the dependency**:

- the **role interface** (method signatures) lives next to the code that uses it, composed from `crud` ports;
- the **contract** (expected behaviour) ships beside it in a `<pkg>contract` package, composed from `crudcontract` suites;
- every implementation (SQL adapter, in-memory fake, REST client, cache) runs that contract in its tests.

This inverts the *behavioural* dependency, not only the signatures. Domain code relies on one enforced behaviour for all its repositories, whatever technology backs them. Adapter authors get dozens of premade scenarios for free: not-found semantics, error contracts, ctx cancellation, races, eventual consistency, and transactions.

## Stop signs

If you are about to do any of these, follow this skill instead:

- Declaring the repository interface in the adapter package, or one wide `Repository` with every method "just in case".
- Signatures that drift from the ports, such as `FindByID(ctx, id) (*Entity, error)` with `nil` meaning "not found", `sql.ErrNoRows` or an HTTP 404 escaping as an error, `[]Entity` results instead of `iter.Seq2[Entity, error]`, a `Create` that returns the entity instead of filling `*ptr`, or renamed verbs (`Get`, `Store`, `Remove`).
- Mocking a repository (gomock, mockery, hand-written stubs with canned answers) in domain tests.
- Hand-writing per-adapter CRUD tests (create, find, not found, delete, ctx cancel, tx) that `crudcontract` already provides.
- Calling `Purge`, or `DeleteAll` as a "reset", from domain logic.
- Running a contract against a shared or production database. Contracts wipe subjects that implement `crud.Purger` or `crud.AllDeleter`.

## The model

| Piece | Lives in | Form |
| --- | --- | --- |
| Entity and typed ID | domain package | struct with an ID field tagged `ext:"id"`, plus `type XID string` (or another dedicated type) |
| Role interface | domain (consumer) package | interface embedding only the `crud` ports it calls, plus custom query methods |
| Contract | `<pkg>contract` package beside the domain | `func XRepository(subject pkg.XRepository, opts ...Option) contract.Contract` composing `crudcontract` suites |
| Implementations | adapter packages | each runs the contract in its `_test.go` |
| Fake for domain tests | `adapter/memory` | `memory.NewRepository[ENT, ID](memory.NewMemory())`, verified by the same contract |

## Design the role interface

| The consumer needs to… | Embed |
| --- | --- |
| store a new entity, letting storage assign the ID and managed fields | `crud.Creator[ENT]` |
| insert-or-replace an entity whose ID the domain owns (upsert) | `crud.Saver[ENT]` |
| load one by ID | `crud.ByIDFinder[ENT, ID]` |
| load several by ID | `crud.ByIDsFinder[ENT, ID]` |
| list everything | `crud.AllFinder[ENT]` (`crud.Finder[ENT, ID]` is ByID + All) |
| change an existing entity | `crud.Updater[ENT]` |
| remove one by ID | `crud.ByIDDeleter[ID]` |
| remove everything | `crud.AllDeleter` (`crud.Deleter[ID]` is ByID + All) |
| stream-insert many (imports) | `crud.Batcher[ENT, crud.Batch[ENT]]` |
| a domain-specific lookup | a method shaped like `crud.QueryOneMethodSignature` or `crud.QueryManyMethodSignature` (see [Custom queries](#custom-queries)) |
| atomic multi-call use cases | `comproto.OnePhaseCommitProtocol`, embedded or as a separate dependency (see [Transactions](#transactions-and-the-commit-manager)) |
| wipe everything between tests | `crud.Purger`: test tooling only, never domain logic |

```go
package mydomain

type MyDomainEntity struct {
	ID   MyDomainEntityID `ext:"id"`
	Name string
}

type MyDomainEntityID string

// MyDomainEntityRepository is exactly the storage this package needs.
type MyDomainEntityRepository interface {
	crud.Saver[MyDomainEntity]
	crud.ByIDFinder[MyDomainEntity, MyDomainEntityID]
	crud.ByIDDeleter[MyDomainEntityID]
}
```

Rules:

- Embed only what the consumer calls. Every extra port is a burden on every adapter. Adding one later is cheap.
- Use `Creator` when storage assigns IDs. Use `Saver` when the domain owns IDs (UUIDs, natural keys) or needs idempotent writes.
- Keep the port method names. The shared vocabulary is the point, and the contracts match on exact method sets. Copy-pasted signatures are equivalent to embedding, but embedding states the intent.
- One role interface per consumer need. Two consumers with different needs get two small interfaces, and one adapter can satisfy both.
- Entity types stay plain domain structs. No ORM tags, `sql.Null*` fields, or storage-only fields; mapping belongs to the adapter.

### Entities and IDs

- Give each entity a dedicated ID type and tag its identity field `ext:"id"`. That is the unambiguous convention. `extid` also falls back to a field named `ID`, a single field of the ID type, an embedded struct, or `extid.RegisterType`.
- Reference other entities by their typed ID (`OwnerID UserID`). See `port/crud/relationship`.
- If the identity field can't follow the convention, pass an accessor, `extid.Accessor[ENT, ID](func(ent *ENT) *ID { return &ent.Key })`, as `crudcontract.Config.IDA` and `memory.Repository.IDA`.
- For integer IDs, the contracts treat zero as a valid ID (index-style sequences). Prefer string or UUID types unless the store really uses sequences.
- Entities must round-trip unchanged, because contracts compare what `FindByID` returns using `assert.Equal`. In `MakeEntity`, normalise values the store would alter: UTC times rounded to the store's precision, and consistent nil versus empty slices.

### Use it from domain code

```go
type Renamer struct {
	Repository MyDomainEntityRepository
	TxManager  comproto.OnePhaseCommitProtocol
}

func (r Renamer) Rename(ctx context.Context, id MyDomainEntityID, name string) (rErr error) {
	tx, err := r.TxManager.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, r.TxManager, tx) // commit on nil error, else rollback

	ent, found, err := r.Repository.FindByID(tx, id)
	if err != nil {
		return err
	}
	if !found {
		return ErrMyDomainEntityNotFound // absence is a domain decision
	}
	ent.Name = name
	return r.Repository.Save(tx, &ent)
}
```

- Check `found` before using the entity. The zero value is never "the entity".
- Check `errors.Is(err, crud.ErrNotFound)` after `Update`/`DeleteByID`, and `errors.Is(err, crud.ErrAlreadyExists)` after `Create`.
- Consume iterators with `for ent, err := range repo.FindAll(ctx) { if err != nil { return err } … }`. Check `err` on every step. `break` early freely.
- Pass the transaction context (`tx`) to every repository call that must be atomic.

## Behaviour the ports promise

These are the semantics the contracts enforce. Design domain logic around them, and implement adapters to them.

| Method | Must |
| --- | --- |
| `Create(ctx, ptr)` | persist; write the assigned ID (if zero) and storage-managed fields back into `*ptr`; same ID again → `crud.ErrAlreadyExists` |
| `Save(ctx, ptr)` | create when absent, replace when present, never duplicate; requires a non-zero ID |
| `FindByID(ctx, id)` | `(ent, true, nil)` when found, `(zero, false, nil)` when not; errors only for real failures |
| `FindByIDs(ctx, ids...)` | exactly the requested entities; no IDs → empty; any missing ID → `crud.ErrNotFound` as the first item, so verify all IDs before yielding |
| `FindAll(ctx)`, QueryMany methods | `iter.Seq2[ENT, error]`; no match → empty iterator, not an error |
| `Update(ctx, ptr)` | replace stored values; unknown ID → `crud.ErrNotFound` |
| `DeleteByID(ctx, id)` | remove only that entity; unknown or already deleted → `crud.ErrNotFound` |
| `DeleteAll(ctx)` | remove every entity of that type |
| `Batch(ctx)` → `Add`, `Close` | buffered insert that doesn't assign IDs; `Close` is mandatory and idempotent; a cancelled ctx fails `Add`/`Close` |
| QueryOne methods | `(ent, found, err)` like `FindByID` |

Across all methods:

- A cancelled `ctx` fails the call with an error matching `errors.Is(err, context.Canceled)`. Iterators yield it.
- Methods are safe for concurrent use. The contracts race them, so run them with `-race`.
- An open iterator must not block other calls. Contracts interleave two iterations, and `FindByID` inside an iteration.
- Reads may be eventually consistent. Contracts poll with `t.Eventually` and `crudtest` helpers.
- With a commit manager: writes made in `tx := cm.BeginTx(ctx)` are visible in `tx` but not outside it until `CommitTx`. `RollbackTx` discards them. Nested `BeginTx` works. Using `tx` after it finished returns an error.
- Wrap errors with `%w` or `crud.ErrNotFound.F(...)` so `errors.Is` keeps working.

## Write the domain contract

```
mydomain/
  mydomain.go                           entity, ID, role interface, use cases
  mydomain_test.go                      use-case specs with the memory fake, plus the contract against the fake
  mydomaincontract/mydomaincontract.go  regular (non-test) package, so adapters can import it
pgrepo/
  mydomainentity.go                     the adapter
  mydomainentity_test.go                runs mydomaincontract against a real database
```

```go
package mydomaincontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/crud"
	"go.llib.dev/frameless/port/crud/crudcontract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"example.com/myapp/mydomain"
)

// MyDomainEntityRepository is the behaviour mydomain expects from every
// mydomain.MyDomainEntityRepository implementation.
func MyDomainEntityRepository(subject mydomain.MyDomainEntityRepository, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config](opts)

	if c.CommitManager == nil { // the subject may manage its own transactions
		c.CommitManager, _ = subject.(comproto.OnePhaseCommitProtocol)
	}

	crudConfig := crudcontract.Config[mydomain.MyDomainEntity, mydomain.MyDomainEntityID]{
		MakeContext:    c.MakeContext,
		MakeEntity:     c.MakeEntity,
		OnePhaseCommit: c.CommitManager, // nil skips the transaction scenarios
		// Save may change everything except the ID.
		ChangeEntity: func(tb testing.TB, ptr *mydomain.MyDomainEntity) {
			ptr.Name = testcase.ToT(&tb).Random.String()
		},
	}
	c.CRUD.Configure(&crudConfig) // adapter-specific tuning wins

	testcase.RunSuite(s,
		crudcontract.Saver(subject, crudConfig),
		crudcontract.ByIDFinder(subject, crudConfig),
		crudcontract.ByIDDeleter(subject, crudConfig),
	)
	if c.CommitManager != nil {
		testcase.RunSuite(s, crudcontract.OnePhaseCommitProtocol(subject, c.CommitManager, crudConfig))
	}

	// crudcontract.ByIDDeleter only checks deletion on subjects that also implement
	// crud.Creator, so this Saver-based role interface states the expectation itself.
	s.Describe("#DeleteByID", func(s *testcase.Spec) {
		var (
			ctx    = let.With[context.Context](s, c.MakeContext)
			entity = let.Var(s, func(t *testcase.T) mydomain.MyDomainEntity {
				ent := c.MakeEntity(t)
				crudConfig.Helper().Save(t, subject, c.MakeContext(t), &ent)
				return ent
			})
		)
		act := func(t *testcase.T) error {
			return subject.DeleteByID(ctx.Get(t), entity.Get(t).ID)
		}

		s.Then("the entity is no longer findable", func(t *testcase.T) {
			assert.NoError(t, act(t))
			crudConfig.Helper().IsAbsent(t, subject, c.MakeContext(t), entity.Get(t).ID)
		})

		s.When("the entity was already deleted", func(s *testcase.Spec) {
			s.Before(func(t *testcase.T) {
				assert.NoError(t, act(t))
			})

			s.Then("it reports crud.ErrNotFound", func(t *testcase.T) {
				assert.ErrorIs(t, act(t), crud.ErrNotFound)
			})
		})
	})

	return s.AsSuite("mydomain.MyDomainEntityRepository")
}

type Option interface {
	option.Option[Config]
}

// Config lets an adapter tune the contract. Every field is optional.
type Config struct {
	// MakeContext returns the context the subject is called with (tenant, auth, ...).
	//
	// Default: context.Background()
	MakeContext func(testing.TB) context.Context
	// MakeEntity returns a valid entity with a unique, non-zero ID.
	MakeEntity func(testing.TB) mydomain.MyDomainEntity
	// CommitManager owns the transactions the subject takes part in,
	// e.g. the database connection the adapter uses.
	//
	// Default: the subject, if it implements comproto.OnePhaseCommitProtocol.
	CommitManager comproto.OnePhaseCommitProtocol
	// CRUD is an escape hatch for adapter-specific crudcontract tuning.
	CRUD crudcontract.Config[mydomain.MyDomainEntity, mydomain.MyDomainEntityID]
}

func (c *Config) Init() {
	c.MakeContext = func(testing.TB) context.Context { return context.Background() }
	c.MakeEntity = func(tb testing.TB) mydomain.MyDomainEntity {
		t := testcase.ToT(&tb)
		return mydomain.MyDomainEntity{
			ID:   mydomain.MyDomainEntityID(t.Random.UUID()),
			Name: t.Random.String(),
		}
	}
}

func (c Config) Configure(t *Config) { *t = reflectkit.MergeStruct(*t, c) }
```

How it fits together:

- **One contract per embedded port.** Map each port to the `crudcontract` constructor with the same name (`crud.Saver` → `crudcontract.Saver`, `crud.Finder` → `crudcontract.Finder`, …). See `references/contract-catalogue.md`.
- **Options.** `option.ToConfig` runs `Init()` for defaults, then applies each option. `Configure` merges only non-zero fields (`reflectkit.MergeStruct`), so an adapter passes just what it needs, for example `mydomaincontract.Config{CommitManager: conn}`. Because zero values never override, an adapter can't switch off a `true` that the domain contract sets.
- **Type arguments** are inferred from the typed `crudConfig`. Without such a value, spell them out: `crudcontract.Saver[mydomain.MyDomainEntity, mydomain.MyDomainEntityID](subject)`. `ID` can't be inferred from the subject alone.
- **Domain requirements live in the contract, not in adapters.** Examples are `ChangeEntity` (which fields may change), `SupportIDReuse`/`SupportRecreate` (the domain re-creates deleted IDs), and a `MakeEntity` that satisfies domain invariants. The `CRUD` field is only an escape hatch, for example `ExampleEntity` for read-only adapters.
- **`MakeEntity` decides who owns IDs.** Populate a unique ID when the domain assigns IDs, and always with `Saver`. Leave the ID zero when storage assigns it through `Creator`. The contract then checks that `Create` writes one back. Every call must return a distinct entity.
- **Add domain-specific specs** with `s.Describe` for behaviour the generic suites don't express: invariants, ordering, uniqueness on other fields, or coverage gaps like the one above. Use the `crudConfig.Helper()` (`crudtest.Helper`) methods `Save`, `Create`, `Delete`, `IsPresent`, `IsAbsent`, and `HasEntity`. They wait for eventual consistency and register cleanup. Follow the `testcase-spec` skill for spec style.
- **Naming.** Use package `<pkg>contract`, a function named after the role interface, and `s.AsSuite("<pkg>.<RoleInterface>")`. A contract that needs no options can take only the subject, like `migrationcontract.StateRepository(subject)`.

### Skipped is not passed

The contracts look up optional capabilities on the subject's dynamic type with type assertions. They register or skip scenarios based on what they find, and skips are silent unless you run `go test -v`. Know your gaps:

- `ByIDDeleter`/`AllDeleter` check deletion only when the subject also implements `crud.Creator`. A strict `Saver`-based adapter gets only the ctx-cancel and transaction scenarios. That is why the example adds its own `#DeleteByID` spec.
- `Saver` verifies stored results only through `ByIDFinder`. Its "count doesn't grow" check needs `AllFinder`.
- Read-only role interfaces (no `Creator` or `Saver`) can't seed data. Provide `CRUD.ExampleEntity`, an entity that already exists in the resource, or the find scenarios skip.
- QueryOne's "empty resource" scenario needs a wipeable resource (`Purger` or `AllDeleter`).
- `OnePhaseCommitProtocol` exercises the subject only if it is `Creator + ByIDFinder + ByIDDeleter` or `Saver`. Otherwise it only tests the manager and logs a warning.
- A fake that embeds `*memory.Repository` exposes extra methods (`Create`, `FindAll`, `DeleteAll`, …), so it runs more scenarios than a strict adapter. Equal pass counts don't mean equal coverage.

Audit with `go test -v -run 'TestMyDomainEntityRepository' ./... | grep -- '--- SKIP'`.

## Transactions and the commit manager

| Situation | Wiring |
| --- | --- |
| The role interface embeds `comproto.OnePhaseCommitProtocol` | The subject is the manager, and the example's fallback picks it up. Transactions are then part of the role, so the tx scenarios always run, as in `migrationcontract.StateRepository` |
| Another component owns transactions (DB connection, `memory.Memory`) and repositories join through `ctx` | The adapter passes it as `Config.CommitManager`. If the domain depends on it, make it mandatory with a positional parameter, as `cachecontract.EntityRepository(subject, commitManager, ...)` does |
| No transactions | Leave it nil; the transaction scenarios are skipped |

Use both levels. `crudcontract.Config.OnePhaseCommit` adds commit, rollback, and finished-tx scenarios inside each port contract (Creator, Saver, Updater, ByIDDeleter, AllDeleter, Batcher). `crudcontract.OnePhaseCommitProtocol(subject, manager, cfg)` adds the cross-cutting ones: isolation, rollback of creates and deletes, nested `BeginTx`, and errors on a finished tx. It also runs `comprotocontract` against the manager.

## Custom queries

Domain lookups beyond ID get a method shaped like `crud.QueryOneMethodSignature` (`(ctx, args) (ENT, found bool, error)`) or `crud.QueryManyMethodSignature` (`(ctx, args) iter.Seq2[ENT, error]`). Name them after the domain question, and back them with `crudcontract.QueryOne` or `crudcontract.QueryMany`:

```go
// mydomain: add to the role interface
FindByName(ctx context.Context, name string) iter.Seq2[MyDomainEntity, error]

// mydomaincontract: add to the RunSuite list
crudcontract.QueryMany(subject, "FindByName", func(tb testing.TB) crudcontract.QueryManySubject[mydomain.MyDomainEntity] {
	t := testcase.ToT(&tb)
	name := t.Random.UUID() // unique per test, so leftovers from other tests never match
	return crudcontract.QueryManySubject[mydomain.MyDomainEntity]{
		Query: func(ctx context.Context) iter.Seq2[mydomain.MyDomainEntity, error] {
			return subject.FindByName(ctx, name)
		},
		IncludedEntity: func() mydomain.MyDomainEntity { // must match; called several times
			ent := c.MakeEntity(t)
			ent.Name = name
			return ent
		},
		ExcludedEntity: func() mydomain.MyDomainEntity { // must not match
			return c.MakeEntity(t)
		},
	}
}, crudConfig),
```

- The first argument is the *resource* the contract seeds through (`Creator`, `Saver`, or for QueryMany also `Batcher`) and wipes (`Purger`/`AllDeleter`). It is usually the subject.
- `crudcontract.QueryOne(resource, "FindByEmail", func(tb testing.TB) crudcontract.QueryOneSubject[ENT]{Query, ExpectedEntity, ExcludedEntity}, cfg)` works the same way. The contract stores `ExpectedEntity` unless it is already findable by its ID. Store it yourself (e.g. with `crudtest.Create`) only when the resource can't create it, or when the query needs its ID up front. To cover the not-found cases, the contract removes it through `ByIDDeleter` or `AllDeleter`.
- Matches must be unique per test (random keys), because other tests may leave entities behind.
- The in-memory fake implements custom queries with `memory.Repository`'s filter helpers:

  ```go
  type FakeMyDomainEntityRepository struct {
  	*memory.Repository[mydomain.MyDomainEntity, mydomain.MyDomainEntityID]
  }

  func (r FakeMyDomainEntityRepository) FindByName(ctx context.Context, name string) iter.Seq2[mydomain.MyDomainEntity, error] {
  	itr, err := r.QueryMany(ctx, func(ent mydomain.MyDomainEntity) bool { return ent.Name == name })
  	if err != nil {
  		return iterkit.Error[mydomain.MyDomainEntity](err)
  	}
  	return itr
  }
  ```

## Run the contract

In an adapter's tests:

```go
func TestMyDomainEntityRepository(t *testing.T) {
	conn, err := postgresql.Connect(os.Getenv("TEST_DATABASE_URL")) // a dedicated test database
	assert.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	repo := pgrepo.MyDomainEntityRepository{Connection: conn}

	mydomaincontract.MyDomainEntityRepository(repo, mydomaincontract.Config{
		CommitManager: conn, // the connection owns transactions; repo joins them through ctx
	}).Test(t)
}
```

Against the in-memory fake, which also proves the fake is trustworthy for domain tests:

```go
func TestMyDomainEntityRepository_memory(t *testing.T) {
	repo := memory.NewRepository[mydomain.MyDomainEntity, mydomain.MyDomainEntityID](memory.NewMemory())
	mydomaincontract.MyDomainEntityRepository(repo).Test(t)
}
```

- A `contract.Contract` offers `.Test(t)`, `.Spec(s)` to mount it inside an existing spec, and `.Benchmark(b)` to compare implementations. Run several with `testcase.RunSuite(t, a, b)`.
- Run with `-race`. Reproduce random-order failures with the printed `TESTCASE_SEED`.
- Use a dedicated database, schema, or namespace. Contracts call `Purge`/`DeleteAll` when the subject has them.
- `memory.Repository` implements every port and `comproto.OnePhaseCommitProtocol`. Several repositories can share one `memory.Memory` as their common commit manager. Set `MakeID` for ID kinds other than string or int, `IDA` for unconventional ID fields, and `ExpectID: true` to emulate stores that require caller-supplied IDs.
- In use-case specs, inject the fake (`memory.NewRepository[...]`) wherever the role interface is needed, and the `memory.Memory` wherever a commit manager is needed. Don't mock.

## Implement an adapter

Reach for a generic implementation before writing SQL by hand:

- `adapter/postgresql.Repository[ENT, ID]` with a `flsql.Mapping`, or `adapter/mariadb.Repository[ENT, ID]`. These, and `adapter/hashicorp/vault`, are separate Go modules: `go get go.llib.dev/frameless/adapter/postgresql`.
- `pkg/httpkit.RESTClient[ENT, ID]` for a remote REST resource (`httpkit.RESTHandlerFromCRUD` exposes a repository as one)
- `pkg/cache.Cache[ENT, ID]` for read/write-through caching in front of a source

To add custom queries, embed the generic implementation in a named type. Remember that embedding also exposes its other methods to the contracts (`DeleteAll` means the table gets wiped).

When writing one by hand, translate the storage's dialect into the port's:

```go
package pgrepo

var _ mydomain.MyDomainEntityRepository = MyDomainEntityRepository{}

type MyDomainEntityRepository struct {
	// Connection runs queries in the transaction carried by ctx, if there is one.
	Connection flsql.Connection // e.g. postgresql.Connection
}

func (r MyDomainEntityRepository) FindByID(ctx context.Context, id mydomain.MyDomainEntityID) (mydomain.MyDomainEntity, bool, error) {
	var ent mydomain.MyDomainEntity
	err := r.Connection.QueryRowContext(ctx, `SELECT id, name FROM my_domain_entities WHERE id = $1`, id).
		Scan(&ent.ID, &ent.Name)
	if errors.Is(err, pgx.ErrNoRows) { // the driver's sentinel; sql.ErrNoRows with database/sql
		return mydomain.MyDomainEntity{}, false, nil // absence is an answer, not an error
	}
	if err != nil {
		return mydomain.MyDomainEntity{}, false, err
	}
	return ent, true, nil
}

func (r MyDomainEntityRepository) DeleteByID(ctx context.Context, id mydomain.MyDomainEntityID) error {
	res, err := r.Connection.ExecContext(ctx, `DELETE FROM my_domain_entities WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return crud.ErrNotFound.F("my domain entity %v", id)
	}
	return nil
}
```

- Keep the compile-time assertion against the role interface.
- Run every query through the component that owns transactions. `flsql.Connection` (for example `postgresql.Connection`) is both the commit manager and a tx-aware `Queryable`, so calls made with `tx` join the transaction. A finished tx must make calls fail.
- Close rows or streams when the consumer stops iterating early (`yield` returns false), and yield errors through the iterator.
- Make the contract pass first, then add adapter-specific tests only for what the contract can't know: mapping edge cases, migrations, performance.

## `crudcontract.Config` reference

| Field | Purpose |
| --- | --- |
| `MakeContext func(testing.TB) context.Context` | Context for every call. Default: `context.Background()` |
| `MakeEntity func(testing.TB) ENT` | A valid, unique entity. Default: a random value with a zero ID, which only suits `Creator` with storage-assigned IDs and no invariants |
| `ChangeEntity func(testing.TB, *ENT)` | Mutates the fields `Update`/`Save` may change. Default: regenerates via `MakeEntity`, keeping the ID |
| `ExampleEntity func(testing.TB) ENT` | Returns an entity already present in the resource, which enables subjects without `Creator`/`Saver` |
| `IDA extid.Accessor[ENT, ID]` | ID accessor for entities outside the ID conventions |
| `OnePhaseCommit comproto.OnePhaseCommitProtocol` | Enables the transaction scenarios inside each port contract |
| `SupportIDReuse bool` | `Creator`/`Batcher` must accept the ID of a deleted entity |
| `SupportRecreate bool` | `Creator`/`Batcher` must accept re-creating a deleted entity with the same values |
| `NonUniqueID bool` | Storage doesn't enforce unique IDs, so the duplicate-create → `ErrAlreadyExists` check is skipped |
| `LazyNotFoundError bool` | Legacy; the current contracts don't read it |

`Config` is itself an option, so pass it directly: `crudcontract.Creator(subject, cfg)`. `cfg.Helper()` returns a `crudtest.Helper` bound to `cfg.IDA`.

## Changing `port/crud` itself

- Ports are a shared vocabulary. Changing a signature breaks every adapter and domain. Add a new port instead of changing an existing one.
- A new or stricter scenario in a `crudcontract` suite applies to every adapter and every domain contract downstream. Gate it on capabilities the subject actually has (type assertions) or on a `Config` field, and keep all in-repo adapters passing.
- Follow the existing shape: `option.ToConfig`, type-asserted optional capabilities, ctx-cancel scenarios, race scenarios via `raceConcurrently` (resolve `testcase` variables before racing), `t.Eventually`/`c.Helper()` for reads, `c.OnePhaseCommit != nil` variants, and cleanup through `t.Cleanup`/`tryDelete`.
- Register new contracts in `port/crud/crudcontract/crudcontract_test.go` (the compile-time list and `contracts()`). Implement new ports in `adapter/memory.Repository` first.
- Dependants: `adapter/*`, `pkg/cache`, `pkg/httpkit`, `pkg/tasker`, `pkg/workflow`, `port/migration`, `internal/spechelper`. Run `go test -race ./port/crud/... ./port/comproto/... ./adapter/memory/... ./pkg/cache/... ./pkg/httpkit/... ./pkg/tasker/... ./pkg/workflow/... ./port/migration/... ./internal/spechelper/...`.
- `adapter/postgresql`, `adapter/mariadb`, and `adapter/hashicorp/vault` are separate modules. Run `go test -race ./...` inside each, with its environment (for example `PG_DATABASE_URL`; see `scripts/testing-postgresql`). Without it they skip, and a skip is not a pass.

## Reference

- `port/crud/crud.go`: ports, with semantics in the doc comments. `port/crud/errors.go`: `ErrNotFound`, `ErrAlreadyExists`
- `port/crud/crudcontract/`: contracts and `Config`. `references/contract-catalogue.md`: what each contract checks, and what it needs
- `port/crud/crudtest/`: `Helper` and assertion helpers. `port/crud/extid/`: ID conventions and `Accessor`
- `port/comproto/`: `OnePhaseCommitProtocol`, `FinishOnePhaseCommit`, `comprotocontract`
- Domain contracts in this repo to imitate:
  - `pkg/tasker/taskercontract`: own `Config` mapped into `crudcontract.Config`
  - `port/migration/migrationcontract`: subject as its own commit manager
  - `pkg/workflow/wfcontract`: custom QueryMany, optional capabilities, domain specs
  - `pkg/cache/cachecontract`: `CRUD` passthrough and a positional commit manager
- Adapters running contracts: `adapter/memory/Repository_test.go`, `adapter/postgresql/Repository_test.go`, `pkg/httpkit/restclient_test.go`

## Checklist

- [ ] The role interface lives in the consuming package and embeds only the ports it calls, with port method names unchanged.
- [ ] The entity has a dedicated ID type on a field tagged `ext:"id"`, and round-trips through storage unchanged.
- [ ] Domain code checks `found`, uses `errors.Is` with `crud.ErrNotFound`/`crud.ErrAlreadyExists`, checks `err` on every iterator step, and passes `tx` for atomic work.
- [ ] A `<pkg>contract` package exports `func <RoleInterface>(subject, opts...) contract.Contract`, running the matching `crudcontract` suite for every embedded port, QueryOne/QueryMany for custom queries, and domain-specific specs for the rest.
- [ ] Domain requirements (`MakeEntity`, `ChangeEntity`, `Support*`) are set in the contract, and the commit manager is wired: subject, `Config.CommitManager`, or mandatory.
- [ ] Skipped scenarios were audited (`go test -v | grep SKIP`), and gaps are covered with domain specs.
- [ ] Every implementation, including the in-memory fake, runs the contract with `-race` against isolated storage.
- [ ] Domain tests use `memory.Repository` (and `memory.Memory` as the commit manager), not mocks.
