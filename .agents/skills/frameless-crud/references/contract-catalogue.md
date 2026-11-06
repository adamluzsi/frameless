# `crudcontract` catalogue

What each contract in `go.llib.dev/frameless/port/crud/crudcontract` checks, and which capabilities a scenario needs. Contracts find optional capabilities through type assertions on the subject's **dynamic** type. They register a scenario only when the capability is there, or skip it at run time, and skips are silent unless you run `go test -v`.

Terms used below:

- **Seeding**: how a contract puts data into the resource. It uses `crud.Creator`, then `crud.Saver`, then `Config.ExampleEntity` (an entity that already exists). QueryMany can also seed through `Batcher`.
- **Wipe**: `Purge` (`crud.Purger`), falling back to `DeleteAll` (`crud.AllDeleter`). Contracts wipe the subject before or after some scenarios, so never point one at shared data.
- **+X**: the scenario only exists when the subject also implements X.
- **tx**: the scenario exists when `Config.OnePhaseCommit` is set.

## Port → contract

| Port in the role interface | Contract constructor |
| --- | --- |
| `crud.Creator[ENT]` | `Creator[ENT, ID](subject, opts...)` |
| `crud.Saver[ENT]` | `Saver[ENT, ID](subject, opts...)` |
| `crud.ByIDFinder[ENT, ID]` | `ByIDFinder[ENT, ID](subject, opts...)` |
| `crud.AllFinder[ENT]` | `AllFinder[ENT, ID](subject, opts...)` |
| `crud.Finder[ENT, ID]` | `Finder[ENT, ID]`, which runs ByIDFinder and AllFinder |
| `crud.ByIDsFinder[ENT, ID]` | `ByIDsFinder[ENT, ID](subject, opts...)` |
| `crud.Updater[ENT]` | `Updater[ENT, ID](subject, opts...)` |
| `crud.ByIDDeleter[ID]` | `ByIDDeleter[ENT, ID](subject, opts...)` |
| `crud.AllDeleter` | `AllDeleter[ENT, ID](subject, opts...)` |
| `crud.Deleter[ID]` | `Deleter[ENT, ID]`, which runs ByIDDeleter and AllDeleter |
| `crud.Purger` | `Purger[ENT, ID](subject, opts...)`; the subject must also be Creator + ByIDFinder + ByIDDeleter |
| `crud.Batcher[ENT, BATCH]` | `Batcher[ENT, ID, BATCH](subject, opts...)`; `Batcher[ENT, ID](subject, cfg)` infers BATCH |
| custom "query one" method | `QueryOne[ENT, ID](resource, "MethodName", func(testing.TB) QueryOneSubject[ENT], opts...)` |
| custom "query many" method | `QueryMany[ENT, ID](resource, "MethodName", func(testing.TB) QueryManySubject[ENT], opts...)` |
| `comproto.OnePhaseCommitProtocol` | `OnePhaseCommitProtocol[ENT, ID](subject any, manager, opts...)` |

Every constructor returns a `contract.Contract` (`.Test(t)`, `.Spec(s)`, `.Benchmark(b)`). Options are `crudcontract.Config[ENT, ID]` values or other `crudcontract.Option[ENT, ID]` implementations.

## Creator

- Creating a new entity succeeds.
- Concurrent `Create` calls are race-free.
- A cancelled ctx → `context.Canceled`.
- If `ENT` has an ID field, `Create` writes a non-zero ID into `*ptr`.
- +ByIDFinder: the created entity is findable and equal to `*ptr`.
- +AllFinder: the created entity is listed by `FindAll`.
- Unless `NonUniqueID`: creating the same entity twice → `crud.ErrAlreadyExists`. Skipped when the entity still has no non-zero ID.
- `SupportIDReuse` +ByIDDeleter: creating with the ID of a deleted entity succeeds.
- `SupportRecreate` +ByIDDeleter: deleting, then creating the same values again, succeeds.
- tx: `BeginTx → Create → CommitTx` persists. With ByIDFinder, the entity is invisible outside the tx before the commit. `RollbackTx` discards. `Create` with a finished tx errors.

## Saver

`MakeEntity` must yield a non-zero ID, unless the adapter assigns one.

- An absent entity is created. Verified through ByIDFinder; skipped without it.
- An entity whose ID no longer points to a record is created.
- A present entity is updated.
- A newer version (changed via `ChangeEntity`) replaces the stored one. +AllFinder: the entity count does not grow.
- Concurrent `Save` calls are race-free.
- tx: committed saves become visible, and are invisible outside the tx before the commit. Rolled-back saves are not persisted. `Save` with a finished tx errors.
- There is no ctx-cancel scenario for `Save` today. The domain contract can add one.

## ByIDFinder

Needs seeding; without it, the scenarios skip.

- An existing ID → `(ent, true, nil)`, equal to what was stored.
- +ByIDDeleter: a deleted ID → `found == false` with no error.
- Built on QueryOne (`#FindByID`):
  - a cancelled ctx → `context.Canceled` with `found == false`;
  - other stored entities don't change the answer;
  - an empty resource → not found (needs a wipe, else skipped);
  - only unrelated entities stored → not found.
- Concurrent `FindByID` calls are race-free.
- `NonUniqueID` logs a warning, because lookups become non-deterministic.

## AllFinder

Runs QueryMany (`#FindAll`) with `IncludedEntity` taken from `ExampleEntity` or `MakeEntity`, and no `ExcludedEntity`. See [QueryMany](#querymany).

## ByIDsFinder

Needs seeding.

- No IDs → an empty result, no error.
- IDs of stored entities → exactly those entities, in any order.
- +ByIDDeleter: one ID is missing → the first item the iterator yields is `crud.ErrNotFound`.

## Updater

Wipes the subject before each scenario.

- Seeding through Creator or Saver:
  - the stored entity is updated with `ChangeEntity`'s changes (verified with +ByIDFinder);
  - concurrent updates are race-free;
  - a cancelled ctx → `context.Canceled`.
- Seeding +ByIDDeleter: an unknown ID → `crud.ErrNotFound`.
- tx (with seeding): a committed update is visible, and invisible outside the tx before the commit. A rolled-back update leaves the old values. `Update` with a finished tx errors.
- Includes a benchmark.

## ByIDDeleter

- A cancelled ctx → `context.Canceled`. This always runs.
- +Creator only, so a `Saver`-only subject gets none of these:
  - the deleted entity is absent (+ByIDFinder);
  - other entities are unaffected (+ByIDFinder);
  - concurrent deletes are race-free (+ByIDFinder);
  - the entity is not listed anymore (+AllFinder);
  - deleting it again → `crud.ErrNotFound`.
- tx (seeding or `ExampleEntity`): a committed delete removes the entity, and a rolled-back delete keeps it (+ByIDFinder). `DeleteByID` with a finished tx errors.

## AllDeleter

- A cancelled ctx → `context.Canceled`.
- +Creator: after `DeleteAll`, `FindByID` reports not found (+ByIDFinder) and `FindAll` is empty (+AllFinder).
- +AllFinder: `FindAll` is empty after `DeleteAll`, and concurrent `DeleteAll` calls are race-free.
- tx (seeding or `ExampleEntity`): a committed `DeleteAll` removes entities, and a rolled-back one keeps them. `DeleteAll` with a finished tx errors.
- Includes a benchmark.

## Purger

The subject must be Creator + ByIDFinder + ByIDDeleter + Purger.

- +AllFinder: the resource is empty after `Purge`, with or without prior entities. Skipped otherwise.

## Batcher

Wipes the subject before and after each scenario.

- `Batch → Add… → Close` succeeds, and `Close` is idempotent.
- +AllFinder: added entities are listed. IDs are compared when `MakeEntity` sets them.
- A cancelled ctx → the batch fails with the ctx error. Cancelling during `Add` fails `Add` and `Close`; cancelling before `Close` fails `Close`.
- (`SupportIDReuse` or `SupportRecreate`) +ByIDDeleter: pre-assigned IDs are accepted, and the entities are findable (+ByIDFinder) and listed (+AllFinder). Needs +Creator when `MakeEntity` leaves IDs zero.
- tx: committed batches persist and rolled-back ones don't (+AllFinder). A finished tx makes `Add` or `Close` fail.

## QueryOne

`resource` is used for seeding (Creator or Saver), for wiping, and for removing the expected entity. It is usually the subject.

`QueryOneSubject[ENT]` fields:

- `Query func(ctx) (ENT, bool, error)`: the call under test, with its arguments already bound. Pass `ctx` through as is, since it may carry a tx.
- `ExpectedEntity ENT`: the contract makes sure it is present. It looks it up by ID (+ByIDFinder) and stores it if absent, then deletes it after the test. The subject func stores it itself (e.g. with `crudtest.Create`) and cleans it up when the resource can't store it, or when the query needs its ID up front, like `#FindByID`. That cleanup must tolerate the entity already being absent, since the contract may delete it.
- `ExcludedEntity func() ENT`: optional maker of entities that must not match. The contract stores them.

The resource is wiped (Purger or AllDeleter) before the subject func runs. For the no-result scenarios, the contract removes the expected entity. It uses `DeleteByID` when the entity has an ID and the resource is a ByIDDeleter, and `DeleteAll` otherwise. Without either, those scenarios are skipped.

Scenarios. Found and not-found results are polled with `t.Eventually`.

- The expected entity is returned with `found == true`.
- A cancelled ctx → `context.Canceled` with `found == false`.
- Excluded entities are stored too → still the expected entity.
- The expected entity is removed → `found == false`. The same holds when 3–7 excluded entities are stored too.
- tx (`Config.OnePhaseCommit`):
  - A query within the tx returns the expected entity. Removing it in the same tx → `found == false`. A finished tx → error with `found == false`.
  - Removing it within a tx: still returned outside the tx; after commit `found == false`; after rollback it is returned.

## QueryMany

`resource` is used for seeding (Creator, Saver, or Batcher) and wiping.

`QueryManySubject[ENT]` fields:

- `Query func(ctx) iter.Seq2[ENT, error]`
- `IncludedEntity func() ENT`: called repeatedly; must return unique, matching entities.
- `ExcludedEntity func() ENT`: optional; its scenario is skipped when nil.

Scenarios:

- An included entity is returned (polled with `t.Eventually`).
- Several included entities → all returned.
- Two iterations of the query can be interleaved, and both complete.
- +ByIDFinder: calling `FindByID` while iterating returns entities equal to the yielded ones.
- Concurrent iterations are race-free.
- An excluded entity is not returned.
- A cancelled ctx → the iterator yields the ctx error.

## OnePhaseCommitProtocol

Always runs against the `manager`, via `comprotocontract`:

- `BeginTx` then `CommitTx` or `RollbackTx` succeeds once. A second finish, or mixing commit and rollback, errors.
- Commit and rollback cancel the tx context.
- Nested `BeginTx` works.
- A cancelled ctx makes `BeginTx`, `CommitTx`, and `RollbackTx` error.
- `#InTx` scenarios run when the manager implements `comproto.InTx`.

When the subject is Creator + ByIDFinder + ByIDDeleter:

- After commit or rollback, `Create`, `FindByID`, and `DeleteByID` with that tx error. So do `FindAll`, `Update`, and `DeleteAll` when implemented.
- Create in a tx: present inside the tx, absent outside. After commit present; after rollback absent.
- Delete in a tx: absent inside the tx, present outside. After commit absent; after rollback present.
- Nested `BeginTx` → `BeginTx` with creates in both levels: committing both publishes both entities.

When the subject is a Saver: the same family of scenarios, with `Save`. The visibility checks need +ByIDFinder, and the delete variants need +ByIDDeleter.

When the subject also implements Purger: purge-in-tx scenarios run (see known issues).

When the subject has none of these capabilities: only the manager is tested, and a warning is logged.

## Known issues

These were observed when this file was written; re-check the source before relying on them.

- `specOPCPPurger` (`OnePhaseCommitProtocol.go`) asserts `IsAbsent` and then `IsPresent` on the same global context, probably meaning to check the tx first. So a subject implementing Creator + ByIDFinder + ByIDDeleter + Purger, passed to `OnePhaseCommitProtocol`, fails those two scenarios. No in-repo crud repository implements `Purger`, so this is unexercised.
- `Config.LazyNotFoundError` is not read by any contract.
- The `extid.Accessor` doc comment mentions `ID()`/`SetID()` methods; `extid` doesn't support them. Its example also writes the accessor as `func(v Foo) *FooID`, but the real signature is `func(*ENT) *ID`.
- `port/intro.md` shows an older, struct-based `crudcontracts` API. Use the function-based `crudcontract` API.
