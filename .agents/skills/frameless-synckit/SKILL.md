---
name: frameless-synckit
description: Use go.llib.dev/frameless/pkg/synckit instead of hand-rolled Go concurrency plumbing. Covers Init/InitErr (lazy, lock-guarded field initialisation), Go and Group (errgroup-style goroutines with cancellation, Isolation and panic propagation), Limiter and LimiterWithCeiling (concurrency caps, semaphores, worker pools), Phaser (latch, barrier and sync.Cond replacement), RWLockerFactory (per-key locks), concurrency-safe Map and Slice, Fan and Broadcast (in-process pub/sub with ACK/NACK), and IsDone. Use when writing goroutines, sync.WaitGroup or errgroup code, semaphore channels, sync.Cond, double-checked locking, mutex-guarded maps or slices, keyed locks, worker pools, fan-out/fan-in pipelines or in-process event broadcasts; when debugging deadlocks or goroutine leaks in such code; when testing concurrent code; or when changing pkg/synckit itself. Applies to any Go project that imports go.llib.dev/frameless.
compatibility: Go projects importing go.llib.dev/frameless/pkg/synckit.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: pkg/synckit
  scope: Global
---

# Frameless synckit

`go.llib.dev/frameless/pkg/synckit` fills the gaps in `sync`. It provides lazy initialisation, cancellable goroutines and groups, concurrency caps, phasers, per-key locks, typed concurrent collections and in-process pub/sub.

Two rules apply to every type in the package:

1. **The zero value is ready to use.** Embed them as struct fields by value (`jobs synckit.Group`, `cache synckit.Map[K, V]`) or declare them as `var`. There are no constructors.
2. **Never copy one after first use.** It is the same rule as for `sync.Mutex`. Use pointer receivers on types that hold them. `go vet` (copylocks) flags value receivers, such as `func (t MyType) Do()` on a struct with a `Group` or `LimiterWithCeiling` field.

```go
import "go.llib.dev/frameless/pkg/synckit"
```

## Stop signs

If you're about to write any of the following, use `synckit` instead:

- `var wg sync.WaitGroup` together with an `errCh`/`errs []error` to collect goroutine failures, or `golang.org/x/sync/errgroup`. Use `Group`.
- `sem := make(chan struct{}, n)` or `golang.org/x/sync/semaphore` to cap parallelism. Use `Limiter` or `LimiterWithCeiling`.
- Double-checked locking: `if c.x == nil { c.mu.Lock(); if c.x == nil { c.x = … } }`. Use `Init` or `InitErr`.
- A `sync.Once` plus a separate `err` field, where a failed initialisation should be retried. Use `InitErr`.
- A `struct { mu sync.RWMutex; m map[K]V }` with hand-written `Get`/`Set` wrappers, or `sync.Map` with type assertions. Use `Map`.
- A mutex-guarded slice used to collect results from goroutines. Use `Slice`.
- `map[K]*sync.Mutex` keyed locks, which never get cleaned up. Use `RWLockerFactory`.
- `sync.NewCond`, or a `chan struct{}` start gate or barrier that you close by hand. Use `Phaser`.
- A bare `go func() { … }()` that callers can't wait on or cancel. Use `synckit.Go`, and keep the `Job`.
- A slice of channels used to fan out or broadcast values in-process. Use `Fan` or `Broadcast`.
- `select { case <-done: return true; default: return false }`. Use `synckit.IsDone(done)`.

Don't add `errgroup`, `semaphore`, `conc`, `ants` or similar libraries to a project that already depends on frameless.

## Pick the tool

| Need | Use |
| --- | --- |
| Lazily create a field (client, connection, ID) exactly once, under a lock you already have | `synckit.Init` / `synckit.InitErr` |
| Run one background goroutine, and wait on it or cancel it later | `job := synckit.Go(ctx, fn)` |
| Run N goroutines, wait for all, collect errors, fail fast or isolate | `var g synckit.Group` |
| Cap how many goroutines do something at once | `synckit.Limiter` (runtime limit) / `LimiterWithCeiling[C]` (type-level limit) |
| Park goroutines until a condition, a signal or a shutdown | `synckit.Phaser` |
| Lock per key (per user, per account, per file) | `synckit.RWLockerFactory[K]` |
| Concurrent map / list with typed values | `synckit.Map[K, V]` / `synckit.Slice[T]` |
| In-process work queue: each message goes to exactly one subscriber, with ACK/NACK | `synckit.Fan[T]` |
| In-process notifications: every current subscriber gets a copy | `synckit.Broadcast[T]` |
| Non-blocking "is this done channel closed?" | `synckit.IsDone(ch)` |

## Lazy initialisation: `Init`, `InitErr`

```go
type Client struct {
	DSN string

	mu sync.RWMutex
	db *sql.DB
}

func (c *Client) conn() (*sql.DB, error) {
	return synckit.InitErr(&c.mu, &c.db, func() (*sql.DB, error) {
		return sql.Open("pgx", c.DSN) // on error nothing is stored, and the next call retries
	})
}
```

- The lock can be `*sync.Once`, `*sync.Mutex` or `*sync.RWMutex`. `InitErr` only accepts the mutexes. `InitErrWL` accepts any `sync.Locker`.
- **With a mutex, "initialised" means `*ptr != zero`.** If `init` returns the zero value (`nil`, `0`, `""`), it runs again on the next call. Use pointer types, or a `*sync.Once`, when zero is a legitimate result.
- **With `*sync.Once`, the first result is cached forever, even a zero value or an error.** This makes it the idiom for "close once and remember the error":

  ```go
  func (b *Batch) Close() error {
  	return synckit.Init(&b.closeOnce, &b.closeErr, func() error { return b.flush() })
  }
  ```

- `T` must be `comparable`. Store `*T` for structs that contain slices, maps or funcs.
- `*sync.RWMutex` gives a read-locked fast path, which is the best choice for hot accessors.
- `init` runs while the lock is held. Don't call `Init` with the same lock from inside `init`, because it deadlocks. Use one lock (or one `Once`) per lazily initialised field.
- Every read of the field must go through the accessor, or hold the same lock.

## Goroutines: `Go` and `Group`

```go
func (s *Service) Dashboard(ctx context.Context, userID string) (Dashboard, error) {
	var (
		g synckit.Group
		d Dashboard
	)
	g.Go(ctx, func(ctx context.Context) (err error) {
		d.Profile, err = s.profile(ctx, userID)
		return err
	})
	g.Go(ctx, func(ctx context.Context) (err error) {
		d.Orders, err = s.recentOrders(ctx, userID)
		return err
	})
	if err := g.Wait(); err != nil { // every task has returned once Wait returns
		return Dashboard{}, err
	}
	return d, nil
}
```

`Group` rules:

- **By default the group fails fast.** The first task error cancels the context of every *running* task. An error that equals the task's own `ctx.Err()` doesn't trigger this. Use `Group{Isolation: true}` for independent tasks, such as background jobs or per-item work, where one failure must not stop the others.
- **`Wait` returns every collected error, merged.** That includes the `context.Canceled` returned by cancelled siblings. Match errors with `errors.Is`, never with `==`.
- **Panics are recovered in the task and re-raised by `Group.Wait`** in the goroutine that waits. `Job.Wait` of a panicked task returns `nil`, so always call `Group.Wait`.
- `ErrorOnGoexit: true` turns `runtime.Goexit` (for example a `t.FailNow` inside a task) into `synckit.ErrGoexit`.
- `Wait` drains the collected errors, so the group can be reused. Start a new batch only after `Wait` has returned.
- `Cancel()` cancels the tasks that are running now. It doesn't wait, and it isn't sticky: later `Go` calls run normally. In submit loops, stop submitting yourself; see [patterns](references/patterns.md#bounded-worker-pool).
- Shutdown idiom: `g.Cancel(); return g.Wait()`. Expect `context.Canceled` in the result, and filter it with `errors.Is` if a clean shutdown should return `nil`.
- Each task's context derives from the `ctx` passed to *that* `Go` call. Every task context is released when the task returns, so long-lived contexts don't leak.
- `*Group` implements `Job` (`Wait`, `Done`, `Cancel`). `Len()` counts running tasks. `Done()` of an empty group is already closed. Tasks may call `g.Go` themselves.
- `Group.Go(nil, …)` falls back to `context.Background()`.

`synckit.Go(ctx, fn) Job` rules:

- **`ctx` must not be nil**, or `Go` panics.
- **Panics in `fn` are not recovered**, just like a bare `go` statement. Use a `Group` if you need recovery.
- `Wait()` returns the same error every time. `Cancel()` is idempotent. `Done()` closes when `fn` returns.
- Keep the `Job` in a field (for example `keepAlive synckit.Job`). On shutdown, call `Cancel()` and then `Wait()`.

## Concurrency caps: `Limiter`, `LimiterWithCeiling`

```go
type Indexer struct {
	limiter synckit.LimiterWithCeiling[synckit.LimitToNumCPU] // shared cap: at most NumCPU documents at once, across all callers
}

func (ix *Indexer) IndexAll(ctx context.Context, docs []Doc) error {
	var g synckit.Group
	for _, doc := range docs {
		ix.limiter.Lock() // the loop blocks while the cap is reached
		g.Go(ctx, func(ctx context.Context) error {
			defer ix.limiter.Unlock()
			return ix.index(ctx, doc)
		})
	}
	return g.Wait()
}
```

- `Limiter`'s zero value is **unlimited**. Use `SetLimit(n)` to change it at runtime: `n > 0` caps it, `0` makes it unlimited, and `n < 0` blocks every `Lock` (`TryLock` returns `false`) until a new limit is set.
- Raising the limit wakes waiters. Lowering it never preempts current holders: new holders wait until fewer than the new limit are held.
- A holder isn't tied to a goroutine. Taking `Lock` in the loop and calling `Unlock` in the spawned goroutine is the intended semaphore pattern. `Unlock` without a held slot panics.
- `Lock` doesn't take a `ctx`. On shutdown, call `SetLimit(0)` so blocked goroutines get in and see their cancelled context.
- `LimiterWithCeiling[C]` puts the cap in the type, so the zero value is configured. `C` is any `LimitCeiling` (`Limit() int`). `LimitToNumCPU` ships with the package. `Limit()` is called on every `Lock`/`TryLock`, so keep it cheap.
- Both types implement `sync.Locker` and `synckit.TryLocker`.
- `Limiter` caps *concurrency*. To cap *rate* (calls per period), use `pkg/resilience` (`SlidingWindow`).

## Coordination: `Phaser`

```go
func (b *Buffer) Take() (Item, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.items) == 0 && !b.closed {
		b.ready.Wait(&b.mu) // releases b.mu while parked, re-acquires it before returning
	}
	if len(b.items) == 0 {
		return Item{}, false
	}
	it := b.items[0]
	b.items = b.items[1:]
	return it, true
}
// Put: lock, append, unlock, then b.ready.Broadcast() (or Signal() to wake one).
// Close: lock, closed = true, unlock, then b.ready.Finish().
```

- `Signal()` releases one waiter and `Broadcast()` releases every current waiter. **Neither is latched**: a call with no waiter is lost. Always wait in a loop on a condition guarded by a lock.
- `Finish()` releases everyone and makes every future `Wait` return immediately. It is permanent; there is no reset.
- `Wait(lockers...)` follows `sync.Cond` semantics. The caller must hold the lockers. They are released while parked and re-acquired before `Wait` returns.
- `Len()` reports how many goroutines are parked. Use it in tests (`Eventually(p.Len() == n)`) and for "everyone else is already waiting" logic.

## Per-key locks: `RWLockerFactory`

```go
type Wallets struct {
	locks synckit.RWLockerFactory[WalletID]
}

func (w *Wallets) Withdraw(ctx context.Context, id WalletID, amount int) error {
	l := w.locks.RWLocker(id) // a fresh locker per critical section
	l.Lock()
	defer l.Unlock()
	// …
}
```

- Different keys never block each other. Entries are removed when their last user unlocks, so an unbounded key space doesn't leak.
- **A returned `RWLocker` tracks one `Lock` and one `RLock` at a time.** Don't share it between goroutines or take two read locks on it. Call `RWLocker(key)` for each use. Unlocking a locker that isn't locked panics.
- When you need several keys, lock them in a consistent order (sort the keys) to avoid deadlocks.
- `ReadOptimised: true` makes reads much faster for read-heavy keys. A read on a key that has no active entry takes the factory-wide read lock instead, so writers that need a new entry wait for those reads to finish.
- The locks are process-local. For locks across replicas, use `port/guard`.

## Concurrent collections: `Map`, `Slice`

```go
var handlers synckit.Map[string, Handler] // package-level reflection and registry caches are a common use

handlers.Set(name, h)
h, ok := handlers.Lookup(name)
n := counters.GetOrInit(key, func() int { return 0 })

_ = counters.Do(func(vs map[string]int) error { vs[key]++; return nil }) // atomic read-modify-write
```

- **A single `RWMutex` guards the whole map, and the locks aren't re-entrant.** The `Do` callback, the `init` of `GetOrInit`/`GetOrInitErr`, the time between `Borrow`/`BorrowWithInit` and `release()`, and the body of an `All` loop all run under the write lock. Keep them short, and never call the same `Map` from inside them, because it deadlocks.
- `GetOrInit` runs `init` under the write lock, so a slow `init` blocks the whole map. For slow per-key setup, store a holder and initialise it with `InitErr`; see [patterns](references/patterns.md#per-key-lazy-clients).
- `GetOrInitErr` doesn't cache errors. `GetOrInit(key, nil)` returns the zero value and stores nothing.
- Iteration rules:
  - `Keys()` and `Values()` take a snapshot, so the loop body may mutate the map.
  - `All()` holds the write lock and `RAll()` holds the read lock while each entry is yielded. Never mutate the map inside those loops; iterate `Keys()` or `ToMap()` instead.
- `ToMap()` returns a copy. JSON (un)marshals as a plain object. `Map` implements `ds.Map`, and `Slice` implements `ds.List`.
- `Slice` provides `Append`, `Lookup`, `Set`, `Insert`, `Delete`, `Len` and `ToSlice` (a copy). `Values()`/`RValues()` hold the lock for the **whole** loop, so don't touch the slice in the body. Iterate `ToSlice()` if you need to.

## In-process pub/sub: `Fan`, `Broadcast`

```go
var queue synckit.Fan[Task] // unicast: each message goes to exactly one subscriber

for msg, err := range queue.Subscribe(ctx) { // consumer
	if err != nil {
		return err
	}
	handle(msg.Context(), msg.Data())
	_ = msg.ACK()
}

err := queue.Publish(ctx, task) // producer: blocks until a subscriber takes the message

_ = queue.Close() // shutdown: reject new publishes; subscriptions drain, then end
queue.Cancel()    // Close, and also cancel the contexts of in-flight messages
```

- **ACK every message.** A message that isn't ACKed by the end of its loop iteration is NACKed automatically and redelivered, forever if it keeps being skipped. NACK only failures that are worth retrying.
- `Fan.Publish` is a synchronous hand-off. It returns when a subscriber *takes* the message, not when it's ACKed. It blocks while nobody is subscribed. It returns `ctx.Err()` when `ctx` is done, and `context.Canceled` after `Close` or `Cancel`.
- `Broadcast.Publish` delivers a copy to every *current* subscriber and waits for each of them, so a slow subscriber slows the publisher. With no subscribers it is a no-op that returns `nil`. Late subscribers miss earlier messages.
- `Close` is permanent: a later `Subscribe` ends immediately. `Len()` counts active subscriptions; wait for `Len() == n` before you publish to a `Broadcast` in tests.
- Both types implement `pubsub.Publisher[T]` and `pubsub.Subscriber[T]`, and `Fan` passes `pubsubcontract.Queue`. They are volatile and in-memory. Use them as in-process plumbing, or to wire a pub/sub port inside one process, not as durable queues.

## `IsDone`

`synckit.IsDone(ch)` reports without blocking whether a close-only done channel is closed. It accepts `ctx.Done()`, `job.Done()` or `g.Done()`. A `nil` channel reports `false`. **It panics if the channel delivers a value**, so never use it on data channels.

## Where synckit stops

- **Locks across processes or replicas**, or leader election: use `port/guard` (`guard.Locker`, `guard.LockerFactory`) with the `memory`, `postgresql`, `mariadb` or `localfs` suppliers. See `frameless-ports`.
- **Durable or cross-service messaging, or a domain that depends on pub/sub**: use the `port/pubsub` role interfaces with `memory.Queue` as the fake and `postgresql.Queue` in production. See `frameless-ports`.
- **App-level background services, graceful shutdown on signals, scheduled or repeated jobs**: use `pkg/tasker` (`tasker.Main`, `Concurrence`, `WithRepeat`, `WithNoOverlap`).
- **Retries, backoff, polling, rate limits**: use `pkg/resilience`. See `frameless-resilience`.
- **Time** (`time.Now`, `time.Sleep`, tickers): use `go.llib.dev/testcase/clock`. See `testcase-clock`.

## Testing concurrent code

Summary below. The full guide, with examples, is in [references/testing.md](references/testing.md).

- Make goroutines deterministic with a `Phaser` gate: the fake calls `p.Wait()`, the test waits for `Eventually(p.Len() == n)`, asserts, then calls `p.Broadcast()` or `p.Finish()`. Register `t.Cleanup(p.Finish)` so nothing leaks. In specs, `let.Phaser(s)` gives you the testcase equivalent with the cleanup already registered.
- Assert blocking with `assert.NotWithin(t, timeout, fn)`. It returns a waiter, so you can check with `assert.Within` that `fn` finishes once released. Use `t.Eventually` for asynchronous outcomes. Never `time.Sleep`.
- Collect results from goroutines with `synckit.Slice` or `synckit.Map`. Test pub/sub consumers with `pubsubtest.Subscribe`.
- Exercise races with `testcase.Race(fn1, fn2, …)` and always run `go test -race`.
- Follow `testcase-spec` and `testcase-assert`.

## Changing `pkg/synckit` itself

- Keep the zero value ready: initialise lazily with an internal `sync.Once`. Add `var _ Iface = (*T)(nil)` assertions. Every new type needs an `Example*` test, a spec in `synckit_test.go` or `exchange_test.go`, a `testcase.Race` test, and a `README.md` section.
- Context-holding primitives need a leak guard test, modelled on `TestFan_leakGuard` and `TestGroup_leakGuard`.
- Run `go test -race ./pkg/synckit/...`, then the dependants: `go test ./pkg/... ./port/... ./adapter/memory/...`. `adapter/postgresql` and `adapter/hashicorp/vault` are separate modules; test them from their own directories.
- Known documentation drift and quirks are listed in [references/pitfalls.md](references/pitfalls.md#known-quirks-in-pkgsynckit). Fix them with tests rather than building on them.

## Reference

- [references/api.md](references/api.md): every exported identifier, its signature and its exact semantics
- [references/pitfalls.md](references/pitfalls.md): verified deadlocks, surprises and documentation drift, each with a minimal reproduction
- [references/patterns.md](references/patterns.md): worked recipes (scatter-gather, bounded worker pool, Fan worker queue, fan-in, per-key lazy clients, background job lifecycle, notifications, close-once)
- [references/testing.md](references/testing.md): testing code that uses synckit, and testing synckit itself
- [references/in-repo-usage.md](references/in-repo-usage.md): where frameless itself uses each primitive, to read as examples
- `pkg/synckit/README.md`: package overview
- `pkg/synckit/synckit.go`: `Map`, `Slice`, `Job`, `Go`, `Group`, `Phaser`, `Limiter`, `RWLockerFactory`, `IsDone`
- `pkg/synckit/init.go`: `Init`, `InitErr`, `InitErrWL`
- `pkg/synckit/exchange.go`: `Fan`, `Broadcast`
- `pkg/synckit/*_test.go`: `Example*` functions and specs that pin the behaviour

## Checklist

- [ ] No hand-written `WaitGroup` plus error channels, semaphore channels, double-checked locking, mutex-wrapped maps, keyed mutex maps, `sync.Cond`, or bare `go` statements that can't be waited on.
- [ ] synckit values are struct fields or `var`s, never copied, and methods on their holders use pointer receivers. `go vet` passes.
- [ ] `Init` with a mutex never relies on a zero result being cached. Errors that should be retried use `InitErr`.
- [ ] Every `Group` is `Wait`ed (which surfaces panics). The fail-fast vs `Isolation` choice is deliberate. Errors are matched with `errors.Is`.
- [ ] Every `synckit.Go` job is kept and is `Cancel`led and `Wait`ed on shutdown. Its `ctx` is never nil.
- [ ] Every `Limiter.Lock` has a matching `Unlock` (`defer`), and shutdown calls `SetLimit(0)` if goroutines could stay blocked.
- [ ] No `Map`/`Slice` method is called from inside `Do`, `GetOrInit`'s `init`, a `Borrow` window, or an `All`/`RAll`/`Values` loop.
- [ ] Every `Fan`/`Broadcast` message is ACKed. Publishers can't block forever (ctx, `Close`, `Cancel`). Broadcast subscribers are fast, or they ACK first and forward to a buffer.
- [ ] Every `Phaser` wait loops on a guarded condition, or the phaser is `Finish`ed.
- [ ] Tests use `Phaser` gates, `Within`/`NotWithin`, `Eventually` and `-race`, with no `time.Sleep`.
