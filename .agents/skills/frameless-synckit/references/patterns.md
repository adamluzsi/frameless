# synckit patterns

Worked recipes that combine synckit primitives correctly. Each one says when to use it, gives the code, and notes the traps that the code avoids. "Seen in" points to the frameless code that uses the same shape; see [in-repo-usage.md](in-repo-usage.md).

Contents:

- [Scatter-gather](#scatter-gather)
- [Best-effort per-item work](#best-effort-per-item-work)
- [Bounded worker pool](#bounded-worker-pool)
- [Fan worker queue](#fan-worker-queue)
- [Fan-in to a serial consumer](#fan-in-to-a-serial-consumer)
- [Per-key lazy clients](#per-key-lazy-clients)
- [Locking several keys](#locking-several-keys)
- [Background job lifecycle](#background-job-lifecycle)
- [Producer feeding a background job](#producer-feeding-a-background-job)
- [Notifications with Broadcast](#notifications-with-broadcast)
- [Close once with a memoised error](#close-once-with-a-memoised-error)
- [Cond-style buffer with Phaser](#cond-style-buffer-with-phaser)
- [Pausing, resuming and draining a Limiter](#pausing-resuming-and-draining-a-limiter)

---

## Scatter-gather

**Use when** a call needs several independent results and should fail as soon as one of them fails.

```go
func FetchAll(ctx context.Context, ids []ID) ([]Item, error) {
	var (
		g     synckit.Group
		items = make([]Item, len(ids)) // each task writes only its own index, so no lock is needed
	)
	for i, id := range ids {
		g.Go(ctx, func(ctx context.Context) (err error) {
			items[i], err = fetch(ctx, id)
			return err
		})
	}
	if err := g.Wait(); err != nil { // Wait happens after every task has returned, so reading items is safe
		return nil, err
	}
	return items, nil
}
```

- The default group fails fast: the first error cancels the running siblings, and `Wait` returns every error merged. Match it with `errors.Is`.
- Writes to distinct slice indexes don't need a lock. When results are keyed, or a task can produce a variable number of them, collect them in a `synckit.Map` or `synckit.Slice`.
- This starts one goroutine per input. For large or unbounded inputs, use the [bounded worker pool](#bounded-worker-pool).

Seen in: `pkg/jsonkit/jsontoken/jsontoken.go` (`Scanner.open` runs every matching `On` handler in a `Group`, each on its own lockstep reader), and the `Dashboard` example in `SKILL.md`.

---

## Best-effort per-item work

**Use when** items are independent and one failure must not stop the others, for example sending notifications.

```go
func NotifyAll(ctx context.Context, users []User) error {
	g := synckit.Group{Isolation: true} // set before the first Go
	for _, u := range users {
		g.Go(ctx, func(ctx context.Context) error {
			if err := notify(ctx, u); err != nil {
				return fmt.Errorf("notify %s: %w", u.ID, err)
			}
			return nil
		})
	}
	return g.Wait() // every failure, merged; successful items are not affected
}
```

- With `Isolation`, an error doesn't cancel siblings. `Wait` still collects every error.
- Wrap each error with the item's identity, because the merged error lists them one per line.

Seen in: `pkg/workflow/schedule.go` (`Runtime.Run`), `pkg/cache/cache.go` (background refreshes).

---

## Bounded worker pool

**Use when** you process many items concurrently, but at most N at a time, and the batch should stop at the first failure.

```go
func ProcessAll(ctx context.Context, items []Item) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		g       synckit.Group
		limiter synckit.Limiter
	)
	limiter.SetLimit(8)

	for _, item := range items {
		limiter.Lock() // back-pressure: at most 8 goroutines exist at once
		if ctx.Err() != nil { // a task failed, or the caller gave up: stop submitting
			limiter.Unlock()
			break
		}
		g.Go(ctx, func(ctx context.Context) error {
			defer limiter.Unlock()
			if err := process(ctx, item); err != nil {
				cancel() // fail-fast only reaches running tasks; this also stops the loop above
				return err
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return ctx.Err() // non-nil only if the caller gave up before every item was submitted
}
```

Why it is shaped like this:

- `Group.Cancel` and fail-fast cancel only the tasks that are running *now*. They aren't sticky, and they don't cancel the parent `ctx`. Without the derived `ctx` and the explicit `cancel()`, the loop keeps submitting new, healthy tasks after a failure.
- Calling `Lock` in the loop, rather than inside the task, means no more than 8 goroutines ever exist. With `Lock` inside the task, all `len(items)` goroutines start at once and queue on the limiter.
- `Unlock` is the task's first deferred call, so every exit path releases the slot.

For a cap that is shared by every caller of a type, make the limiter a field. `LimiterWithCeiling[synckit.LimitToNumCPU]` gives a zero value that is already configured; see the `Indexer` example in `SKILL.md`.

---

## Fan worker queue

**Use when** a fixed set of long-lived workers takes work from a stream that arrives over time, or each worker owns per-worker state, such as a connection.

```go
func Run(ctx context.Context, tasks iter.Seq[Task], workers int) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		queue synckit.Fan[Task]
		g     synckit.Group
	)
	for range workers {
		g.Go(ctx, func(ctx context.Context) error {
			for msg, err := range queue.Subscribe(ctx) {
				if err != nil {
					return err
				}
				if err := handle(msg.Context(), msg.Data()); err != nil {
					_ = msg.ACK() // settle it: the run fails instead of redelivering forever
					cancel()      // unblock the publisher below; the Group only cancels sibling tasks
					return err
				}
				_ = msg.ACK()
			}
			return nil
		})
	}

	for task := range tasks {
		if err := queue.Publish(ctx, task); err != nil {
			break // a worker failed, or the caller gave up
		}
	}
	_ = queue.Close() // reject new publishes; subscriptions drain, then end
	if err := g.Wait(); err != nil {
		return err
	}
	return ctx.Err()
}
```

- `Publish` blocks until a worker takes the message, which gives natural back-pressure. Without `cancel()`, a publisher whose workers have all failed would block until its context ends.
- Every message is ACKed inside its iteration. An un-ACKed message is redelivered at the end of the iteration.
- **Never `Publish` to a `Fan` from its own subscribers.** `Publish` waits for an idle subscriber, so if every worker publishes at once, nobody is left to receive.
- Retry transient failures inside `handle` with `pkg/resilience` (see `frameless-resilience`), not with NACK loops. A `Fan` message carries no attempt count.

Seen in: `ExampleFan_out` in `pkg/synckit/exchange_test.go`.

---

## Fan-in to a serial consumer

**Use when** many producers feed one consumer that must process items one at a time, for example a single writer to a file, a socket or a CSV export.

```go
func Export(ctx context.Context, sources []Source, w *csv.Writer) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		rows synckit.Fan[[]string]
		g    synckit.Group
	)
	for _, src := range sources {
		g.Go(ctx, func(ctx context.Context) error {
			for row, err := range src.Rows(ctx) {
				if err != nil {
					return err
				}
				if err := rows.Publish(ctx, row); err != nil {
					return err
				}
			}
			return nil
		})
	}
	producers := synckit.Go(ctx, func(context.Context) error {
		defer rows.Close() // ends the consumer loop once every producer has returned
		return g.Wait()
	})

	for msg, err := range rows.Subscribe(ctx) {
		if err != nil {
			return err
		}
		if err := w.Write(msg.Data()); err != nil {
			_ = msg.ACK()
			cancel()             // producers blocked in Publish return
			_ = producers.Wait() // don't leave them running
			return err
		}
		_ = msg.ACK()
	}
	return producers.Wait() // the producers' error, if any, or nil
}
```

- `Close` after the producers finish is what ends the consumer's `range` loop. `ExampleFan_in` in `pkg/synckit` omits it and would hang; see [pitfalls](pitfalls.md#known-quirks-in-pkgsynckit).
- A producer failure fails fast: the group cancels the other producers, `g.Wait` returns, `Close` ends the loop, and `producers.Wait()` reports the error.
- If a producer panics, `g.Wait` re-panics inside the `synckit.Go` goroutine, which doesn't recover. That crashes the process, just like an unrecovered panic in a bare goroutine.
- Rows from one source keep their order. Rows from different sources interleave.
- If the consumer doesn't need to run concurrently with the producers, collecting into a `synckit.Slice` and processing after `g.Wait()` is simpler.

---

## Per-key lazy clients

**Use when** you need one expensive object per key (a client per tenant, a connection per shard), created on first use, with errors retried and no global lock held during the slow part.

```go
type tenantClient struct {
	mu     sync.RWMutex
	client *billing.Client
}

type Clients struct {
	Config Config

	tenants synckit.Map[TenantID, *tenantClient] // pointer values: the holder's mutex is never copied
}

func (c *Clients) For(ctx context.Context, id TenantID) (*billing.Client, error) {
	tc := c.tenants.GetOrInit(id, func() *tenantClient {
		return &tenantClient{} // cheap: only the holder is created under the map-wide lock
	})
	return synckit.InitErr(&tc.mu, &tc.client, func() (*billing.Client, error) {
		return billing.Dial(ctx, c.Config.For(id)) // slow: blocks only this tenant's callers; on error, the next call retries
	})
}
```

- `GetOrInit` runs `init` under the map's write lock, so a slow `init` would block every key. Keep it to an allocation.
- Values must be pointers. `Map.Get` returns a copy, and a copied `sync.RWMutex` doesn't protect anything.
- `InitErr` with an `RWMutex` gives a read-locked fast path once the client exists.
- The `Map` never evicts. `Delete` the key, and close its client, when the tenant goes away.

---

## Locking several keys

**Use when** an operation must hold the locks of several keys at once, for example a transfer between two accounts.

```go
type Wallets struct {
	locks synckit.RWLockerFactory[WalletID] // type WalletID string
}

func (w *Wallets) Transfer(ctx context.Context, from, to WalletID, amount int) error {
	if from == to {
		return ErrSameWallet // per-key locks aren't re-entrant: locking the same key twice deadlocks
	}
	keys := []WalletID{from, to}
	slices.Sort(keys) // every caller locks in the same order, so there is no lock-order deadlock
	for _, key := range keys {
		l := w.locks.RWLocker(key) // a fresh handle per key and per call
		l.Lock()
		defer l.Unlock()
	}
	// … both wallets are exclusively locked here
	return nil
}

func (w *Wallets) Balance(ctx context.Context, id WalletID) (int, error) {
	l := w.locks.RWLocker(id)
	l.RLock() // readers of the same key don't block each other
	defer l.RUnlock()
	// …
	return 0, nil
}
```

These locks are process-local. If several replicas write the same wallets, use `port/guard` with a shared supplier; see `frameless-ports`.

Seen in: `pkg/zerokit/zerokit.go` (`initLocks`, keyed by pointer address, with `ReadOptimised: true`).

---

## Background job lifecycle

**Use when** a component owns one long-running goroutine (a poller, a keep-alive, a watcher) that must stop cleanly.

```go
type Poller struct {
	Interval time.Duration

	job synckit.Job
}

func (p *Poller) Start(ctx context.Context) {
	p.job = synckit.Go(ctx, p.loop) // ctx must not be nil
}

func (p *Poller) Close() error {
	if p.job == nil {
		return nil
	}
	p.job.Cancel()
	return p.job.Wait() // returns the loop's error; the same error on every call
}

func (p *Poller) loop(ctx context.Context) error {
	ticker := clock.NewTicker(p.Interval) // go.llib.dev/testcase/clock, so tests can time travel
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil // a requested shutdown isn't an error
		case <-ticker.C:
			if err := p.poll(ctx); err != nil {
				return err
			}
		}
	}
}
```

- Pass a context that lives as long as the component. If `Start` is called from a request handler, detach with `context.WithoutCancel(ctx)` and rely on `Close` to stop the job.
- Return `nil` on `ctx.Done()`, so that `Close` reports only real failures.
- `synckit.Go` doesn't recover panics. If the loop can panic and the process must survive, run it in a `Group` and `Wait` the group in `Close`.
- For several background jobs, keep a `synckit.Group` field and implement `Close` as `g.Cancel(); return g.Wait()`.
- For application-level services, signal handling and scheduled repeats, use `pkg/tasker` rather than hand-wiring jobs in `main`.

Seen in: `adapter/postgresql/guard.go` (`lockContext.keepAlive`: `Cancel` then `Wait` before the lock record is deleted), `pkg/cache/cache.go` (`Close`).

---

## Producer feeding a background job

**Use when** callers push items to a background consumer goroutine through a channel, and must not block forever if that consumer has failed.

```go
type Batch struct {
	Context context.Context

	startOnce sync.Once
	closeOnce sync.Once
	closeErr  error

	input chan Row
	job   synckit.Job
}

func (b *Batch) start() {
	b.startOnce.Do(func() {
		b.input = make(chan Row)
		b.Context = cmp.Or(b.Context, context.Background())
		b.job = synckit.Go(b.Context, b.consume) // consume ranges over b.input until it is closed
	})
}

func (b *Batch) Add(row Row) error {
	b.start()
	select {
	case <-b.Context.Done():
		return b.Context.Err()
	case <-b.job.Done(): // the consumer failed or stopped: report its error instead of blocking
		return b.job.Wait()
	case b.input <- row:
		return nil
	}
}

func (b *Batch) Close() error {
	b.start()
	return synckit.Init(&b.closeOnce, &b.closeErr, func() error {
		close(b.input) // the consumer's range loop ends
		return b.job.Wait()
	})
}
```

`job.Done()` in the `select` is what keeps `Add` from hanging when the consumer has died. `Close` memoises its result; see the next pattern.

Seen in: `adapter/postgresql/Repository.go` (`Batch.Add` and `Batch.Close`).

---

## Notifications with Broadcast

**Use when** in-process components must hear about changes, where losing a notification is acceptable, and a slow listener must not stall the publisher.

```go
var changes synckit.Broadcast[Change]

// publisher: delivers to every current subscriber; a no-op that returns nil when nobody listens
_ = changes.Publish(ctx, change)

// subscriber: ACK at once, then forward to a buffer and drop when it is full
func listen(ctx context.Context, bc *synckit.Broadcast[Change]) (<-chan Change, synckit.Job) {
	out := make(chan Change, 64)
	job := synckit.Go(ctx, func(ctx context.Context) error {
		defer close(out)
		for msg, err := range bc.Subscribe(ctx) {
			if err != nil {
				return err
			}
			_ = msg.ACK() // notifications aren't redelivered: settle first
			select {
			case out <- msg.Data():
			default: // the reader is busy: drop, rather than stall every publisher
			}
		}
		return nil
	})
	return out, job
}
```

- `Broadcast.Publish` waits for every current subscriber to take its copy, so the subscriber loop must return to `range` quickly. Forwarding to a buffered channel decouples the two.
- Choose a drop policy deliberately. `pkg/workflow/schedule.go` waits up to a second (using a `clock` ticker) before it drops, instead of dropping immediately.
- Late subscribers miss earlier notifications. Treat notifications as hints ("something changed, go and re-read"), not as data.
- For durable delivery or delivery across processes, use `port/pubsub`; see `frameless-ports`.

Seen in: `pkg/workflow/schedule.go` (`listenToLocalChangeBroadcast` and `runListenToNotifications`).

---

## Close once with a memoised error

**Use when** `Close` (or `Stop`, or `Flush`) must run its body once, and every later call must return the same result.

```go
type Writer struct {
	closeOnce sync.Once
	closeErr  error
}

func (w *Writer) Close() error {
	return synckit.Init(&w.closeOnce, &w.closeErr, func() error {
		return w.flush()
	})
}
```

With a `*sync.Once`, `Init` caches the first result, even `nil` or an error. This is exactly what close-once needs, and exactly what lazy initialisation that should retry does *not* need; for that, use `InitErr` with a mutex.

Seen in: `adapter/memory/EventLogRepository.go` (`EventLogRepositoryBatch.Close`).

---

## Cond-style buffer with Phaser

**Use when** goroutines must wait for a state change that is guarded by a mutex: a `sync.Cond` use case, without `sync.NewCond`.

```go
type Buffer[T any] struct {
	mu     sync.Mutex
	ready  synckit.Phaser
	items  []T
	closed bool
}

func (b *Buffer[T]) Put(v T) bool {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return false
	}
	b.items = append(b.items, v)
	b.mu.Unlock()
	b.ready.Signal() // wake one taker
	return true
}

func (b *Buffer[T]) Take() (T, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(b.items) == 0 && !b.closed { // the closed flag stops the loop from spinning after Finish
		b.ready.Wait(&b.mu) // registers as a waiter, then releases b.mu; holds it again on return
	}
	if len(b.items) == 0 {
		var zero T
		return zero, false
	}
	v := b.items[0]
	b.items = b.items[1:]
	return v, true
}

func (b *Buffer[T]) Close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.ready.Finish() // latched: wakes every taker now, and every future Wait returns at once
}
```

Why no wake-up is lost: a taker registers as a waiter *before* `Wait` releases `b.mu`. `Put` needs `b.mu` to append, so its `Signal` always comes after the registration.

To make `Take` honour a context, wake the takers when the context ends. Take the mutex first, so the wake-up can't slip in between the context check and the registration:

```go
stop := context.AfterFunc(ctx, func() {
	b.mu.Lock() // waits until the taker has registered and released b.mu
	b.mu.Unlock()
	b.ready.Broadcast()
})
defer stop()
// … then, inside the wait loop, before b.ready.Wait(&b.mu):
// if err := ctx.Err(); err != nil { return zero, err }
```

Seen in: `pkg/iokit/iokit.go` (`lockstepSource.phaser`). It calls `phaser.Wait(&out.src.mutex)`, broadcasts whenever a reader advances, and uses `phaser.Len() == readersLen()-1` ("everyone else is already waiting") to avoid a deadlock.

---

## Pausing, resuming and draining a Limiter

**Use when** the concurrency of a worker set must change at runtime: start paused, scale up and down, or let everyone out on shutdown.

```go
type Workers struct {
	limiter synckit.Limiter // zero value: unlimited
}

func (w *Workers) Pause()       { w.limiter.SetLimit(-1) } // new Lock calls wait; current holders finish
func (w *Workers) Resume(n int) { w.limiter.SetLimit(n) }  // raising the limit wakes waiters

// Drain is called once on shutdown, after the workers' context has been cancelled.
// Lock ignores contexts, so goroutines blocked in Lock stay blocked until the limit lets them in.
func (w *Workers) Drain() { w.limiter.SetLimit(0) }

func (w *Workers) Do(ctx context.Context, work func(context.Context) error) error {
	w.limiter.Lock()
	defer w.limiter.Unlock()
	if err := ctx.Err(); err != nil { // let in by Drain: return at once
		return err
	}
	return work(ctx)
}
```

Wire the drain to the lifetime of the workers, not to a single call. A per-call `context.AfterFunc(ctx, w.Drain)` would lift the cap for everyone as soon as one caller gave up.

```go
ctx, cancel := context.WithCancel(appCtx) // the workers' lifetime
defer cancel()
context.AfterFunc(ctx, workers.Drain) // runs once ctx is cancelled, so everyone it lets in sees ctx.Err()
```

- `SetLimit(-1)` before you start the workers, then `SetLimit(n)` once they are all running, gives a start gate.
- Lowering the limit never preempts holders; it only gates new entries.
- Every `SetLimit(-1)` needs a path that sets the limit back. Otherwise every `Lock` blocks forever.
- `Limiter` caps concurrency, not rate. For calls per period, use `pkg/resilience`.
