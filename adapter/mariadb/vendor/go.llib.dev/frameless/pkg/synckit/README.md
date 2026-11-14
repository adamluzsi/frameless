# synckit

Small, generic concurrency helpers that fill the gaps in the standard `sync` package.

Everything here follows the same rule as `sync.Mutex`: **the zero value is ready to use**, so these types drop into structs without constructors or extra wiring.

```go
import "go.llib.dev/frameless/pkg/synckit"
```

## Lazy initialisation

Initialise a value exactly once, behind a lock you already have.
Makes it convenient to write init functions for certain values.

```go
db := synckit.Init(&once, &conn, func() *DB {
    return mustConnect()
})
```

`Init` works with `*sync.Once`, `*sync.Mutex` or `*sync.RWMutex`. 

Use `InitErr` when the initialiser can fail:

```go
db, err := synckit.InitErr(&mu, &conn, func() (*DB, error) {
    return connect()
})
```

## Go & Group

`Go` runs a function in a goroutine and hands back a cancellable `Job`.

```go
job := synckit.Go(ctx, func(ctx context.Context) error {
    return work(ctx)
})

err := job.Wait() // blocks until done
job.Cancel()      // cancels the job's context
<-job.Done()      // locks until the job finishes
```

`Group` is an `errgroup`-style coordinator that is itself a `Job`.

`Group` can sync cancellation across the group members,
but can also isolate the goroutines
and just act as a high-level control structure.

```go
var g synckit.Group
defer g.Cancel() // cancel group members

g.Go(ctx, fetchA)
g.Go(ctx, fetchB)
job := g.Go(ctx, fetchC)

err := g.Wait() // wait on all group members
```

## Limiter

Limiter is a max concurrency count limiter.

It can be dynamically adjusted at runtime too.
For example, upon context cancellation,
the limit can be set to unlimited 
to allow all affected goroutines
to detect the context cancellation.

```go
var l synckit.Limiter
l.SetLimit(10) // 0 = unlimited, <0 = block everything, >0 = fixed limit 

for range 42 {
		go func(){
			l.Lock() // could be called from the range itself too
			defer l.Unlock()
			if err := ctx.Err(); err != nil {
				return
			}
		}()
}

<-ctx.Done()
l.SetLimit(0)
```

Prefer type-level configuration?
Use `LimiterWithCeiling` so the cap is part of the zero value:

```go
type MyType struct {
	l synckit.LimiterWithCeiling[synckit.LimitToNumCPU]
}

func (i MyType) Do() {
	i.l.Lock()
	defer i.l.Unlock()

	// the max number of concurrent execution limited 
	// to the number of CPU available on the system
}
```

`TryLock` is supported.

## Phaser

A coordination primitive that blends a latch, a barrier and a phaser. 
Goroutines register via `Wait`,
and are released one at a time (`Signal`),
all at once (`Broadcast`), or permanently (`Finish`).

```go
var p synckit.Phaser

go func() { p.Wait(); proceed() }()

b.Signal()
p.Broadcast() // release everyone currently waiting
p.Finish()    // release all and make future Wait calls return immediately
```

In case a sync mutex is used to guard a certain code path,
`Phaser#Wait` accepts optional `sync.Locker`s,
which are released while waiting and re-acquired afterwards,
mirroring `sync.Cond` semantics without forcing the use of a mutex.

## RWLockerFactory

Per-key read/write locks, so independent keys never block each other.

```go
var locks synckit.RWLockerFactory[string]

l := locks.RWLocker("user:42")
l.Lock()
defer l.Unlock()
```

Set `ReadOptimised` to trade a small write-path cost for much faster read locking.

## Helpers

- `IsDone(<-chan struct{})`: non-blocking check whether a done channel is closed.
- `TryLocker`, `RWLocker`, `Waiter`: small interfaces used across the package.
