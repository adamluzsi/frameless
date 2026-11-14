# synckit API catalogue

The complete exported surface of `go.llib.dev/frameless/pkg/synckit`, with the exact semantics of each identifier. The behaviour described here was verified against the implementation (`pkg/synckit/*.go`) and its specs. Where the doc comments in the source disagree with the code, this file follows the code, and [pitfalls.md](pitfalls.md#known-quirks-in-pkgsynckit) lists the drift.

Contents:

- [Interfaces](#interfaces)
- [Lazy initialisation](#lazy-initialisation)
- [Job and Go](#job-and-go)
- [Group](#group)
- [Limiter, LimiterWithCeiling, LimitCeiling, LimitToNumCPU](#limiter)
- [Phaser](#phaser)
- [RWLockerFactory](#rwlockerfactory)
- [Map](#map)
- [Slice](#slice)
- [Fan and Broadcast](#fan-and-broadcast)
- [IsDone](#isdone)
- [ErrGoexit](#errgoexit)

---

## Interfaces

```go
type TryLocker interface {
	TryLock() bool
	Unlock()
}

type Waiter interface{ Wait() }

type RWLocker interface {
	sync.Locker
	RLock()
	RUnlock()
	RLocker() sync.Locker
}

type Job interface {
	Wait() error
	Done() <-chan struct{}
	Cancel()
}
```

| Interface | Implemented by |
| --- | --- |
| `TryLocker` | `*sync.Mutex`, `*sync.RWMutex`, `*Limiter`, `*LimiterWithCeiling[C]` |
| `RWLocker` | `*sync.RWMutex`, the value returned by `RWLockerFactory.RWLocker` |
| `Waiter` | anything with `Wait()`: `*sync.WaitGroup`, `*Phaser` (its variadic `Wait(...sync.Locker)` doesn't match the signature, so `Phaser` does **not** satisfy it) |
| `Job` | the value returned by `Go` and `Group.Go`, and `*Group` itself |

Use these as the parameter types of your own helpers, for example to accept a `TryLocker` rather than a concrete `*sync.Mutex`.

---

## Lazy initialisation

```go
func Init[T comparable, L sync.RWMutex | sync.Mutex | sync.Once](l *L, ptr *T, init func() T) T
func InitErr[T comparable, L sync.RWMutex | sync.Mutex](l *L, ptr *T, init func() (T, error)) (T, error)
func InitErrWL[T comparable](l sync.Locker, ptr *T, init func() (T, error)) (T, error)
```

The behaviour depends on the lock you pass:

| Lock | Fast path | When is `init` called? | Caches zero results? | Caches errors? |
| --- | --- | --- | --- | --- |
| `*sync.Once` (`Init` only) | `Once.Do` | exactly once, ever | yes | yes, if `T` is `error` |
| `*sync.Mutex` | none; every call takes the lock | while `*ptr == zero` | **no**: a zero result means `init` runs again | `InitErr`: **no**, so the next call retries |
| `*sync.RWMutex` | `RLock` check, then `Lock` and a double check | while `*ptr == zero` | **no** | `InitErr`: **no** |
| any `sync.Locker` (`InitErrWL`) | uses the `RWLocker` path if `l` implements it, else the mutex path | while `*ptr == zero` | **no** | **no** |

Further details:

- If `ptr` or `l` is `nil`, the call panics with `[synckit.Init]: nil …`.
- `init` runs while the lock is held. A panic in `init` propagates, and the lock is released by `defer`.
- `T comparable` rules out slices, maps and funcs as `T`. Use a pointer (`*Config`) or a comparable struct instead.
- The functions aren't re-entrant on the same lock.

---

## Job and Go

```go
func Go(ctx context.Context, fn func(ctx context.Context) error) Job
```

- Starts `fn` in a new goroutine with a context derived from `ctx`, and returns at once.
- If `ctx` is nil it panics, because it calls `context.WithCancel(nil)`.
- The job context is cancelled when `fn` returns, so the parent `ctx` doesn't keep finished jobs alive.
- `Wait()` blocks until `fn` returns and then returns its error. Repeated calls return the same error immediately.
- `Done()` is closed when `fn` returns.
- `Cancel()` cancels the job's context. It is idempotent and safe to call concurrently with `Wait`.
- A panic in `fn` is **not** recovered and crashes the process, like `go`.

---

## Group

```go
type Group struct {
	// Isolation: a task's error doesn't cancel its siblings.
	Isolation bool
	// ErrorOnGoexit: a task stopped by runtime.Goexit makes Wait return ErrGoexit.
	ErrorOnGoexit bool
	// unexported state
}

func (g *Group) Go(ctx context.Context, fn func(ctx context.Context) error) Job
func (g *Group) Wait() error
func (g *Group) Cancel()
func (g *Group) Done() <-chan struct{}
func (g *Group) Len() int
```

`Go`:

- A `nil` `ctx` falls back to `context.Background()`.
- The task runs with its own cancellable context, derived from the `ctx` of that call. The context is released when the task returns.
- `Go` must happen before `Wait` while the group is empty. While tasks are running, `Go` may be called at any time, including from inside a task. A pending `Wait` then waits for the new task too.
- To reuse a group for a new batch, call `Go` only after the previous `Wait` has returned.
- The returned `Job` covers that one task:
  - `Wait()` returns its error. It returns `nil` if the task panicked, because the panic is reported by `Group.Wait`.
  - `Cancel()` cancels only that task.
  - `Done()` closes when that task returns.

Error handling:

- Without `Isolation`, when a task returns an error and that error isn't `errors.Is(err, taskCtx.Err())`, the group calls `g.Cancel()`. Errors caused by the task's own cancellation don't cascade.
- With `Isolation: true`, errors never cancel siblings.
- Every non-nil task error is collected, including `context.Canceled` from cancelled siblings.

`Wait`:

- Blocks until every task has returned.
- If a task panicked, `Wait` re-panics with the original panic value, in the caller's goroutine.
- Otherwise it returns the collected errors merged into one error. `errors.Is` matches each of them, and `Error()` joins the messages with newlines. If there are none it returns `nil`.
- It **drains** the collected errors and the panic. A second `Wait` with no new tasks returns `nil`.
- When `Wait` returns, every task context has been released.

The other methods:

- `Cancel()` cancels the context of every task running *now*. It doesn't wait, and later `Go` calls aren't affected. Cancelled tasks still count in `Len` and `Done` until they return.
- `Done()` returns a closed channel when no task is running, and otherwise a channel that closes when the running count drops to zero.
- `Len()` returns the number of running tasks.

---

## Limiter

```go
type Limiter struct{ /* unexported */ }

func (l *Limiter) Lock()
func (l *Limiter) Unlock()
func (l *Limiter) TryLock() bool
func (l *Limiter) Limit() int
func (l *Limiter) SetLimit(n int)
```

| `limit` | Meaning |
| --- | --- |
| `0` (the zero value) | unlimited: `Lock` never blocks, `TryLock` always succeeds |
| `n > 0` | at most `n` concurrent holders |
| `n < 0` | disabled: `Lock` blocks and `TryLock` returns `false` until the limit changes |

- `SetLimit` wakes all waiters if there is room after the change. The surplus waiters go back to waiting.
- Lowering the limit never preempts holders. New holders are admitted only once `held < limit`.
- `Unlock` with nothing held panics with `synckit.Limiter: Unlock without Lock`.
- Holders aren't goroutine-bound, so one goroutine may `Lock` and another may `Unlock`.
- `Lock` doesn't take a context.

```go
type LimitCeiling interface{ Limit() int }

type LimiterWithCeiling[Ceiling LimitCeiling] struct{ /* Limiter + Ceiling */ }

func (l *LimiterWithCeiling[C]) Lock()
func (l *LimiterWithCeiling[C]) Unlock()
func (l *LimiterWithCeiling[C]) TryLock() bool
func (l *LimiterWithCeiling[C]) Limit() int

type LimitToNumCPU struct{} // Limit() == runtime.NumCPU()
```

- `Lock` and `TryLock` call `SetLimit(ceiling.Limit())` first, on every call, using the zero value of `Ceiling`. The ceiling is therefore re-evaluated on every acquisition.
- There is no `SetLimit` on `LimiterWithCeiling`.
- A custom ceiling is any type with a `Limit() int` method that works on its zero value:

  ```go
  type limitTo8 struct{}

  func (limitTo8) Limit() int { return 8 }

  var l synckit.LimiterWithCeiling[limitTo8]
  ```

---

## Phaser

```go
type Phaser struct{ /* unexported */ }

func (p *Phaser) Wait(ls ...sync.Locker)
func (p *Phaser) Signal()
func (p *Phaser) Broadcast()
func (p *Phaser) Finish()
func (p *Phaser) Len() int
```

`Wait(ls...)`:

- Parks the calling goroutine until a `Signal` that picks it, a `Broadcast`, or `Finish`.
- After `Finish`, it returns immediately.
- If you pass lockers, the caller must hold them. They are released after the goroutine is registered as a waiter, so there is no lost wake-up between the check and the park. They are re-acquired before `Wait` returns.

The other methods:

- `Signal()` wakes one parked goroutine, if there is one. It isn't latched.
- `Broadcast()` wakes every parked goroutine. It isn't latched.
- `Finish()` broadcasts and latches the phaser as finished, permanently.
- `Len()` returns the number of currently parked goroutines. It counts up once registration has completed, so `Len() == n` means a `Broadcast` will reach all `n`.

---

## RWLockerFactory

```go
type RWLockerFactory[K comparable] struct {
	ReadOptimised bool
	// unexported
}

func (l *RWLockerFactory[K]) RWLocker(key K) RWLocker
```

- `RWLocker(key)` returns a lightweight handle. Its `Lock`/`Unlock` take the key's write lock, and its `RLock`/`RUnlock` take the key's read lock.
- The handle stores the unlock function of its *current* `Lock` and of its *current* `RLock`. Unlocking without a matching lock, or twice, panics with `synckit: unlock of unlocked mutex`.
- `RLocker()` returns a `sync.Locker` whose `Lock`/`Unlock` map to `RLock`/`RUnlock` on the key.
- Per-key entries are reference-counted and deleted when the last holder releases them.
- With `ReadOptimised`, an `RLock` on a key that has no entry takes only the factory-wide read lock. That is much faster for reads. The cost is that any writer which has to create a new entry waits until those reads finish.

---

## Map

```go
type Map[K comparable, V any] struct{ /* sync.RWMutex + map[K]V */ }
```

| Method | Lock | Notes |
| --- | --- | --- |
| `Set(k, v)` | W | |
| `Get(k) V` | R | zero value if missing |
| `Lookup(k) (V, bool)` | R | |
| `Delete(k)` | W | |
| `Len() int` | R | |
| `Reset()` | W | empties the map |
| `Do(fn func(map[K]V) error) error` | W for the whole of `fn` | `fn` gets the live, non-nil map. A `nil` `fn` is a no-op. A panic in `fn` propagates, and the map stays usable |
| `GetOrInit(k, init func() V) V` | R fast path, then W around `init` | `init == nil` returns the zero value without storing it |
| `GetOrInitErr(k, init func() (V, error)) (V, error)` | as above | errors aren't stored, so the next call retries |
| `Borrow(k) (*V, release func(), ok bool)` | W from the call until `release()` | missing key: `nil, nil, false`. `release` writes `*ptr` back and unlocks |
| `BorrowWithInit(k, init func() V) (*V, release func())` | W until `release()` | a missing key is initialised with `init` (or the zero value), and stored on `release` |
| `Keys() iter.Seq[K]` | R while taking the snapshot | yields without holding the lock |
| `Values() iter.Seq[V]` | R while taking the snapshot | yields without holding the lock |
| `All() iter.Seq2[K, V]` | W **per entry, during yield** | releases and re-acquires between entries. Also visits keys added during iteration, and skips keys deleted during it |
| `RAll() iter.Seq2[K, V]` | R per entry, during yield | as `All`, but other readers aren't blocked |
| `ToMap() map[K]V` | R | a clone |
| `MarshalJSON` / `UnmarshalJSON` | R / W | a plain JSON object |

`Map` implements `ds.Map`, `ds.ReadOnlyMap`, `ds.MapConvertible`, `ds.Keys`, `ds.Values`, `ds.All` and `ds.Len`, and passes `dscontract.Map`.

---

## Slice

```go
type Slice[T any] struct{ /* sync.RWMutex + []T */ }
```

| Method | Lock | Notes |
| --- | --- | --- |
| `Append(vs ...T)` | W | |
| `Lookup(i) (T, bool)` | R | `false` if out of range |
| `Set(i, v) bool` | W | `false` if out of range |
| `Insert(i, vs ...T) bool` | W | |
| `Delete(i) bool` | W | |
| `Len() int` | R | |
| `ToSlice() []T` | R | a clone |
| `Values() iter.Seq[T]` | W for the **whole** iteration | |
| `RValues() iter.Seq[T]` | R for the **whole** iteration | |

`Slice` implements `ds.List`, `ds.ReadOnlyList`, `ds.SliceConvertible` and `ds.Len`, and passes `dscontract.List`, `dscontract.OrderedList` and `dscontract.Sequence`.

---

## Fan and Broadcast

```go
type Fan[T any] struct{ /* unexported */ }
type Broadcast[T any] struct{ /* unexported */ }

func (x *Fan[T]) Publish(ctx context.Context, v T) error
func (x *Fan[T]) Subscribe(ctx context.Context) pubsub.Subscription[T] // iter.Seq2[pubsub.Message[T], error]
func (x *Fan[T]) Close() error
func (x *Fan[T]) Cancel()
func (x *Fan[T]) Len() int
// Broadcast has the same method set.
```

`pubsub.Message[T]` (from `port/pubsub`) provides:

- `Data() T`
- `ACK() error`
- `NACK() error`
- `Context() context.Context`

Shared semantics:

| Aspect | Behaviour |
| --- | --- |
| Buffering | none: `Publish` hands the message over synchronously |
| `nil` ctx | treated as `context.Background()` |
| Message context | derived from the subscription ctx, and cancelled when that loop iteration ends |
| End of a loop iteration without ACK | automatic NACK, so the message is redelivered |
| `ACK()` | settles the message |
| `Close()` | idempotent and permanent. Waiting and future `Publish` calls return `context.Canceled`, but a publish that races with a still-draining subscriber may be handed over, so never rely on `Close` to reject a specific message. Each subscription ends once nothing is in flight or pending redelivery. In-flight message contexts are **not** cancelled. A `Subscribe` after `Close` ends immediately |
| `Cancel()` | `Close()`, plus cancelling the context of every in-flight message |
| `Len()` | the number of active subscriptions |
| Subscription end | the subscription `ctx` is done, `Close` has drained, or `Cancel` |

`Fan` specifics (unicast, a work queue):

- Each message is delivered to exactly one subscriber.
- `Publish` blocks until a subscriber receives the message. That happens before the ACK. It returns `ctx.Err()` if `ctx` ends first.
- A NACKed message is handed to an idle subscriber if one is waiting. Otherwise it goes to a shared backlog that any subscriber picks up.
- `Fan` implements `pubsub.Publisher`, `pubsub.Subscriber`, `io.Closer` and `ds.Len`, and passes `pubsubcontract.Queue`.

`Broadcast` specifics (multicast, volatile):

- `Publish` delivers an identical copy to every subscriber registered *at publish time*, one after the other. It waits for each one to take its copy, which is back-pressure.
- Subscribers may join or leave while a `Publish` is waiting. One that leaves is skipped.
- With no subscribers, `Publish` returns `nil` immediately and the message is gone.
- A NACK redelivers to the same subscriber. If that subscriber leaves with NACKed messages, the next new subscription adopts them.

---

## IsDone

```go
func IsDone(done <-chan struct{}) bool
```

| Input | Result |
| --- | --- |
| `nil` | `false` |
| open, empty | `false` |
| closed | `true` |
| a value is ready to receive | **panics** with `implementation-error` (it is meant for close-only channels) |

---

## ErrGoexit

```go
const ErrGoexit errorkitlite.Error = "ErrGoexit"
```

`Group.Wait` returns it when `ErrorOnGoexit` is set and a task ended through `runtime.Goexit`, for example `t.FailNow` or `t.SkipNow` called inside the task. Match it with `errors.Is(err, synckit.ErrGoexit)`.
