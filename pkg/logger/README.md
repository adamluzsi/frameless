# package `logger`

Package `logger` represents logging as a global resource, as most application log is meant to happen to the same place.
The `logger` package makes it convinent to write your application that does application logging, that doesn't get into your way in your tests.
With `logger`, you can use context to add logging details to your call stack.
`logger` is based on `pkg/logging` but it can be configured to use any third party logging library thourgh providing a Hijack function.

## How to use

```go
package main

import (
	"context"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
)

func main() {
	ctx := context.Background()

	// You can add details to the context; thus, every logging call using this context will inherit the details.
	ctx = logging.ContextWith(ctx, logging.Fields{
		"foo": "bar",
		"baz": "qux",
	})

	// You can use your own Logger instance or the logger.Default logger instance if you plan to log to the STDOUT. 
	logger.Info(ctx, "foo", logging.Fields{
		"userID":    42,
		"accountID": 24,
	})
}

```

> example output when snake case key format is defined (default):

```json
{
  "account_id": 24,
  "baz": "qux",
  "foo": "bar",
  "level": "info",
  "message": "foo",
  "timestamp": "2023-04-02T16:00:00+02:00",
  "user_id": 42
}
```

> example output when camel key format is defined:

```json
{
  "accountID": 24,
  "baz": "qux",
  "foo": "bar",
  "level": "info",
  "message": "foo",
  "timestamp": "2023-04-02T16:00:00+02:00",
  "userID": 42
}
```

## How to configure:

`logging.Logger` can be configured through its struct fields; please see the documentation for more details.
To configure the default logger, simply configure it from your main.

```go
package main

import "go.llib.dev/frameless/pkg/logger"

func main() {
	logger.Default.MessageKey = "msg"
	logger.Default.TimestampKey = "ts"
}
```

## Key string case style consistency in your logs

Using the logger package can help you maintain historical consistency in your logging key style
and avoid mixed string case styles.
This is particularly useful since many log collecting systems rely on an append-only strategy,
and any inconsistency could potentially cause issues with your alerting scripts that rely on certain keys.
So, by using the logger package, you can ensure that your logging is clean and consistent,
making it easier to manage and troubleshoot any issues that may arise.

The default key format is snake_case, but you can change it easily by supplying a formetter in the KeyFormatter Logger
field.

```go
package main

import (
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/stringkit"
)

func main() {
	logger.Default.KeyFormatter = stringkit.ToKebab
}

```

## Security concerns

It is crucial to make conscious decisions about what we log from a security standpoint
because logging too much information can potentially expose sensitive data about users.
On the other hand, by logging what is necessary and relevant for operations,
we can avoid security and compliance issues.
Following this practice can reduce the attack surface of our system and limit the potential impact of a security breach.
Additionally, logging too much information can make detecting and responding to security incidents more difficult,
as the noise of unnecessary data can obscure the signal of actual threats.

The logger package has a safety mechanism to prevent exposure of sensitive data;
it requires you to register any DTO or Entity struct type with the logging package
before you can use it as a logging field.

```go
package mydomain

import "go.llib.dev/frameless/pkg/logger"

type MyEntity struct {
	ID               string
	NonSensitiveData string
	SensitiveData    string
}

var _ = logger.RegisterFieldType(func(ent MyEntity) logging.LoggingDetail {
	return logging.Fields{
		"id":   ent.ID,
		"data": ent.NonSensitiveData,
	}
})

```

## ENV based Configuration

You can set the default's loggers level with the following env values:

- LOG_LEVEL
- LOGGER_LEVEL
- LOGGING_LEVEL

| logging level | env value | env short format value |
|---------------|-----------|------------------------|
| Debug Level   | 	debug    | d                      | 
| Info Level    | 	info     | i                      | 
| Warn Level    | 	warn     | w                      | 
| Error Level   | 	error    | e                      | 
| Fatal Level   | 	fatal    | f                      | 
| Fatal Level   | 	critical | c                      | 

## Logging performance optimisation by using an async logging strategy

If your application requires high performance and uses a lot of concurrent actions,
using an async logging strategy can provide the best of both worlds.
This means the application can have great performance that is not hindered by logging calls,
while also being able to observe and monitor its behavior effectively.

| ----------------- | no concurrency | heavy concurrency |
|-------------------|----------------|-------------------|
| sync logging      | 5550 ns/op     | 54930 ns/op       |
| async logging     | 700.7 ns/op    | 1121 ns/op        |

> tested on MacBook Pro with M1 when writing into a file
>
> $ go test -bench BenchmarkLogger_AsyncLogging -run -

```go
package main

import (
	"go.llib.dev/frameless/pkg/logger"
)

func main() {
	defer logger.AsyncLogging()() // from here on, logging will be async

	logger.Info(nil, "this is logged asynchronously")
}
```

## Helping the TDD Flow during Testing

using the `logger.LogWithTB` helper method will turn your logs into structured testing log events
making it easier to investigate unexpected behaviours using the DEBUG logs.


> myfile.go

```go
package mypkg

import (
	"go.llib.dev/frameless/pkg/logger"
)

func MyFunc() {
	logger.Debug(nil, "the logging message", logging.Field("bar", 24))
}
```

> myfile_test.go

```go
package mypkg_test

import (
	"testing"

	"mypkg"

	"go.llib.dev/frameless/pkg/logger"
)

func TestMyFunc(t *testing.T) {
	logger.LogWithTB(t) // logs are redirected to use *testing.T.Log  

	mypkg.MyFunc()
}

```

will produce the following output:

```text
=== RUN   TestLogWithTB_spike
	myfile.go:11: the logging message | level:debug bar:24
	--- PASS: TestLogWithTB_spike (0.00s)
	PASS
```
