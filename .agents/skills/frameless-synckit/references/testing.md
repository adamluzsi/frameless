# Testing concurrent code with synckit

How to test code that uses synckit, how to use synckit to make concurrency tests deterministic, and how to test `pkg/synckit` itself. It pairs with the `testcase-spec`, `testcase-assert` and `testcase-clock` skills. Read those for spec structure, assertions and time control.

Contents:

- [Rules](#rules)
- [Gate fakes with a Phaser](#gate-fakes-with-a-phaser)
- [Assert that something blocks, then unblocks](#assert-that-something-blocks-then-unblocks)
- [Wait for asynchronous outcomes](#wait-for-asynchronous-outcomes)
- [Phasers in specs: let.Phaser](#phasers-in-specs-letphaser)
- [Collect results from goroutines](#collect-results-from-goroutines)
- [Test Group and Job behaviour](#test-group-and-job-behaviour)
- [Test pub/sub consumers and publishers](#test-pubsub-consumers-and-publishers)
- [Contracts](#contracts)
- [Race tests](#race-tests)
- [Time](#time)
- [Testing pkg/synckit itself](#testing-pkgsynckit-itself)
- [Anti-patterns](#anti-patterns)

---

## Rules

- **Never `time.Sleep` to "let goroutines run".** Park them on a `Phaser` and wait for `Len()`, or use `Eventually`, `Within` and `NotWithin`.
- **Never leave goroutines parked.** Register `t.Cleanup(p.Finish)` for every phaser that a test uses, and cancel or `Wait` every job and group the test starts.
- **Always run with `-race`**: `go test -race ./...`.
- Fakes are real implementations with gates, not mocks. A fake that has to block exposes a `Phaser`. A fake that records calls uses a `synckit.Slice` or `synckit.Map`.

---

## Gate fakes with a Phaser

A gated fake parks every call until the test releases it. While the calls are parked, the test can observe how many are in flight.

```go
type GatedProcessor struct {
	Gate    synckit.Phaser
	Handled synckit.Slice[Item]
}

func (p *GatedProcessor) Process(ctx context.Context, item Item) error {
	p.Gate.Wait()
	p.Handled.Append(item)
	return nil
}
```

Testing that a batch never runs more than 3 items at once:

```go
func TestBatch_ProcessAll_capsConcurrency(t *testing.T) {
	var proc GatedProcessor
	t.Cleanup(proc.Gate.Finish) // never leave parked goroutines behind

	b := Batch{Processor: &proc, MaxParallel: 3}

	var err error
	w := assert.NotWithin(t, 100*time.Millisecond, func(context.Context) {
		err = b.ProcessAll(t.Context(), make([]Item, 10)) // not the block's ctx: see below
	})

	assert.Eventually(t, time.Second, func(t testing.TB) {
		assert.Equal(t, proc.Gate.Len(), 3) // exactly 3 parked, not 10
	})

	proc.Gate.Finish()
	assert.Within(t, time.Second, func(context.Context) { w.Wait() })
	assert.NoError(t, err)
	assert.Equal(t, proc.Handled.Len(), 10)
}
```

- `Len()` counts goroutines that are fully parked, so `Len() == n` means a `Broadcast` or `Finish` reaches all `n`.
- Use `Broadcast()` to release the current round and keep the gate usable. Use `Finish()` to release everyone, permanently.
- A `Signal` or `Broadcast` with nobody parked is lost. Always wait for `Len()` first.

---

## Assert that something blocks, then unblocks

```go
w := assert.NotWithin(t, timeout, func(context.Context) {
	_ = job.Wait() // must still be blocked after timeout
})

release()

assert.Within(t, timeout, func(context.Context) {
	w.Wait() // the blocked call from above has now returned
})
```

- `assert.NotWithin` returns an `*assert.Async`. Its `Wait()` waits for the block to finish, and re-raises a panic or `Goexit` from it in your goroutine.
- `assert.Within` fails if the block doesn't finish in time.
- **The `context.Context` that `Within` and `NotWithin` pass to the block is cancelled when the assertion returns.** If the code under test must keep running after `NotWithin` returns, give it `t.Context()` or a context you own, not the block's.
- Write results to variables captured by the block, and read them after `w.Wait()`. `Wait` gives you the happens-before edge that the race detector needs.

---

## Wait for asynchronous outcomes

```go
// plain testing.T
assert.Eventually(t, time.Second, func(t testing.TB) {
	assert.Equal(t, fake.Calls.Len(), 2)
})

// testcase.T, using the spec's retry strategy
t.Eventually(func(t *testcase.T) {
	got, ok := gateway.Get(t).Charged(invoiceID)
	assert.True(t, ok)
	assert.Equal(t, got, amount)
})
```

`Eventually` retries the block until it passes or the budget runs out. Use it for the effects of background jobs, consumers and broadcasts.

---

## Phasers in specs: let.Phaser

```go
s.When("the gateway is slow", func(s *testcase.Spec) {
	gate := let.Phaser(s).EagerLoading(s)

	gateway.Let(s, func(t *testcase.T) *FakeGateway {
		return &FakeGateway{OnCharge: func(ctx context.Context) error {
			gate.Get(t).Wait()
			return nil
		}}
	})

	s.Then("callers are not blocked", func(t *testcase.T) {
		assert.Within(t, time.Second, func(context.Context) { act(t) })
	})

	s.Then("charges complete once the gateway responds", func(t *testcase.T) {
		act(t)
		gate.Get(t).Finish()
		t.Eventually(func(t *testcase.T) { /* … */ })
	})
})
```

- `let.Phaser(s)` returns a `testcase.Var[*tcsync.Phaser]`. It has the same API as `synckit.Phaser` (`Wait(...sync.Locker)`, `Signal`, `Broadcast`, `Finish`, `Len`), and `t.Cleanup(p.Finish)` is already registered.
- Use `.EagerLoading(s)` when the first `Get` may happen in a background goroutine. Eager loading registers the cleanup on the test goroutine, rather than on the job's goroutine, where it would race with `T.Eventually`. `pkg/synckit/synckit_test.go` does this for long-lived jobs.
- `tcsync.Phaser` is testcase's own copy, not `synckit.Phaser`. Fixes in one don't reach the other.

---

## Collect results from goroutines

```go
var got synckit.Slice[string]
g.Go(ctx, func(ctx context.Context) error { got.Append("a"); return nil })
g.Go(ctx, func(ctx context.Context) error { got.Append("b"); return nil })
assert.NoError(t, g.Wait())
assert.ContainsExactly(t, got.ToSlice(), []string{"a", "b"}) // order-independent
```

- Use `synckit.Map[K, struct{}]` to collect distinct values from goroutines. `pkg/netkit/netkit_test.go` gathers the ports that concurrent `FreePort` calls return (started with `testcase.Race`), and asserts that more than one distinct port came back.
- Assert on `ToSlice()` or `ToMap()` copies, not inside `Values()` loops, which hold the lock.

Seen in: `pkg/chankit/batch_test.go`, `pkg/iokit/iokit_test.go` (a `BGRead` helper that embeds a `Group` and keeps `Results synckit.Slice[[]byte]`).

---

## Test Group and Job behaviour

```go
// errors: merged, so match with ErrorIs
assert.ErrorIs(t, g.Wait(), errBoom)

// panics: re-raised by Group.Wait
assert.Panic(t, func() { _ = g.Wait() })

// a task has finished, without blocking
assert.True(t, synckit.IsDone(job.Done()))

// the group is idle
assert.Equal(t, g.Len(), 0)
```

- Assertions that call `t.FailNow()` inside a group task end the task with `runtime.Goexit`. Use `synckit.Group{ErrorOnGoexit: true}` so that `Wait` reports it as `synckit.ErrGoexit` instead of losing it. testcase's own `T` does this internally.
- For long-running jobs, register cleanup right after you start them: `t.Cleanup(func() { job.Cancel(); _ = job.Wait() })`. `examples/ports-and-adapters/domain/billing/billing_test.go` does this with `t.Defer`.

---

## Test pub/sub consumers and publishers

`port/pubsub/pubsubtest.Subscribe` starts a background consumer that ACKs and records every message. It stops automatically at cleanup.

```go
var bc synckit.Broadcast[string]

r1 := pubsubtest.Subscribe[string](t, &bc, t.Context())
r2 := pubsubtest.Subscribe[string](t, &bc, t.Context())

// Broadcast drops messages that nobody hears: wait until both subscribers are registered
assert.Eventually(t, time.Second, func(t testing.TB) { assert.Equal(t, bc.Len(), 2) })

assert.NoError(t, bc.Publish(t.Context(), "hello"))

r1.Eventually(t, func(t testing.TB, got []string) { assert.Equal(t, got, []string{"hello"}) })
r2.Eventually(t, func(t testing.TB, got []string) { assert.Equal(t, got, []string{"hello"}) })
```

`*pubsubtest.AsyncResults[T]` provides:

- `Values()`: a copy of what was received
- `Eventually(tb, func(testing.TB, []T))`
- `AssertEmpty(tb)`: waits briefly, then asserts that nothing arrived
- `ReceivedAt()`
- `Finish()`: stops the consumer early
- `Subscription()`

To test a consumer you wrote, publish into a `synckit.Fan`, run the consumer in a `Group` or with `synckit.Go`, and assert its side effects with `Eventually`. To test a publisher, subscribe with `pubsubtest.Subscribe` *before* you trigger it, because `Fan.Publish` blocks without a subscriber and `Broadcast.Publish` drops the message.

---

## Contracts

If you build an adapter on top of synckit, for example a `pubsub` port backed by a `Fan`, run the same contract that synckit runs:

```go
func TestMyQueue(t *testing.T) {
	q := &MyQueue{} // wraps a synckit.Fan[Job]
	pubsubcontract.Queue[Job](q, q, pubsubcontract.Config[Job]{
		MakeData: func(tb testing.TB) Job { return Job{ID: testcase.ToT(&tb).Random.UUID()} },
	}).Test(t)
}
```

For wrappers around `synckit.Map` or `synckit.Slice`, use `dscontract.Map[K, V](mk)` and `dscontract.List(mk)` from `port/ds/dscontract`. Every contract offers `.Test(t)` for plain tests and `.Spec(s)` inside a spec.

Seen in: `pkg/synckit/exchange_test.go` (`"implements pubsub Queue contract"`), and `pkg/synckit/synckit_test.go` (`dscontract.Map`, `List`, `OrderedList`, `Sequence`).

---

## Race tests

```go
testcase.Race(
	func() { m.Set("a", 1) },
	func() { _ = m.Get("a") },
	func() { _ = m.ToMap() },
)
```

`testcase.Race` starts every function at the same moment and waits for all of them. It doesn't assert anything by itself; the race detector does the work, so run it with `go test -race`. Add a `Race` test for every new type or method that is meant to be safe for concurrent use.

---

## Time

Code that waits on time (tickers, timeouts, backoff) should use `go.llib.dev/testcase/clock` (`clock.NewTicker`, `clock.After`, `clock.Now`). Tests then move time with `timecop.Travel` instead of sleeping. See `testcase-clock`. The keepAlive in `adapter/postgresql/guard.go` and the Broadcast listener in `pkg/workflow/schedule.go` both use `clock.NewTicker`.

---

## Testing pkg/synckit itself

- Specs live in `pkg/synckit/synckit_test.go` (`Map`, `Slice`, `Job`, `Group`, `Phaser`, `Limiter`, `RWLockerFactory`, `IsDone`), `pkg/synckit/exchange_test.go` (`Fan`, `Broadcast`) and `pkg/synckit/init_test.go`. White-box tests are in `synckit_internal_test.go`.
- Every exported type needs:
  - an `Example*` function
  - a spec
  - a `testcase.Race` test
  - a contract run, where the type implements a port (`pubsubcontract`, `dscontract`)
- **Give examples an `// Output:` comment when they can be deterministic.** At the moment none has one, so `go test` never runs them, and `ExampleLimiter_changingLimitDynamically` and `ExampleFan_in` would hang if it did. See [pitfalls](pitfalls.md#known-quirks-in-pkgsynckit).
- Primitives that create a context per task or per message need a leak guard. `TestFan_leakGuard` and `TestGroup_leakGuard` run 200,000 short tasks or messages against one long-lived context, force GC, and assert that the retained `HeapAlloc` per operation stays small. Copy that shape.
- To reproduce a flaky spec, rerun it with the seed it printed, for example `TESTCASE_SEED=2569458535983166363 go test -run 'TestRWLockerFactory/…' ./pkg/synckit/`. `pkg/synckit/TODO.md` records one such seed.
- Commands:

  ```sh
  go test -race ./pkg/synckit/...
  go test -race -count=20 -run TestPhaser ./pkg/synckit/   # shake out ordering bugs
  go vet ./pkg/synckit/...
  go test ./pkg/... ./port/... ./adapter/memory/...         # dependants in the root module
  ```

  `adapter/postgresql`, `adapter/mariadb`, `adapter/localfs` and `adapter/hashicorp/vault` are separate modules. Run their tests from their own directories if your change can affect them.

---

## Anti-patterns

| Don't | Do |
| --- | --- |
| `time.Sleep(10 * time.Millisecond)` "to let it start" | park on a `Phaser`, then `Eventually(p.Len() == n)` |
| `select { case <-done: case <-time.After(time.Second): t.Fatal() }` | `assert.Within(t, time.Second, …)` |
| assert "still blocked" with a sleep and a flag | `w := assert.NotWithin(…)`, later `w.Wait()` |
| a mutex-guarded `[]T` written by hand in the test | `synckit.Slice[T]` |
| a `chan struct{}` gate closed by hand, never closed if the test fails | `Phaser` with `t.Cleanup(p.Finish)`, or `let.Phaser(s)` |
| `Broadcast.Publish` right after starting subscribers | wait for `bc.Len() == n` first |
| `assert.Equal(t, err, context.Canceled)` on a `Group` error | `assert.ErrorIs(t, err, context.Canceled)` |
| passing `NotWithin`'s block ctx to long-running code under test | `t.Context()` or a context you own |
