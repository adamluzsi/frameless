# Package `resilience`

The `resilience` package provides tooling to make your code more resilient against failures or to make downstream dependencies more protected against overloads.

## retry

Using a retry policy easy as iterating with a for loop,
but instead of making a condition based on a max value,
we check it with `resilience.RetryPolicy#ShouldTry`.

> **Example**:

```go
package mypkg

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/pkg/resilience"
)

func (ms MyStruct) MyFunc(ctx context.Context) error {
	var rp resilience.ExponentialBackoff

	for range := resilience.Retries(ctx, rp) {
		err := ms.DoAction(ctx)
		if err != nil {
			if ms.isErrTemporary(err) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("failed to DoAction")
}
```

The package contains multiple strategies for retrying:

- **ExponentialBackoff**: Implements an exponential backoff strategy where the wait time between retries doubles after each failure.
- **Jitter**: Adds randomness (jitter) to the delay between retries to prevent thundering herd issues.
- **Waiter**: Uses a fixed timeout since the start of the operation to determine if retry attempts should continue.
- **FixedDelay**: Waits for a fixed amount of time between retries without exponential backoff.

### ExponentialBackoff

ExponentialBackoff is a RetryPolicy implementation that uses an exponentially increasing delay between retry attempts.
This strategy helps prevent overwhelming the downstream system by giving it more time to recover after each failed attempt.

**Key Features:**

- Initial delay (default: 500ms)
- Exponentially increases delay for each subsequent retry
- Configurable maximum number of retries (default: 5)
- Optional timeout to limit total waiting time

**When to use**:

- When you want to gradually increase the wait time between attempts
- If you need a balance between fast retries and system recovery time
- When the failure rate is high but expected to decrease over time

**Example**:

```go
retry := resilience.ExponentialBackoff{
    Delay:  time.Second,
    Timeout: 30 * time.Second, // set Timeout OR max Attempts
    Attempts: 10,
}
```

### Jitter

Jitter adds randomness to the delay between retry attempts.
This helps prevent multiple clients from retrying simultaneously (thundering herd problem)
while still maintaining a reasonable maximum wait time.

**Key Features:**

- Randomized delay up to a configured maximum
- Configurable maximum number of retries (default: 5)
- Prevents synchronization between retrying clients if the resource experiencing a temporal outage

**When to use**:

- In distributed systems with multiple clients
- When you want to avoid thundering herd issues
- If you need some randomness in your retry timing

**Example**:

```go
retry := resilience.Jitter{
    Delay:   10 * time.Second,
    Attempts: 7,
}
```

### Waiter

Waiter strategy retries based on the total elapsed time since the operation started.
It will keep retrying as long as the timeout hasn't been exceeded.

**Key Features:**

- Single fixed timeout for all attempts
- Doesn't count individual attempts - only tracks total time
- Simple and predictable behavior

**When to use**:

- When you need a hard time limit on total retry time
- If you prefer simplicity over more complex backoff strategies
- For operations with strict time constraints

**Example**:

```go
retry := resilience.Waiter{
    Timeout: 30 * time.Second,
}
```

### FixedDelay

FixedDelay retries with a constant delay between attempts.
Unlike exponential backoff, the wait time doesn't increase - it stays fixed for all attempts.

**Key Features:**

- Fixed delay between retries (default: 500ms)
- Configurable maximum number of retries (default: 5)
- Optional timeout to limit total waiting time
- Simple and predictable timing

**When to use**:

- When you want consistent timing between retries
- If a simple retry strategy is sufficient
- For operations with known, fixed recovery times

**Example**:

```go
retry := resilience.FixedDelay{
    Delay:   time.Second,
    Timeout: 30 * time.Second,
    Attempts: 10,
}
```

## Rate Limiting

Rate limiting ensures that operations are performed at a controlled rate, preventing overloads on systems or services.

### SlidingWindow

SlidingWindow implements a token bucket-like approach with a sliding time window to enforce rate limits.

**Key Features:**

- **Token Management:** Tracks the number of requests (events) within a configurable time window.
- **Rate Enforcement:** Ensures operations do not exceed a specified rate, calculated as N tokens per Per duration.
- **Sliding Window:** Dynamically adjusts the window based on request timing to prevent thundering herd issues.
- **Context Awareness:** Honors context cancellation and returns appropriate errors.
- **Efficient Timing:** Calculates necessary wait periods when the rate limit is exceeded.

**When to use:**

- To enforce strict rate limits, such as API call quotas.
- When you need smooth, evenly distributed requests over time.

**Configuration Options:**

- `Rate`: A struct specifying the number of tokens (`N`) and the duration (`Per`) for which this rate applies. The `Pace()` method calculates the minimum interval between allowed requests.
  - Example: `Rate{N: 10, Per: time.Minute}` allows up to 10 requests per minute.

### How It Works

1. **Initialization**: Create a SlidingWindow instance with your desired Rate.
2. **Usage**: Call `RateLimit(context.Context)` before performing the operation you wish to rate limit.
3. **Flow**:
   - If the context is canceled, returns immediately with the context error.
   - Checks if current requests are within the allowed rate for the window.
     - If under the limit, proceeds and records the event.
     - If over the limit, calculates the wait time until the window slides enough to allow more requests.

### Example

```go
package mypkg

import (
	"context"
	"fmt"
	"time"

	"go.llib.dev/frameless/pkg/resilience"
)

func main() {
	ms := MyStruct{
		RateLimitPolicy: &resilience.SlidingWindow{
			Rate: resilience.Rate{N: 5, Per: time.Minute}, // Allow 5 requests per minute
		},
	}

	_ = ms // start your app that uses MyStruct#MyFunc multiple times that requires rate limiting.
}

type MyStruct struct {
	RateLimitPolicy resilience.RateLimitPolicy
}

func (ms MyStruct) MyFunc(ctx context.Context) error {
	if err := ms.RateLimitPolicy.RateLimit(ctx); err != nil {
		return fmt.Errorf("rate limit exceeded: %w", err)
	}

	// Perform the rate-limited operation here.
	fmt.Println("Performing request...")

	return nil
}

```

### Behavior

- **Context Cancellation**: If the provided context is canceled during a rate limit wait, `RateLimit` returns immediately with the context error.
- **Zero Rate Configuration**: If `.Rate` is not set (zero value), calls to `RateLimit` will not block execution, allowing unlimited requests.
- **Even Distribution**: Requests are distributed as evenly as possible within the specified window, preventing spikes and ensuring smooth operation.

### When To Use

- **API Rate Limits:** Enforce API call quotas imposed by external services.
- **Resource Protection:** Prevent overloading of internal or external resources.
- **Distributed Systems:** Avoid synchronized retries in distributed environments, reducing the likelihood of thundering herd issues.
- **Predictable Workloads:** Maintain a consistent request rate for systems that require predictable load patterns.

##