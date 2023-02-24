# package `logger`

Package logger provides tooling for structured logging.
With logger, you can use context to add logging details to your call stack.

## How to use

```go
ctx := context.Background()

// You can add details to the context; thus, every logging call using this context will inherit the details.
ctx = logger.ContextWithDetails(ctx, logger.Details{
    "foo": "bar",
    "baz": "qux",
})

// You can use your own Logger instance or the logger.Default logger instance if you plan to log to the STDOUT. 
logger.Info(ctx, "foo", logger.Details{
    "userID":    42,
    "accountID": 24,
})
```

> example output:
```json
{"accountID":24,"baz":"qux","foo":"bar","level":"info","message":"foo","timestamp":"2023-02-24T02:11:33+01:00","userID":42}
```

## How to configure:

`logger.Logger` can be configured through its struct fields; please see the documentation for more details.
To configure the default logger, simply configure it from your main.


```go
func main() {
    logger.Default.MessageKey = "msg"
}
```
