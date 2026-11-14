# synckit pitfalls

Each entry gives the symptom, a minimal reproduction and the fix. Unless an entry says it was read from the code, its behaviour was observed by running the reproduction against `pkg/synckit`.

Contents:

- [General](#general)
- [Init and InitErr](#init-and-initerr)
- [Go and Group](#go-and-group)
- [Limiter](#limiter)
- [Phaser](#phaser)
- [RWLockerFactory](#rwlockerfactory)
- [Map and Slice](#map-and-slice)
- [Fan and Broadcast](#fan-and-broadcast)
- [IsDone](#isdone)
- [Known quirks in pkg/synckit](#known-quirks-in-pkgsynckit)

---

## General

### Copying a synckit value

Every synckit type holds a `sync` lock or `sync.Once` internally. A copy made after first use shares nothing with the original, or worse, shares half of it.

```go
type Indexer struct {
	limiter synckit.LimiterWithCeiling[synckit.LimitToNumCPU]
}

func (ix Indexer) Do() { // value receiver: every call locks its own copy, so nothing is limited
	ix.limiter.Lock()
	defer ix.limiter.Unlock()
}
```

`go vet` (copylocks) reports this. It was confirmed for `Group` and `LimiterWithCeiling`.

**Fix:** use pointer receivers on every type that holds a synckit value, and pass the holder by pointer. Never return a synckit value from a function or store it in a slice of values that gets appended to.

---

## Init and InitErr

### A zero result is never cached with a mutex

```go
var (
	mu    sync.Mutex
	n     int
	calls int
)
synckit.Init(&mu, &n, func() int { calls++; return 0 })
synckit.Init(&mu, &n, func() int { calls++; return 0 })
// calls == 2: with a mutex, "initialised" means n != 0
```

**Fix:** if zero is a legitimate result, use a `*sync.Once`, or store a pointer (`*int`, `*Config`) so the initialised value is never zero.

### `*sync.Once` caches the first result forever

```go
var (
	once   sync.Once
	client *api.Client
)
c := synckit.Init(&once, &client, func() *api.Client {
	c, err := api.Dial()
	if err != nil {
		return nil // nil is cached; every later call gets nil
	}
	return c
})
```

**Fix:** when initialisation can fail and should be retried, use `InitErr` with a `*sync.Mutex` or `*sync.RWMutex`. `InitErr` stores nothing on error, so the next call retries. Keep `Once` for "do it once and remember the outcome", such as close-once (see [patterns](patterns.md#close-once-with-a-memoised-error)).

### Nested `Init` on the same lock deadlocks

```go
func (c *Client) api() *API {
	return synckit.Init(&c.mu, &c.apiV, func() *API {
		return &API{Auth: c.auth()} // c.auth() also calls synckit.Init(&c.mu, …): deadlock
	})
}
```

`init` runs while the lock is held, and none of the locks is re-entrant.

**Fix:** one lock (or one `Once`) per lazily initialised field. If initialisers depend on each other, give each its own lock.

### Reading the field directly

```go
func (c *Client) Close() error {
	if c.db != nil { // races with InitErr writing c.db
		return c.db.Close()
	}
	return nil
}
```

**Fix:** read through the accessor, or take the same lock (`c.mu.RLock()` when the lock is an `RWMutex`).

---

## Go and Group

### `synckit.Go` with a nil context

```go
synckit.Go(nil, fn) // panics: context.WithCancel(nil)
```

`Group.Go(nil, fn)` is fine; it falls back to `context.Background()`.

**Fix:** always pass a context. For a job that must outlive the caller's request, pass `context.WithoutCancel(ctx)` (or `contextkit.WithoutCancel`) and keep the `Job` so you can cancel it yourself.

### Panics: crashed in `Go`, hidden in `Group` until `Wait`

- `synckit.Go` doesn't recover panics. A panic in `fn` crashes the process, like a bare `go` statement.
- `Group.Go` recovers the panic and stores it. Only `Group.Wait` re-raises it. `Job.Wait()` of the panicked task returns `nil`.

```go
var g synckit.Group
job := g.Go(ctx, func(context.Context) error { panic("boom") })
_ = job.Wait() // nil
g.Wait()       // panics with "boom", in this goroutine
```

Read from the code: a group that is never `Wait`ed loses its panics and errors silently.

**Fix:** always `Wait` the group, in the goroutine that owns it. Use a `Group` rather than `synckit.Go` when the task may panic and the process must survive.

### Matching errors with `==`

```go
var g synckit.Group
g.Go(ctx, func(context.Context) error { return errBoom })
g.Go(ctx, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })

err := g.Wait()
// err.Error() == "boom\ncontext canceled"
// err == errBoom                   -> false
// errors.Is(err, errBoom)          -> true
// errors.Is(err, context.Canceled) -> true
```

In fail-fast mode the siblings' `context.Canceled` errors are collected too.

**Fix:** match with `errors.Is` or `errors.As`, never with `==`.

### Filtering `context.Canceled` can hide a real failure

`errors.Is(err, context.Canceled)` is also true for a merged error that contains a real failure *and* a cancellation. Code like this turns real failures into success:

```go
g.Cancel()
if err := g.Wait(); err != nil && !errors.Is(err, context.Canceled) {
	return err
}
return nil
```

**Fix:** have long-running tasks return `nil` when their context ends during a normal shutdown, as `adapter/postgresql/guard.go`'s keepAlive does. Then `g.Cancel(); return g.Wait()` returns only real errors.

### `Cancel` isn't sticky

```go
var g synckit.Group
g.Cancel()
g.Go(ctx, fn) // fn runs normally: its context is not cancelled
```

Fail-fast behaves the same way: a failing task cancels the tasks that are running *at that moment*, not the ones that a submit loop starts later. The parent `ctx` isn't cancelled either, so the submitter doesn't notice.

**Fix:** derive `ctx, cancel := context.WithCancel(ctx)`, call `cancel()` from the failing task, and stop submitting when `ctx.Err() != nil`. See [patterns](patterns.md#bounded-worker-pool).

### Cancelling one task's parent context

```go
ctx1, cancel1 := context.WithCancel(ctx)
g.Go(ctx1, waitForCtx) // returns ctx.Err()
g.Go(ctx, waitForCtx)
cancel1()
```

Only the first task is cancelled; the second keeps running. Its `context.Canceled` is still collected, so `g.Wait()` returns an error that matches `context.Canceled`.

**Fix:** use `g.Cancel()` for a group-wide stop. Give tasks per-task contexts only when you mean to cancel them one at a time.

### Changing `Isolation` or `ErrorOnGoexit` while tasks run

Read from the code: running task goroutines read both fields without a lock, so writing them after the first `Go` is a data race.

**Fix:** set them in the composite literal (`synckit.Group{Isolation: true}`) or before the first `Go`.

### More than one goroutine calling `Wait`

`Wait` drains the collected errors and the panic. A second `Wait` that returns afterwards gets `nil`.

```go
err1 := g.Wait() // "boom"
err2 := g.Wait() // nil
```

**Fix:** let exactly one owner call `Wait`. Other goroutines that only need to know that the group is idle should use `<-g.Done()`.

### Waiting on an empty group

`Done()` of an empty group is already closed, and `Wait` returns immediately. A watcher started before the first `Go` sees an idle group and returns at once.

**Fix:** add the tasks first, then wait. If tasks are added over time, keep the owner's `Wait` after the submit loop.

### `ErrorOnGoexit` is off by default

A task that ends through `runtime.Goexit` (for example `t.FailNow()` inside a task) is invisible to `Wait` unless `ErrorOnGoexit: true`.

**Fix:** set `ErrorOnGoexit: true` in test helpers that run assertions inside tasks, and match `synckit.ErrGoexit` with `errors.Is`.

---

## Limiter

### Slot leaks

```go
l.Lock()
if err := validate(x); err != nil {
	return err // the slot is never released; after N such returns every Lock blocks
}
defer l.Unlock()
```

**Fix:** `defer l.Unlock()` right after `Lock`. In the semaphore pattern (`Lock` in the loop, `Unlock` in the task), the task's first statement is `defer l.Unlock()`.

### `Unlock` without a held slot

```go
defer l.Unlock()
if !l.TryLock() {
	return errBusy // the deferred Unlock panics: "synckit.Limiter: Unlock without Lock"
}
```

**Fix:** `defer l.Unlock()` only after `Lock` or a successful `TryLock`.

### A negative limit that is never raised

```go
var l synckit.Limiter
l.SetLimit(-1)
l.Lock() // blocks forever; TryLock returns false
```

`ExampleLimiter_changingLimitDynamically` in `pkg/synckit/synckit_test.go` does exactly this. It would deadlock if it ran; see [known quirks](#known-quirks-in-pkgsynckit).

**Fix:** every `SetLimit(-1)` (pause) needs a code path that sets a non-negative limit (resume or shutdown).

### `Lock` ignores contexts

Goroutines blocked in `Lock` stay blocked after their context is cancelled.

**Fix:** when the limiter's owner shuts down, lift the limit, and have goroutines check the context once they get in:

```go
// once, where the limiter's lifetime is managed (not per call: that would lift the cap for everyone)
context.AfterFunc(shutdownCtx, func() { l.SetLimit(0) }) // unlimited: every blocked Lock gets in

// in each worker
l.Lock()
defer l.Unlock()
if err := ctx.Err(); err != nil {
	return err
}
```

### Lowering the limit doesn't preempt

`SetLimit(2)` while five holders are inside leaves all five inside. New holders get in only once fewer than two are held.

**Fix:** if work must stop, cancel it through its context. The limit only gates new entries.

### `LimiterWithCeiling.Limit()` is 0 before first use

Read from the code: the ceiling is applied inside `Lock` and `TryLock`, so `Limit()` returns 0 (unlimited) until one of them has run.

**Fix:** to size buffers or worker counts up front, call the ceiling directly, for example `synckit.LimitToNumCPU{}.Limit()`.

### `Limit()` on a custom ceiling runs on every acquisition

The ceiling's `Limit()` is called on every `Lock` and `TryLock`, not once. A ceiling that reads an environment variable or a config file does that on every acquisition.

**Fix:** keep ceilings cheap. Cache expensive lookups in a package-level `sync.OnceValue`.

---

## Phaser

### Lost wake-ups

```go
var p synckit.Phaser
p.Broadcast()  // nobody is waiting: lost
go p.Wait()    // blocks until the next Broadcast/Signal/Finish
```

`Signal` and `Broadcast` aren't latched.

**Fix:** pick the right shape:

- A one-shot event ("started", "closed"): use `Finish()`, which is latched.
- A state change: wait in a loop on a condition guarded by a mutex, with `Wait(&mu)`.
- Tests: wait for `p.Len() == n` before you `Broadcast`.

### `Finish` is permanent, and makes cond loops spin

After `Finish`, every `Wait` returns immediately, without releasing the lockers. A loop whose condition doesn't account for the finished state spins forever while holding the lock:

```go
// Close() set nothing, only called b.ready.Finish()
for len(b.items) == 0 {
	b.ready.Wait(&b.mu) // returns at once, forever: a hot loop holding b.mu
}
```

**Fix:** keep a guarded `closed` flag, set it before `Finish`, and include it in the loop condition (`for len(b.items) == 0 && !b.closed`). Don't `Finish` a phaser you still want to use as a per-round barrier; there is no reset.

### `Wait(&mu)` without holding `mu`

```go
var mu sync.Mutex
p.Wait(&mu) // fatal error: sync: unlock of unlocked mutex (not recoverable)
```

**Fix:** lock the mutex before `Wait`. The lockers follow `sync.Cond` rules: held on entry, released while parked, held again on return.

### `Phaser` is not a `synckit.Waiter`

`Waiter` is `interface{ Wait() }`. `Phaser.Wait` is variadic (`Wait(...sync.Locker)`), so `*Phaser` doesn't implement it. Wrap it if you need one: `waiterFunc(func() { p.Wait() })`.

---

## RWLockerFactory

### Sharing one handle, or taking two read locks on it

```go
l := locks.RWLocker("user:42")
l.RLock()
l.RLock()
l.RUnlock()
l.RUnlock() // panics: "synckit: unlock of unlocked mutex"
```

A handle remembers one current `Lock` and one current `RLock`.

**Fix:** call `locks.RWLocker(key)` for each critical section, in the goroutine that uses it. Never store a handle in a shared field.

### The same key twice in one goroutine

```go
func (w *Wallets) Transfer(from, to WalletID, amount int) error {
	a := w.locks.RWLocker(from)
	a.Lock()
	defer a.Unlock()
	b := w.locks.RWLocker(to)
	b.Lock() // deadlock when from == to
	defer b.Unlock()
	// …
}
```

Per-key locks aren't re-entrant. Two goroutines that lock the same two keys in opposite orders also deadlock.

**Fix:** handle `from == to` separately, and lock several keys in a sorted order. See [patterns](patterns.md#locking-several-keys).

### `ReadOptimised` delays writers on other keys

With `ReadOptimised: true`, an `RLock` on a key that has no active entry takes the factory-wide read lock. While such a read is held, a writer that needs a new entry waits, even for a different key.

**Fix:** use `ReadOptimised` only when reads are short and far outnumber writes, as in `pkg/zerokit`'s initialisation locks.

### Process-local only

The factory doesn't coordinate across processes or replicas.

**Fix:** use `port/guard` (`guard.LockerFactory`) with a shared supplier such as `adapter/postgresql`. See `frameless-ports`.

---

## Map and Slice

### Calling the same `Map` from inside a callback

```go
m.GetOrInit("a", func() int {
	return m.Get("b") + 1 // deadlock: init runs under the write lock
})

_ = m.Do(func(vs map[string]int) error {
	m.Set("x", 1) // deadlock
	return nil
})

v, release, _ := m.Borrow("a")
_ = m.Get("b") // deadlock: Borrow holds the write lock until release()
release()
```

**Fix:** inside `Do`, work on the map that `fn` receives (`vs["x"] = 1`). Compute dependent values before `GetOrInit`. Inside a `Borrow` window, touch only `*v`.

### `Borrow` locks the whole map

`Borrow` and `BorrowWithInit` take the map-wide write lock, not a per-key lock. Every other key is blocked until `release()`. There is a TODO about this in `pkg/synckit/synckit.go`.

Read from the code: `release` unlocks the map's `sync.RWMutex` each time it is called. A missing `release` blocks the map forever. A second call is a fatal `sync: Unlock of unlocked RWMutex`.

**Fix:** keep Borrow windows tiny, and call `release` exactly once (`defer release()`). For an atomic read-modify-write, prefer `Do`.

### Mutating inside an `All` or `RAll` loop

```go
for k := range m.All() {
	m.Set(k+"!", 1) // deadlock: All holds the write lock while yielding
}

for k := range m.RAll() {
	_ = m.Get(k) // works, but it is a recursive RLock: it deadlocks if a writer queues up in between
}
```

`Keys()` and `Values()` are snapshots, so mutating inside them is fine:

```go
for k := range m.Keys() {
	m.Set(k+"!", 1) // fine
}
```

**Fix:** iterate `Keys()`, `Values()` or `ToMap()` when the body touches the map.

### `Slice.Values` and `RValues` hold the lock for the whole loop

```go
for v := range s.Values() {
	s.Append(v) // deadlock
}
```

A slow loop body also blocks every writer for the whole iteration.

**Fix:** iterate `s.ToSlice()`, which is a copy.

### A slow `GetOrInit` blocks the whole map

`init` runs under the map's write lock. A network dial inside it stalls every reader and writer of every key.

**Fix:** store a cheap holder and initialise the slow part with `InitErr` on the holder's own lock. See [patterns](patterns.md#per-key-lazy-clients).

### `Map` never evicts

A `synckit.Map` keyed by request ID, session ID or user ID grows for the life of the process.

**Fix:** `Delete` entries when their owner finishes, or use a bounded key space. The package-level `Map`s in frameless are keyed by `reflect.Type` and similar finite keys.

### Mutating a copied value

```go
var m synckit.Map[string, Counter]
c := m.Get("a")
c.N++ // changes the copy only
```

**Fix:** use `m.Do(func(vs map[string]Counter) error { c := vs["a"]; c.N++; vs["a"] = c; return nil })`, a `Borrow` window, or `Set`. Use `Lookup` instead of `Get` when you need to tell "missing" from "zero".

---

## Fan and Broadcast

### Forgetting to ACK

```go
for msg, err := range f.Subscribe(ctx) {
	if err != nil {
		return err
	}
	handle(msg.Data()) // no ACK: auto-NACK at the end of the iteration
}
// observed deliveries: [7 7 7 …], the same message forever
```

**Fix:** ACK every handled message inside the loop body. NACK only failures that are worth retrying, and bound those retries yourself.

### ACKing from another goroutine

```go
for msg, err := range f.Subscribe(ctx) {
	if err != nil {
		return err
	}
	go func() {
		handle(msg.Data())
		_ = msg.ACK() // too late: a no-op, the message was already redelivered
	}()
}
```

ACK and NACK each go through a `sync.Once`, so the first one wins. When the iteration ends, the message is NACKed automatically, so the later ACK does nothing. The message context is cancelled at the same time.

**Fix:** process and ACK inside the iteration. If you must hand off, ACK first and accept at-most-once delivery, as in [the notifications pattern](patterns.md#notifications-with-broadcast).

### Keeping `msg.Context()` for background work

The message context is cancelled when its loop iteration ends. Work that outlives the iteration sees a cancelled context.

**Fix:** finish the work inside the iteration, or detach with `context.WithoutCancel(msg.Context())` and own the lifecycle yourself.

### NACK after the message context is done

Read from the code: if the message context is already done (for example after `Cancel()`), `NACK` settles the message and returns the context error instead of redelivering. The message is gone.

**Fix:** don't count on redelivery during `Cancel`. Use `Close` for a graceful drain.

### `Fan.Publish` blocks without a subscriber

```go
var f synckit.Fan[int]
_ = f.Publish(context.Background(), 1) // blocks forever: nobody subscribes
```

**Fix:** start subscribers before publishing. Give producers a context that ends on shutdown, or call `Close()`/`Cancel()` on shutdown so blocked publishers return `context.Canceled`.

### Treating a nil `Publish` error as "processed"

`Fan.Publish` returns when a subscriber *takes* the message, before the handler runs and before the ACK.

**Fix:** if the publisher must know that processing finished, close the `Fan` and `Wait` the consumer `Group`, or collect results with a `synckit.Slice`.

### `Fan` is unicast

Ten publishes produce ten deliveries in total across all subscribers, not ten per subscriber.

**Fix:** use `Broadcast` when every subscriber must see every message.

### `Broadcast` drops messages that nobody hears

`Broadcast.Publish` with no subscribers returns `nil`, and the message is gone. A subscriber that starts later misses it.

**Fix:** treat `Broadcast` as volatile notifications. In tests, wait for `bc.Len() == n` before publishing. For durable delivery, use `port/pubsub` with a durable supplier.

### A slow `Broadcast` subscriber stalls every publisher

`Broadcast.Publish` waits for each current subscriber to take its copy. One busy subscriber blocks the next `Publish` for everyone.

**Fix:** ACK immediately and forward to a buffered channel with a drop or timeout policy. See [patterns](patterns.md#notifications-with-broadcast).

### `Close` and late `Publish` or `Subscribe` calls

- After `Close`, `Publish` returns `context.Canceled`. A publish that races with a still-draining subscriber may still be delivered, so never rely on `Close` to reject a specific message.
- `Subscribe` after `Close` ends immediately. A retry loop that re-subscribes must stop once the exchange is closed, or it spins.
- `Close` doesn't cancel in-flight message contexts; `Cancel` does.

### Waiting for an error to detect the end of a subscription

Read from the code: `Fan` and `Broadcast` subscriptions never yield a non-nil error. They end silently when the subscription context ends or after `Close`/`Cancel`.

**Fix:** treat the end of the `range` loop as the end. Keep the `if err != nil` branch anyway, so the consumer also works with other `pubsub.Subscriber` suppliers.

---

## IsDone

### Using `IsDone` on a data channel

```go
ch := make(chan struct{}, 1)
ch <- struct{}{}
synckit.IsDone(ch) // panics: "implementation-error"
```

On an open data channel with nothing ready it returns `false`, and it consumes a value when one is ready. Neither result is meaningful.

**Fix:** use `IsDone` only on close-only channels: `ctx.Done()`, `job.Done()`, `g.Done()`, or your own `chan struct{}` that is only ever closed.

---

## Known quirks in pkg/synckit

These are documentation drift and rough edges found in `pkg/synckit` itself. None of them is fixed. Don't copy the broken snippets, and if you change `pkg/synckit`, fix them, with tests where behaviour is involved.

| Where | Quirk |
| --- | --- |
| `README.md`, Go & Group | `<-job.Done() // locks until the job finishes` should say "blocks". |
| `README.md`, Go & Group | `job := g.Go(ctx, fetchC)` declares `job` and never uses it. Pasted as is, it doesn't compile. |
| `README.md`, Limiter | `func (i MyType) Do()` is a value receiver on a struct with a `LimiterWithCeiling`. `go vet` flags it, and every call locks a copy, so nothing is limited. Use `func (i *MyType) Do()`. |
| `README.md`, Phaser | `b.Signal()` should be `p.Signal()`. The snippet also races: the goroutine may not be parked yet when `Signal` runs, so the wake-up can be lost. |
| `README.md` | `Map`, `Slice`, `Fan`, `Broadcast`, `InitErrWL` and `ErrGoexit` aren't documented. |
| `synckit.go`, `Limiter` doc | The doc shows `synckit.Limiter[synckit.LimitByNumCPU]` and `synckit.Limiter[synckit.LimitAtRuntime]{Ceiling: 10}`. `Limiter` isn't generic, and `LimitByNumCPU`/`LimitAtRuntime` don't exist. The real API is `LimiterWithCeiling[LimitToNumCPU]` and `Limiter.SetLimit`. |
| `synckit.go`, `LimitCeiling` doc | The comment starts with `limiterLimit` instead of `LimitCeiling`, mentions the same non-existent ceilings, and says `Limit()` "is consulted once, on the Limiter's first use". The code calls it on every `Lock`/`TryLock`. It also says the value "must be greater than zero"; the code passes it to `SetLimit`, so 0 means unlimited and a negative value blocks. |
| `synckit.go`, `SetLimit` doc | The last sentence ends mid-way ("to something that can"). |
| `synckit.go`, `Phaser` doc | "I that combines the behavior of …" is a typo. |
| `synckit.go`, `Map.Borrow` | Locks the whole map. A `TODO` notes that it should lock only the affected key. |
| `synckit_test.go`, `MyTypeLimiterExample` | The comment says "at most runtime.NumCPU()", but the code calls `SetLimit(4)`. |
| `synckit_test.go`, `ExampleLimiter_changingLimitDynamically` | Sets the limit to `-1` and never raises it, so it would deadlock if it ran. |
| `exchange_test.go`, `ExampleFan_in` | Never closes the `Fan`, so the consumer loop never ends and the example would hang if it ran. See [patterns](patterns.md#fan-in-to-a-serial-consumer) for a working version. |
| All `Example*` functions in `pkg/synckit` | None has an `// Output:` comment, so `go test` compiles them but never runs them. That is why the two broken examples above go unnoticed. |
| `pkg/synckit/TODO.md` | Records a flaky `TestRWLockerFactory` case with seed `TESTCASE_SEED=2569458535983166363`. |
| `pkg/workflow/spawn_test.go:1279` | Defers `stage1.Finish()` twice and never defers `stage2.Finish()`. The happy path calls `stage2.Finish()` explicitly, so it only leaks when the test fails early. `t.Cleanup(p.Finish)` right after each declaration avoids this. |
| `go.llib.dev/testcase` | testcase vendors its own copy (`internal/synckit`, `tcsync.Phaser`). Fixes in frameless don't reach it, and the reverse is also true. |
