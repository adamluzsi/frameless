# Health Package

The `health` package is designed to help you build a health check easily for your Go application.
It includes functionality for checking the status of various components, reporting issues,
and collecting health related metrics.

It allows you to expose this through an HTTP API.
Additionally, it enables you to gather health status information from your dependent services in a cascading manner.
This means that when your application encounters issues,
the health check endpoint can quickly identify the root cause,
saving you time and effort in troubleshooting.

The health package also enables providing contextual information about the system, application,
and its dependencies in a human-readable format in our health check reports.
This allows for a quick overview of the system during an outage or partial outage,
helping identify problem's root cause efficiently.

By including metrics on the health check endpoint from dependent services, 
it becomes easier to correlate potential root causes for an outage. 
The health package is meant to be used by a human primarily and not meant to replace tools such as `Prometheus`.

The health package focuses on runtime information 
to provide contextual details about the system's dependencies and their statuses. 
By checking our systems state during runtime, it helps identify potential issues 
which affect your application's ability to serve requests normally on its API.

The health package is meant to assist in identifying and troubleshooting runtime issues during outages,
and not to verify if your application is correctly implemented.

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
		Checks: health.MonitorChecks{
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
			
			// registering a downstream serice as our dependency by using their /health endpoint 
			health.HTTPHealthCheck("https://downstreamservice.mydomain.ext/health", nil),
		},
	}
}

```

Once you have registered your checks and dependencies,
you can use the `Monitor` struct's `HTTPHandler` method to create an HTTP handler for serving health check responses.
The http response of the `Monitor`'s HTTP Handler is compatible with how `Kubernetes`' health check integration.

```go
http.Handle("/health", monitor.HTTPHandler())
```

> Here is an example, that represents a partial outage, 
> due to having the DB connection severed in our downstream service.

- we have a PARTIAL_OUTAGE status
  - message states that one or more dependencies are experiencing issues
- we check our dependencies for issues
  - downstream-service-name is in a PARTIAL_OUTAGE state as well
- we check the dependencies of downstream-service-name
  - we can see that xy-db is in a DOWN state.
- we reach out to the team responsible for xy-db 

```json
{
  "status": "PARTIAL_OUTAGE",
  "message": "The service is running, but one or more dependencies are experiencing issues.",
  "dependencies": [
    {
      "status": "PARTIAL_OUTAGE",
      "name": "downstream-service-name",
      "message": "The service is running, but one or more dependencies are experiencing issues.",
      "issues": [
        {
          "code": "xy-db-disconnected",
          "message": "failed to ping the database through the connection"
        }
      ],
      "dependencies": [
        {
          "status": "DOWN",
          "name": "xy-db",
          "timestamp": "0001-01-01T00:00:00Z"
        }
      ],
      "timestamp": "0001-01-01T00:00:00Z",
      "metrics": {
        "http-request-throughput": 42
      }
    }
  ],
  "timestamp": "2024-03-25T13:29:26Z",
  "metrics": {
    "metric-name": 42
  }
}
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
