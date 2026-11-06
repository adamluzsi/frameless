---
name: frameless-resilience
description: Use go.llib.dev/frameless/pkg/resilience instead of hand-writing retry, backoff, polling, or throttling logic in Go. Use whenever code needs to re-attempt a failing operation (loops with time.Sleep, attempt counters, exponential backoff, jitter), wait until something becomes ready, retry outgoing HTTP requests, resume interrupted stream reads or downloads, or rate-limit calls to a dependency. Also use when configuring a RetryStrategy for frameless components (httpkit.RESTClient, workflow.Runtime), testing code that retries, or changing pkg/resilience itself. Applies to any Go project that imports go.llib.dev/frameless.
compatibility: Go projects importing go.llib.dev/frameless/pkg/resilience.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: pkg/resilience
  scope: Global
---

# Frameless Resilience

`go.llib.dev/frameless/pkg/resilience` already solves retries, backoff, polling, resilient HTTP, resumable streams, and rate limiting. **Do not write your own retry logic.** Reach for this package first, and inject its types as dependencies so callers and tests can tune them.

## Stop signs

If you are about to write any of these, use `resilience` instead:

- `for i := 0; i < maxRetries; i++ { ...; time.Sleep(d) }` or any attempt counter
- backoff math (`delay *= 2`, `math.Pow`, `rand.Intn` jitter, `min(delay, maxDelay)`)
- `time.Sleep` / `time.After` / `time.Ticker` polling until something is ready
- a retry wrapper around `http.Client.Do`, or a custom retrying `http.RoundTripper`
- re-opening a stream and skipping bytes after a read error, or resumable download code
- token-bucket, ticker, or semaphore based throttling of calls to a dependency

Do not add third-party retry or rate-limit libraries (`cenkalti/backoff`, `avast/retry-go`, `hashicorp/go-retryablehttp`, `golang.org/x/time/rate`, …) to a project that already depends on frameless.

## Pick the tool

| Need | Use |
| --- | --- |
| Re-attempt an operation that fails transiently | `for range resilience.Retries(ctx, strategy)` |
| Retry outgoing HTTP requests | `resilience.HTTPRoundTripper` as `http.Client.Transport` (alias: `httpkit.RetryRoundTripper`) |
| REST resource client | `httpkit.RESTClient` — its default `http.Client` already retries |
| Keep reading when a stream fails mid-read | `*resilience.Reader` |
| Copy/download a large stream, resumable into a file | `resilience.TransferManager` |
| Cap the rate of calls to a dependency | `*resilience.SlidingWindow` behind `resilience.RateLimitPolicy` |
| Poll a condition until it holds or a deadline passes | `resilience.Waiter{...}.While(func() (continue bool))` |
| Configure retries of a frameless component | Set its `RetryStrategy` field (e.g. `workflow.Runtime.RetryStrategy`) |

## Retry an operation

Expose the strategy as an optional dependency and let `Retries` drive the loop:

```go
type Client struct {
	// RetryStrategy [optional] decides whether another attempt should be made.
	//
	// Default: resilience.DefaultRetryStrategy
	RetryStrategy resilience.RetryStrategy
}

func (c Client) Fetch(ctx context.Context, id string) (Thing, error) {
	var err error
	for range resilience.Retries(ctx, c.RetryStrategy) {
		var thing Thing
		thing, err = c.fetch(ctx, id)
		if err == nil {
			return thing, nil // happy path
		}
		if !isTemporary(err) {
			return Thing{}, err // permanent failure: don't burn the retry budget
		}
		// temporary failure: next iteration waits according to the strategy
	}
	if err == nil { // no attempt was made, e.g. ctx was already cancelled
		err = ctx.Err()
	}
	return Thing{}, err // budget exhausted: return the last error
}
```

Rules:

- The first attempt runs immediately; the strategy does the waiting before each later attempt and stops the loop when the budget is spent or `ctx` is done. **Never call `time.Sleep` inside the loop body.**
- Classify errors. Retry only transient failures (timeouts, connection resets, `503`, lock contention, …). Return permanent ones (validation, not found, auth, most `4xx`, `context.Canceled`) immediately. Never retry blindly on every error.
- Keep the last error and return it once the loop ends; wrap it with context if useful.
- Pass the caller's `ctx`; do not substitute `context.Background()` in request paths.
- Only retry idempotent operations, or make them idempotent first (idempotency keys, upserts).
- A `nil` strategy resolves to `resilience.DefaultRetryStrategy` (`Jitter{}`: up to 5 attempts, random wait of up to 5s). `Retries` resolves it for you; use `resilience.GetRetryStrategy(rs)` only when you call `ShouldTry` yourself.
- `Retries` yields a `resilience.RetryAttempt` (`FailureCount` starting at 0, `StartedAt`) if you need it for logging.
- For fine-grained budget control (e.g. failures that should not count), drive the strategy directly. `ShouldTry` blocks for the delay:

  ```go
  for attempt := resilience.MakeRetryAttempt(); rs.ShouldTry(ctx, attempt); attempt.Next() {
  	// ...
  }
  ```

### Strategies

All strategies are value types, usable as zero values, and stop when `ctx` is done.

| Strategy | Wait before a retry | Fields (default) | Good for |
| --- | --- | --- | --- |
| `Jitter` (package default) | random in `[0, Delay]` | `Delay` (5s), `Attempts` (5) | many clients hitting one dependency; avoids thundering herds |
| `ExponentialBackoff` | `Delay × 2^FailureCount` | `Delay` (500ms), `Attempts` (5 unless `Timeout` set), `Timeout` (off) | overloaded or recovering dependencies |
| `FixedDelay` | constant `Delay` | `Delay` (500ms), `Attempts` (5 unless `Timeout` set), `Timeout` (off) | predictable pacing, polling external state |
| `Waiter` | none in `ShouldTry`; `WaitDuration` in `While` | `Timeout` (30s), `WaitDuration` (1ms) | deadline-bounded loops whose attempts block on their own; `While` polling |

- `Attempts` is the total number of tries, including the first one.
- With `Timeout` and no `Attempts`, `ExponentialBackoff` and `FixedDelay` keep retrying until the accumulated wait reaches `Timeout`.
- `Waiter` with `Retries` does not pause between attempts, so a non-blocking body becomes a busy loop. Use `FixedDelay` for that case, or `Waiter.While` for polling.
- A custom strategy only needs `ShouldTry(ctx context.Context, attempt resilience.RetryAttempt) bool`. It must do its own waiting and return `false` once `ctx` is done.

## HTTP

Make the transport resilient instead of wrapping `client.Do`:

```go
client := &http.Client{
	Transport: resilience.HTTPRoundTripper{
		Transport:     base, // optional, default: http.DefaultTransport
		RetryStrategy: resilience.ExponentialBackoff{Delay: time.Second, Timeout: time.Minute}, // optional
		OnStatus: map[int]bool{ // optional overrides
			http.StatusConflict: true,  // also retry 409
			http.StatusNotFound: false, // never retry 404
		},
	},
	Timeout: 2 * time.Minute, // spans every attempt, so size it for the whole retry budget
}
```

Behaviour you get for free:

- Retries transport errors that report `Timeout() == true`, plus `net.ErrClosed` and `http.ErrHandlerTimeout`, and the statuses `408`, `429`, `500`, `502`, `503`, `504`, `507`. `OnStatus` overrides individual status codes.
- The request body is rewound for every attempt. If it isn't already an `io.ReadSeeker`, it is buffered in memory.
- For `GET`, `HEAD`, `OPTIONS`, `TRACE`, and `CONNECT`, a response body that fails mid-read is transparently re-requested and resumed at the current offset. Opt out with `DisableResponseBodyRetry: true`.
- When the budget runs out on a retryable status, the last response is returned with a `nil` error. Still check `resp.StatusCode`.

Caveats:

- Retryable statuses and timeouts are retried for every method, including `POST`/`PATCH`. Protect non-idempotent endpoints with idempotency keys, or narrow retries with `OnStatus` or a stricter strategy.
- `Retry-After` headers are not honoured yet.
- Don't layer a `Retries` loop around a client whose transport already retries. The budgets multiply.
- `httpkit.RESTClient` falls back to `httpkit.DefaultRestClientHTTPClient`, which already uses `RetryRoundTripper` with `ExponentialBackoff{Delay: time.Second, Timeout: time.Minute}`. Only supply `HTTPClient` to change that.

## Resilient streams

`Reader` re-opens its source on non-EOF read errors and replays it to the current offset (via `io.Seeker` when available, otherwise by discarding bytes), so consumers see one continuous stream:

```go
r := &resilience.Reader{
	Context: ctx,
	Open: func() (io.Reader, error) {
		return openSource(ctx) // must yield the same bytes from the start every time
	},
	RetryStrategy: resilience.FixedDelay{Delay: time.Second, Attempts: 7},
}
defer r.Close() // also closes the current source if it is an io.Closer

data, err := io.ReadAll(r)
```

`TransferManager` copies a source into an output resiliently and closes both before returning:

```go
tm := resilience.TransferManager{
	RetryStrategy: resilience.ExponentialBackoff{Attempts: 10}, // optional
	// BufferSize: 16 * iokit.Megabyte (default)
}

err := tm.Transfer(ctx,
	func() (io.ReadCloser, error) { return openRemote(ctx) },
	func() (io.WriteCloser, error) {
		return os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644) // no O_TRUNC, so it can resume
	},
)
```

- Resume: if the output already has data, exposes `Stat() (fs.FileInfo, error)` (as `*os.File` does), and is an `io.Seeker`, the transfer appends from the existing size and replays the source from that offset. If it can't seek but can `Truncate`, it starts over.
- The source is kept alive during slow writes to avoid idle read timeouts on remote streams.

## Rate limiting

```go
type Worker struct {
	// RateLimit [optional] throttles calls to the dependency.
	RateLimit resilience.RateLimitPolicy
}

w := Worker{RateLimit: &resilience.SlidingWindow{
	Rate: resilience.Rate{N: 5, Per: time.Minute},
}}

if err := w.RateLimit.RateLimit(ctx); err != nil {
	return err // ctx cancelled while waiting for a slot
}
callDependency(ctx)
```

- `RateLimit` blocks until a slot is free within the sliding window, or returns `ctx.Err()`.
- Use a pointer and share **one** instance per limited resource across goroutines. It holds the event history behind a mutex: don't copy it after use, and don't create one per call.
- A zero `Rate` means no limit.
- When combining with retries, call `RateLimit` at the top of each attempt inside the `Retries` loop so retries also respect the limit.

## Test code that retries

- Inject a fast strategy, e.g. `resilience.FixedDelay{Delay: time.Nanosecond, Attempts: 3}` or `resilience.Jitter{Delay: time.Nanosecond, Attempts: n}`.
- Disable retries with a single-attempt strategy:

  ```go
  type singleAttempt struct{}

  func (singleAttempt) ShouldTry(_ context.Context, a resilience.RetryAttempt) bool {
  	return a.FailureCount == 0
  }
  ```

- Strategies read time through `go.llib.dev/testcase/clock`, so `timecop.SetSpeed(t, math.MaxFloat64)` or `timecop.Travel(t, d)` controls delays and deadlines without real sleeping.
- Cover at least: transient failure then success → success; permanent error → exactly one call; budget exhausted → last error returned; cancelled `ctx` → loop stops.
- For HTTP, put `HTTPRoundTripper` in front of a fake `http.RoundTripper` (or an `httptest.Server`) that fails N times before succeeding.
- Follow the project's `go.llib.dev/testcase` spec style.

## Changing `pkg/resilience` itself

- Strategies must honour `ctx`, take time from `go.llib.dev/testcase/clock` (never `time.Now`/`time.After` directly) so `timecop` works, and stay usable as zero values with documented defaults.
- New strategies need a `var _ RetryStrategy = X{}` assertion, an `Example*` test, spec tests, and a section in `pkg/resilience/README.md`.
- Dependants: `pkg/httpkit`, `pkg/netkit`, `pkg/workflow` (and `wftest`, `internal/promise`), and `port/crud/crudtest`. Run `go test ./pkg/resilience/... ./pkg/httpkit/... ./pkg/netkit/... ./pkg/workflow/... ./port/crud/...` after changes.

## Reference

- `pkg/resilience/README.md`: overview with examples
- `pkg/resilience/retry.go`: `RetryStrategy`, `Retries`, the strategies
- `pkg/resilience/http.go`: `HTTPRoundTripper`
- `pkg/resilience/resio.go`: `Reader`, `TransferManager`
- `pkg/resilience/ratelimit.go`: `SlidingWindow`, `Rate`
- `pkg/resilience/*_test.go`: `Example*` functions and spec tests

## Checklist

- [ ] No hand-written retry loops, backoff math, sleep-polling, or throttling. `resilience` (or a component's `RetryStrategy` field) is used instead.
- [ ] The retry strategy and rate limiter are injectable fields with sensible defaults, not hard-coded.
- [ ] Only transient errors are retried; permanent errors return immediately; the last error is returned when the budget runs out.
- [ ] Retried operations are idempotent, or protected by idempotency keys.
- [ ] The caller's `ctx` flows into `Retries`, `Reader.Context`, `Transfer`, and `RateLimit`.
- [ ] HTTP retries live in the transport, with no extra retry loop around `client.Do`; `http.Client.Timeout` covers the whole retry budget.
- [ ] Tests use near-zero delays or `timecop`, and cover the success, permanent-failure, exhausted-budget, and cancellation paths.
