---
name: frameless-ports
description: Design dependencies the frameless way, using ports and adapters (hexagonal architecture) with dependency inversion. The consuming package owns a small role interface composed from go.llib.dev/frameless/port interfaces, plus a `<pkg>contract` package that every supplier, whether a real adapter or an in-memory fake, must pass. Use when code needs a dependency or collaborator (storage, queue, pub/sub, events, locks, files, codecs, transactions, metadata, migrations); when integrating a third-party API, SDK or external service (payments, email); when deciding where an interface belongs; when tempted to write mocks; when writing or running a domain contract; when discovering which ports, contracts and suppliers frameless offers; or when adding a port to frameless. Routes to frameless-software-design for application layout and wiring, and to frameless-crud, frameless-workflow, frameless-resilience and frameless-cli for their areas. Applies to any Go project that imports go.llib.dev/frameless.
compatibility: Go projects importing go.llib.dev/frameless; contracts also use go.llib.dev/testcase.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: port
  scope: Global
---

# Frameless ports

In frameless, the code that needs a dependency owns its definition, in two parts:

- **Role interface.** The consumer declares only what it needs, composed from the ports in `go.llib.dev/frameless/port`.
- **Contract.** A `<pkg>contract` package beside the consumer turns the expected behaviour into tests, mostly by composing the ports' own contracts.

Every supplier runs that contract: a frameless adapter, your own adapter, or an in-memory fake. An interface inverts only the signatures. The contract inverts the behaviour too, so domain code can rely on one meaning of `Publish`, `Lock` or `FindByID`, whatever technology is behind it.

Specialised skills: `frameless-crud` (entity storage and transactions), `frameless-workflow` (workflows), `frameless-resilience` (retries, backoff, rate limits) and `frameless-cli` (command-line adapters). This skill covers the shared model and the other ports. For application structure, directory layout and wiring in `main`, see `frameless-software-design`.

## Stop signs

If you are about to do any of these, follow this skill instead:

- Declaring the interface in the supplier's package, or writing one wide interface that mirrors an SDK or client.
- Calling an SDK, driver or client library (Kafka, Redis, S3, an HTTP API) directly from domain code.
- Inventing verbs or semantics a port already defines. For example, `Enqueue`/`Dequeue`/`Ack(id)` instead of `pubsub.Publisher`/`Subscriber` with `Message.ACK`, or `Acquire`/`Release` instead of `guard.Locker`.
- Naming an interface after a technology (`PostgresStore`, `KafkaProducer`) instead of the role it plays.
- Mocks: gomock, mockery, `testify/mock`, or hand-written stubs with canned answers.
- Hand-writing per-adapter tests for behaviour a port contract already covers.
- Putting a contract in a `_test.go` file, where suppliers can't import it, or making a contract import a supplier.

## The model

| Piece | Lives in | Example |
| --- | --- | --- |
| Port | `go.llib.dev/frameless/port/...` | `pubsub.Publisher[T]`, `guard.LockerFactory[K, L]` |
| Port contract | `<port>contract` beside the port | `pubsubcontract.Queue`, `guardcontract.LockerFactory` |
| Role interface | the consuming package | `billing.InvoiceEvents` embeds `pubsub.Publisher` and `pubsub.Subscriber` |
| Domain contract | `<pkg>contract` beside the consumer | `billingcontract.InvoiceEvents` composes `pubsubcontract.Queue` and `pubsubcontract.Durable` |
| Supplier | adapter packages (frameless's or yours) | `postgresql.Queue[billing.InvoiceIssued]` |
| Fake | an in-memory supplier | `memory.Queue`, or your own for domain-specific ports |

- Dependencies point inward. The consumer knows only its role interfaces. Suppliers import the consumer's package to implement and verify them.
- A fake is a supplier that passes the same contract, so domain tests that use it can be trusted. That is why mocks are unnecessary.
- A shared vocabulary makes generic tooling possible. For example, `pkg/cache.Cache` wraps any crud source, `httpkit.RESTHandlerFromCRUD` serves any repository over HTTP, and `httpkit.RESTClient` is a repository backed by a remote one.
- A published `<pkg>contract` is a consumer-driven contract. Another team's adapter runs it in their CI.

## Workflow

1. **Name the role** from the consumer's point of view: what does it need done? Choose `InvoiceEvents`, not `KafkaClient`.
2. **Discover** the ports, contracts and suppliers that exist ([below](#discover-what-frameless-offers)).
3. **Decide:**
   - If a port fits, embed it.
   - If the need spans several ports, compose them into one role interface (for example `Publisher` + `Subscriber`).
   - If nothing fits, write a domain-specific port in frameless shapes and write its specs yourself.
   - If it isn't a dependency to invert, see [Not everything is a port](#not-everything-is-a-port).
4. **Declare** the role interface in the consuming package. Inject it through exported struct fields. For optional dependencies, write `X [optional]` in the doc comment with a `Default:` line, as `workflow.Runtime` does.
5. **Write `<pkg>contract`.** Compose the port contracts, then add specs for what they don't cover.
6. **Run it** against every supplier with `-race`: the fake in domain tests, and each real adapter in its own tests.

## Discover what frameless offers

```sh
go list go.llib.dev/frameless/port/...                           # ports, with their contracts
go list go.llib.dev/frameless/...contract                        # every contract, including pkg/cache, pkg/tasker, pkg/workflow
go doc -short go.llib.dev/frameless/port/guard                   # the interfaces of a port
go doc go.llib.dev/frameless/port/pubsub/pubsubcontract Durable  # what a contract promises
go list -m -f '{{.Dir}}' go.llib.dev/frameless                   # module source dir, then find suppliers:
grep -rlE 'guardcontract\.[A-Z]' <dir>/adapter <dir>/pkg --include=*_test.go
```

- Paste the printed directory into the next command. Don't use shell substitution.
- In vendor mode, `go list` and `go doc` see only vendored packages, and `.Dir` is empty. Add `-mod=mod` (`go list -mod=mod …`, `GOFLAGS=-mod=mod go doc …`). It reads the module cache and leaves `go.mod` untouched.
- `adapter/postgresql`, `adapter/mariadb` and `adapter/hashicorp/vault` are separate modules, so they aren't inside the root module's directory. Run `go get go.llib.dev/frameless/adapter/postgresql` first, then use `go doc` or the `.Dir` of that module.
- `pkg/cache`, `pkg/tasker` and `pkg/workflow` are consumers in their own right. Each has role interfaces and a domain contract, the same pattern you are applying.

### Catalogue

| Need | Port | Contract | Suppliers in frameless |
| --- | --- | --- | --- |
| entity storage | `port/crud` (see `frameless-crud`) | `crudcontract` | `memory.Repository`, `postgresql.Repository`, `mariadb.Repository`, `vault.Repository`, `httpkit.RESTClient` |
| transactions | `comproto.OnePhaseCommitProtocol` | `comprotocontract` | `memory.Memory`, `postgresql.Connection`, `mariadb.Connection`, `flsql.ConnectionAdapter` |
| queues, events, broadcasts | `pubsub.Publisher`, `pubsub.Subscriber` | `pubsubcontract`: `Queue`, `FIFO`, `LIFO`, `Ordering`, `Durable`, `Volatile`, `Blocking`, `Buffered`, `Broadcast`, `TransactionalPublisher`, `(Non)TransactionalMessageContext` | `memory.Queue` (`LIFO`, `Volatile`, `Blocking` flags), `memory.FanOutExchange`, `postgresql.Queue`, `postgresql.Broadcast` |
| mutual exclusion | `guard.Locker`, `guard.NonBlockingLocker`, `guard.LockerFactory[K, L]` | `guardcontract` | `memory.NewLocker`, `memory.NewLockerFactory`, `postgresql.Lock`/`LockerFactory`, `mariadb.Locker`/`LockerFactory` |
| files | `filesystem.FileSystem` | `filesystemcontract` | `memory.FileSystem`, `localfs.FileSystem` |
| serialisation | `codec.Codec` | `codeccontract` | `jsonkit.Codec`, `jsonkit.LinesCodec`, `formurlencoded.Codec` |
| metadata carried in ctx | `meta.MetaAccessor` | `metacontract` | `memory.EventLog`, `postgresql.MetaAccessor` |
| schema migrations | `migration.StateRepository`, `migration.Migratable` | `migrationcontract` | `postgresql.NewMigrationStateRepository`, `mariadb.MakeMigrationStateRepository` |
| collections | `port/ds` (`Map`, `List`, `Sequence`, …) | `dscontract` | `ds/dsmap`, `ds/dslist`, `ds/dsset`, `synckit.Map` |
| iterators | `iter.Seq`, `iter.Seq2` | `iterkitcontract` | `pkg/iterkit` |
| caching | `pkg/cache` role interfaces | `cachecontract` | `memory.NewCacheRepository`, `mariadb.CacheRepository` |
| scheduled jobs | `pkg/tasker` role interfaces | `taskercontract` | `TaskerScheduler*` in `memory`, `postgresql`, `mariadb` |
| workflows | `pkg/workflow` role interfaces (see `frameless-workflow`) | `wfcontract` | `Workflow*` in `memory`, `postgresql` |

### Not everything is a port

- **Time:** call `clock.Now`, `clock.After` and `clock.Sleep` from `go.llib.dev/testcase/clock`. Control time in tests with `timecop.Travel` and `timecop.SetSpeed`. Don't inject a `Clock`.
- **Logging:** call the `pkg/logger` package functions, and capture output in tests with `logger.Stub(t)`. Don't inject a `Logger`.
- **Retries, backoff, rate limits:** use `pkg/resilience` (see `frameless-resilience`).
- **URL and request paths:** build and parse them with `pkg/pathkit` (see `frameless-pathkit`).
- **Configuration:** load it into structs with `pkg/env`.
- **Pure domain logic and value objects:** use plain functions and types.

## Declare the role interface

From `examples/ports-and-adapters` (see [Reference](#reference)):

```go
package billing

type InvoiceEvents interface { // Behaviour: billingcontract.InvoiceEvents
	pubsub.Publisher[InvoiceIssued]
	pubsub.Subscriber[InvoiceIssued]
}

type InvoiceLocks interface { // Behaviour: billingcontract.InvoiceLocks
	guard.LockerFactory[InvoiceID, guard.Locker]
}

// PaymentGateway is domain-specific: no port covers it.
type PaymentGateway interface { // Behaviour: billingcontract.PaymentGateway
	// Charge is idempotent: charging an invoice again returns the original ChargeID.
	Charge(ctx context.Context, id InvoiceID, amount int) (ChargeID, error)
}

type Collector struct {
	Events  InvoiceEvents
	Locks   InvoiceLocks
	Gateway PaymentGateway
}
```

frameless follows the same shapes: `workflow.Queue` is `Publisher` + `Subscriber`, and `tasker.SchedulerLocks` is `guard.LockerFactory[ScheduleID, guard.Locker]`.

- Embed only what the consumer calls. Every extra method burdens every supplier, and adding one later is cheap.
- Keep the port's verbs and signatures. Contracts and generic tooling match on exact method sets.
- Write domain-specific methods in frameless shapes:
  - `ctx` first
  - typed IDs (`type InvoiceID string`)
  - `(T, found bool, error)` for lookups, because absence is not an error
  - `iter.Seq2[T, error]` for streams
  - sentinel errors as `const ErrX errorkit.Error = "…"`, checked with `errors.Is`
- Write one role interface per need. A single supplier can satisfy several of them.
- Use only domain types in signatures: no SDK, driver or `sql.Null*` types.

Then consume the port semantics as the port defines them:

```go
for msg, err := range c.Events.Subscribe(ctx) { // check err on every step
	if err != nil {
		return err
	}
	if err := c.handle(msg); err != nil {
		return err
	}
}

func (c Collector) handle(msg pubsub.Message[InvoiceIssued]) (rErr error) {
	defer comproto.FinishTx(&rErr, msg.ACK, msg.NACK) // ACK on success, NACK (redeliver) on error

	event := msg.Data()
	l := c.Locks.LockerFor(event.InvoiceID)
	lockCtx, err := l.Lock(msg.Context())
	if err != nil {
		return err
	}
	defer errorkit.Finish(&rErr, func() error { return l.Unlock(lockCtx) })

	_, err = c.Gateway.Charge(lockCtx, event.InvoiceID, event.Amount) // lockCtx is cancelled if the lock is lost
	return err
}
```

## Write the domain contract

```go
package billingcontract

import (
	"context"
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/contract"
	"go.llib.dev/frameless/port/guard"
	"go.llib.dev/frameless/port/guard/guardcontract"
	"go.llib.dev/frameless/port/option"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"example.com/app/domain/billing"
)

// InvoiceEvents is the behaviour billing expects from every billing.InvoiceEvents.
func InvoiceEvents(subject billing.InvoiceEvents, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config](opts)

	pc := pubsubcontract.Config[billing.InvoiceIssued]{MakeContext: c.MakeContext, MakeData: c.MakeInvoiceIssued}
	testcase.RunSuite(s,
		pubsubcontract.Queue(subject, subject, pc),   // one Collector per event
		pubsubcontract.Durable(subject, subject, pc), // nothing is lost between subscriptions
	)
	return s.AsSuite("billing.InvoiceEvents")
}

func InvoiceLocks(subject billing.InvoiceLocks, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config](opts)

	guardcontract.LockerFactory[billing.InvoiceID, guard.Locker](subject, guardcontract.LockerFactoryConfig[billing.InvoiceID]{
		LockerConfig: guardcontract.LockerConfig{MakeContext: c.MakeContext},
		MakeKey:      func(tb testing.TB) billing.InvoiceID { return c.MakeInvoiceIssued(tb).InvoiceID },
	}).Spec(s)
	return s.AsSuite("billing.InvoiceLocks")
}

// PaymentGateway has no port contract to inherit, so its specs are written here.
func PaymentGateway(subject billing.PaymentGateway, opts ...Option) contract.Contract {
	s := testcase.NewSpec(nil)
	c := option.ToConfig[Config](opts)

	s.Describe("#Charge", func(s *testcase.Spec) {
		var (
			ctx   = let.With[context.Context](s, c.MakeContext)
			event = let.Var(s, func(t *testcase.T) billing.InvoiceIssued { return c.MakeInvoiceIssued(t) })
		)
		act := func(t *testcase.T) (billing.ChargeID, error) {
			return subject.Charge(ctx.Get(t), event.Get(t).InvoiceID, event.Get(t).Amount)
		}

		s.When("the invoice was already charged", func(s *testcase.Spec) {
			first := let.Var(s, func(t *testcase.T) billing.ChargeID {
				id, err := act(t)
				assert.NoError(t, err)
				return id
			}).EagerLoading(s)

			s.Then("it returns the original charge", func(t *testcase.T) {
				id, err := act(t)
				assert.NoError(t, err)
				assert.Equal(t, first.Get(t), id)
			})
		})
		// … returns an ID, distinct IDs per invoice, a cancelled ctx fails with context.Canceled
	})
	return s.AsSuite("billing.PaymentGateway")
}

type Option interface{ option.Option[Config] }

// Config lets a supplier tune the contracts. Every field is optional.
type Config struct {
	MakeContext       func(testing.TB) context.Context
	MakeInvoiceIssued func(testing.TB) billing.InvoiceIssued // a valid event for a new, unique invoice
}

func (c *Config) Init() {
	c.MakeContext = func(testing.TB) context.Context { return context.Background() }
	c.MakeInvoiceIssued = func(tb testing.TB) billing.InvoiceIssued {
		t := testcase.ToT(&tb)
		return billing.InvoiceIssued{InvoiceID: billing.InvoiceID(t.Random.UUID()), Amount: t.Random.IntBetween(1, 100000)}
	}
}

func (c Config) Configure(t *Config) { *t = reflectkit.MergeStruct(*t, c) }
```

- **Packaging.** Write a regular package named `<pkg>contract`. It imports the consumer, ports, port contracts and testcase, and never a supplier.
- **Shape.** Export one function per role interface, `func X(subject pkg.X, opts ...Option) contract.Contract`, returning `s.AsSuite("pkg.X")`.
- **Composition.** Mount port contracts with `testcase.RunSuite(s, …)` or `.Spec(s)`. Then add `s.Describe` specs for domain-specific ports, invariants and coverage gaps. Follow the `testcase-spec` skill for spec style.
- **Configuration.** `option.ToConfig` runs `Init` for defaults, then applies the options. `Configure` merges only non-zero fields, so a supplier passes just what it needs, for example `billingcontract.Config{MakeContext: withTenant}`.
- **Domain requirements live in the contract.** This includes generators that produce valid, unique data (random IDs), because scenarios share the supplier's storage.
- **Port contract config.** Port contracts have their own config types. Map yours onto them, and read them with `go doc` for capability flags and hooks such as `pubsubcontract.Config.SupportPublishContextCancellation` or `guardcontract.LockerConfig.MakeLockLost`.
- **Running.** A `contract.Contract` offers `.Test(t)`, `.Spec(s)` and `.Benchmark(b)`.

### Skipped is not passed

Contracts register scenarios based on the subject's capabilities and on optional hooks. Skips are visible only with `go test -v`. For example, the in-memory locker can't lose a lock, so it skips the lock-loss scenarios, which need `MakeLockLost`. Audit with `go test -v ./... | grep -- '--- SKIP'`. If the domain relies on skipped behaviour, either expose the hook in your `Config` or write the spec yourself.

## Run the contract against every supplier

```go
// Domain tests: the frameless in-memory suppliers become trusted fakes once they pass.
func TestInvoiceEvents_memory(t *testing.T) {
	billingcontract.InvoiceEvents(&memory.Queue[billing.InvoiceIssued]{}).Test(t)
}

func TestInvoiceLocks_memory(t *testing.T) {
	billingcontract.InvoiceLocks(memory.NewLockerFactory[billing.InvoiceID, guard.Locker]()).Test(t)
}

// The fake for a domain-specific port.
func TestPaymentGateway(t *testing.T) {
	billingcontract.PaymentGateway(&inmemory.PaymentGateway{}).Test(t)
}

// A real supplier against a dedicated test database.
func TestInvoiceEvents(t *testing.T) {
	events := postgresql.Queue[billing.InvoiceIssued]{Table: "billing_invoice_issued", Connection: conn}
	assert.NoError(t, events.Migrate(ctx))
	billingcontract.InvoiceEvents(events).Test(t)
}
```

- Use-case tests wire the verified fakes into the consumer, for example `billing.Collector{Events: &memory.Queue[…]{}, Locks: memory.NewLockerFactory[…](), Gateway: &inmemory.PaymentGateway{}}`. A fake can expose observation methods for assertions (`Charged(id)`); domain code sees only the role interface.
- Put fakes for your own ports in a regular package (for example `adapter/inmemory`) rather than in `_test.go`, so every consumer's tests can use them.
- Run with `-race`, against isolated resources: a dedicated database, unique table names. Contracts purge and drain what they test.

## Implement a supplier

- Assert the role interface at compile time: `var _ billing.InvoiceLocks = InvoiceLocks{}`.
- Translate the technology's dialect into port semantics:
  - "not found" becomes `found == false`
  - a failed delivery becomes a NACK and a redelivery
  - driver errors are wrapped with `%w`
- **Go has no covariant return types.** `postgresql.LockerFactory[K].LockerFor` returns `*postgresql.Lock`, so it doesn't satisfy `guard.LockerFactory[K, guard.Locker]`. Wrap it in a named type whose method returns the port type, as `postgresql.TaskerSchedulerLocks` and `WorkflowProcessLocks` do:

  ```go
  type InvoiceLocks struct{ Connection postgresql.Connection }

  func (l InvoiceLocks) LockerFor(id billing.InvoiceID) guard.Locker {
  	return &postgresql.Lock{Name: "billing/invoice/" + string(id), Connection: l.Connection} // the lock table is shared
  }

  func (l InvoiceLocks) Migrate(ctx context.Context) error {
  	return (&postgresql.Lock{Connection: l.Connection}).Migrate(ctx)
  }
  ```

- Implement `migration.Migratable` (`Migrate(ctx) error`) when the supplier owns schema, and forward it from wrappers.
- Make the contract pass first. Then add supplier-specific tests only for what the contract can't know: mapping edge cases, migrations, performance.

## Changing frameless ports

- A port is a shared vocabulary, so changing a signature breaks every consumer and supplier. Add a new port, or an optional interface, instead.
- Every new port needs a `<port>contract`, and an `adapter/memory` supplier that passes it.
- New or stricter contract scenarios apply to every supplier downstream. Gate them on capabilities (type assertions) or on `Config` fields. Keep every in-repo supplier passing, including the separate modules under `adapter/`, which need their environment (for example `scripts/testing-postgresql`). A skip is not a pass.

## Reference

- `examples/ports-and-adapters` in the frameless repository holds the complete example above, runnable, with a fake, a use-case spec, PostgreSQL suppliers and a `cmd/collector` composition root. It is a separate module, so it isn't in the module cache. Online: https://github.com/adamluzsi/frameless/tree/main/examples/ports-and-adapters
- Domain contracts in frameless to imitate:
  - `pkg/tasker/taskercontract`: `SchedulerLocks` over `guardcontract.LockerFactory`
  - `pkg/workflow/wfcontract`: `Queue`, `ProcessLocks`, `NotificationBroadcast`
  - `port/migration/migrationcontract`
  - `pkg/cache/cachecontract`
- `port/option` is the `Option`/`Init`/`Configure` pattern. `port/contract` defines `Contract`.
- `port/README.md` and `port/intro.md` explain the motivation. The code in `intro.md` uses an older, struct-based contract API, so don't copy it.

## Checklist

- [ ] The role interface lives in the consumer, is named after the role, embeds only what is called, and keeps port verbs unchanged.
- [ ] Domain-specific methods use frameless shapes and domain types only.
- [ ] A `<pkg>contract` regular package exports one function per role interface. Each composes port contracts plus hand-written specs, configured through `Config` with `Init`/`Configure`.
- [ ] Every supplier, fakes included, runs the contract with `-race` against isolated resources, and skips were audited.
- [ ] Domain tests use verified fakes, not mocks.
- [ ] Suppliers assert the role interface at compile time, and wrap covariant-return mismatches.
