# Health Package

The `health` package is designed to help you build a health check easily for your Go application.
It includes functionality for checking the status of various components, reporting issues,
and collecting health related metrics.

It allows you to expose this through an HTTP API.
Additionally, it enables you to gather health status information from your dependent services in a cascading manner.
This means that when your application encounters issues,
the health check endpoint can quickly identify the root cause,
saving you time and effort in troubleshooting.

## Features

* Easy-to-use health check endpoint
* Support for registering checks and dependencies
* Customizable health state messages
* Reporting of issues and errors during health checks
* HTTP handler for serving health check responses

## Usage

To use the `health` package, you will need to import it into your Go application:

```go
package main

import "go.llib.dev/frameless/pkg/devops/health"

```

You can then create a new instance of the `Monitor` and register checks, dependencies and metrics as needed.

For example:

```go
package main

import (
	"context"
	"database/sql"
	"go.llib.dev/frameless/pkg/devops/health"
	"net/http"
	"sync"
)

const metricKeyForHTTPRetryPerSec = "http-retry-average-per-second"

func healthCheckMonitor(appMetrics *sync.Map, db *sql.DB) health.Monitor {
	return health.Monitor{
		// our service related checks
		Checks: []health.CheckFunc{
			func(ctx context.Context) error {
				value, ok := appMetrics.Load(metricKeyForHTTPRetryPerSec)
				if !ok {
					return nil
				}
				averagePerSec, ok := value.(int)
				if !ok {
					return nil
				}
				if 42 < averagePerSec {
					return health.Issue{
						Causes: health.Degraded,
						Code:   "too-many-http-request-retries",
						Message: "There could be an underlying networking issue, " +
							"that needs to be looked into, the system is working, " +
							"but the retry attemt average shouldn't be so high",
					}
				}
				return nil
			},
		},
		// our service's dependencies like DB or downstream services
		Dependencies: health.MonitorDependencies{
			func(ctx context.Context) health.Report {
				var hs health.Report
				err := db.PingContext(ctx)

				if err != nil {
					hs.Issues = append(hs.Issues, health.Issue{
						Causes:  health.Down,
						Code:    "xy-db-disconnected",
						Message: "failed to ping the database through the connection",
					})
				}

				// additional health checks on this DB connection

				return hs
			},
		},
	}
}

```

Once you have registered your checks and dependencies,
you can use the `Monitor` struct's `HTTPHandler` method to create an HTTP handler for serving health check responses.

For example:

```go
http.Handle("/health", monitor.HTTPHandler())
```

## Testing

The `health` package includes a suite of tests that demonstrate its functionality.
These tests cover various scenarios, such as when all checks and dependencies pass,
when one check or dependency fails, and when there are issues during the health check evaluation process.

To run the tests, you can use the `go test` command in the `pkg/devops/health` directory:

```sh
$ go test
```

The output of running the tests is included above and demonstrates the various scenarios that are covered.
