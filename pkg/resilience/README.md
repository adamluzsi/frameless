# Package `resilience`

The `resilience` package provides tooling to make your code more resilient against failures,
and to protect downstream dependencies against overloads.

It covers a few complementary areas:

- **Retry** — general re-attempt operations that fail temporarily.
- **Rate Limiting** — keep operations within a controlled rate.
- **HTTP** — automatic retries for outgoing HTTP requests.
- **Resilient I/O** — stream copies and downloads that survive transient failures.

## Retry

Using a retry policy is as easy as iterating with a for loop.
Instead of a max-attempts condition, you let a `RetryStrategy` decide whether another attempt should be made.

```go
type MyStruct struct {
	RetryStrategy resilience.RetryStrategy
}

func (ms MyStruct) MyFunc(ctx context.Context) error {
	var err error 
	for range resilience.Retries(ctx, ms.RetryStrategy) {
		err = ms.do(ctx)
		if err != nil {
			if ms.isErrTemporary(err) {
				continue // try again
			}
			return err // break on a non-recoverable issue
		}
		return nil // happy path
	}
	// in case retry attempts ran out of budget,
	// return the last error
	return err 
}
```

The package ships with several interchangeable strategies — pass whichever one fits to `Retries`.
You can also use them directly without the `Retries` helper function if you need a fine tuned error retry budget handling.

### ExponentialBackoff

Doubles the wait time after each failed attempt, giving a struggling dependency progressively more room to recover.

```go
retry := resilience.ExponentialBackoff{
	Delay:    time.Second,
	Attempts: 10,
}
```

### Jitter

Adds randomness to the delay between attempts, so multiple clients don't retry in lockstep and overwhelm the system (the thundering herd problem).

```go
retry := resilience.Jitter{
	Delay:    10 * time.Second,
	Attempts: 7,
}
```

### Waiter

Retries until a single overall timeout elapses, without counting attempts. Simple and predictable.

```go
retry := resilience.Waiter{
	Timeout: 30 * time.Second,
}
```

### FixedDelay

Waits a constant amount of time between attempts — no backoff, just steady, predictable timing.

```go
retry := resilience.FixedDelay{
	Delay:    time.Second,
	Attempts: 10,
}
```

## Rate Limiting

Rate limiting keeps operations within a controlled rate, preventing overloads on systems or services.

`SlidingWindow` enforces a limit of N events per time window, spreading requests evenly and waiting when the limit is reached.
Call `RateLimit` before the operation you want to throttle.

```go
var rl = &resilience.SlidingWindow{
	Rate: resilience.Rate{N: 5, Per: time.Minute}, // up to 5 requests per minute
}
```

```go
for _, job := range jobs {x
	if err := rl.RateLimit(ctx); err != nil {
		return err
	}
	doRequest(job)
}
```

If the context is canceled while waiting, `RateLimit` returns the context error.
A zero-value `Rate` disables limiting, allowing unlimited requests.

## HTTP

`HTTPRoundTripper` is an `http.RoundTripper` that adds automatic retries to outgoing requests.
Drop it into any `http.Client` and recoverable failures — network timeouts, dropped connections, and `5xx` / `429` / `408` responses — are retried for you, honouring the server's `Retry-After` and rate limit hints.

For methods that are safe to repeat, like `GET`, it can even recover a response body that fails mid-read by re-issuing the request and resuming the stream.

```go
client := &http.Client{
	Transport: resilience.HTTPRoundTripper{},
}

resp, err := client.Get("https://example.com")
```

You can plug in a specific retry strategy, and override which status codes are retried via `OnStatus`:

```go
client := &http.Client{
	Transport: resilience.HTTPRoundTripper{
		RetryStrategy: resilience.ExponentialBackoff{Attempts: 5},
		OnStatus: map[int]bool{
			http.StatusConflict: true,  // also retry 409
			http.StatusNotFound: false, // never retry 404
		},
	},
}
```

### Rate limits and `Retry-After`

A retryable response, typically `429 Too Many Requests` or `503 Service Unavailable`, may tell the client how long to back off.
`HTTPRoundTripper` then waits at least that long before the next attempt.
The retry strategy's own delay still applies, and the longer of the two wins.

The supported retry hints, in order of precedence:

| Header | Example | Format |
| --- | --- | --- |
| `Retry-After-Ms`, `X-Ms-Retry-After-Ms` | `retry-after-ms: 1500` | Milliseconds. Used by Azure and Azure OpenAI. |
| `Retry-After` | `Retry-After: 30` or `Retry-After: Fri, 01 Jan 2027 00:00:00 GMT` | Seconds or an HTTP-date, as defined in [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110#section-10.2.3). |
| `RateLimit` | `RateLimit: "default";r=0;t=30` | The [IETF RateLimit header fields draft](https://datatracker.ietf.org/doc/draft-ietf-httpapi-ratelimit-headers/). The longest `t` among the policies with no remaining quota (`r=0`) is used. |
| `RateLimit-Reset`, `X-RateLimit-Reset` | `X-RateLimit-Reset: 1798761600` | Seconds, Unix epoch seconds, an HTTP-date or RFC 3339. Used only when the matching `*-Remaining` header is absent or `0`. |

Absolute times are measured from the response's `Date` header, so clock skew between the client and the server doesn't distort the delay.
Malformed hints are ignored.

`MaxRetryAfter` caps how long `HTTPRoundTripper` is willing to wait for a hint. The default is 1 minute.
If the server asks for longer, or for longer than the time left before the request context's deadline (for example, the deadline set by `http.Client.Timeout`), the response is returned at once with a `nil` error instead of retrying too early.
Check `resp.StatusCode` and the hint headers to decide what to do next.
A negative `MaxRetryAfter` disables retry hints.

```go
client := &http.Client{
	Transport: resilience.HTTPRoundTripper{
		MaxRetryAfter: 30 * time.Second,
	},
	Timeout: time.Minute, // leave room for the hinted wait
}
```

## Resilient I/O

### TransferManager

`TransferManager` copies a source stream into an output stream resiliently — ideal for downloading large files over a flaky connection.
If a read fails partway through, the source is re-opened and replayed from where it left off, and an interrupted transfer into a file can resume instead of starting over.

```go
tm := resilience.TransferManager{
	RetryStrategy: resilience.ExponentialBackoff{Attempts: 10},
}

err := tm.Transfer(ctx,
	func() (io.ReadCloser, error) {
		resp, err := http.Get("https://example.com/large-file.zip")
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	},
	func() (io.WriteCloser, error) {
		return os.OpenFile("large-file.zip", os.O_RDWR|os.O_CREATE, 0644)
	},
)
```

### Reader

`Reader` is the building block behind the resilient behaviour above.
It's a seamless `io.Reader` that, when a read fails, re-opens the underlying source via `Open` and replays it up to the current offset — so the consumer sees one uninterrupted stream.

```go
reader := &resilience.Reader{
	Open: func() (io.Reader, error) {
		return os.Open("name")
	},
	RetryStrategy: resilience.FixedDelay{
		Delay:    time.Second,
		Attempts: 7,
	},
}

data, err := io.ReadAll(reader)
```
