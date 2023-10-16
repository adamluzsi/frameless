# Package `retry`

The `retry` package provides tooling to do retries easily in your application code. 

## retry strategies

Using a retry strategy easy as iterating with a for loop,
but instead of making a condition based on a max value,
we check it with retry.Strategy#ShouldTry.

> Example:

```go
package mypkg

import (
	"context"
	"fmt"
	"go.llib.dev/frameless/pkg/retry"
)

func (ms MyStruct) MyFunc(ctx context.Context) error {
	var rs retry.ExponentialBackoff

	for i := 0; rs.ShouldTry(ctx, i); i++ {
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

The package contains strategies for retrying operations.

## ExponentialBackoff

This strategy will retry an operation up to a specified maximum number of times,
with an increasing delay between each retry. 
The delay doubles with each failed attempt. 
This gives the system more time to recover from any issues.

## Jitter

This strategy will also retry an operation up to a specified maximum number of times,
but adds a random variation to the backoff time. 
This helps distribute retry attempts evenly over time and reduces the risk of overwhelming the system.
