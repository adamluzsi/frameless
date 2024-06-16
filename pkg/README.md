# frameless/pkg

- **`tasker`**: A task management tool enabling graceful shutdowns via context cancellations.
  - Supports HTTP Server graceful shutdowns.
  - Manages concurrent tasks and signalling.
  - Minimalistic API for task creation.
  - Task scheduling similar to cron.

- **`txkit`**: Defines rollback steps where native commit protocols are lacking.
  - Integrates rollbacks into all functions without separate cleanup functions.
  - Assists in error handling for resources without transaction support.

- **`cache`**: A robust caching implementation for CRUD interfaces with passthrough caching.

- **`logger`**: A centralised logging package.
  - Flexible logging using context for details.
  - Easily configured with any logger library.
  - Promotes application-level singleton logging.

- **`devops`**: Improves application operability, complementing tools like Prometheus.
  - **`devops/health`**: Creates a /health endpoint for outage investigations.

- **`httpkit`**: Provides HTTP-related tools.
  - Enables the creation of standardised RESTful APIs.
  - RFC7807 error format support

- **`retry`**: Implements various retry strategies in your tools.

- **`serializers`**: Stream-based implementations.
  - E.g., consumes or produces JSON streams without loading all elements into memory.

- **`env`**: Simplifies working with environment variables and populating config structures.

## Utility

- **`iokit`**: Adds missing functionalities to `io`, like reading with limits and keep-alive heartbeats.
- **`errorkit`**: A powerful error utility package.
- **`contextkit`**: Makes context handling easier.
- **`chankit`**: Adds tools for channel operations, like merging channels.
- **`containers`**: Implements generic container structures.
- **`convkit`**: Simplifies string parsing with an easy API.
- **`dtokit`**: Simplifies Data Transfer Object mapping for external gateways.
  - **`jsondto`**: Facilitates marshaling and unmarshaling of interface types.
- **`enum`**: A simple enum implementation.
- **`logging`**: Enables structured and asynchronous logging.
- **`mk`**: Facilitates recursive initialization of Go structures with an `Init` function.
- **`pointer`**: Makes pointer operations convenient with one-liner syntax.
- **`reflectkit`**: Adds extra tools for reflection.
- **`units`**: Contains commonly used units like `Megabyte`.
- **`stringcase`**: Converts string cases, e.g., snake_case to PascalCase.
- **`teardown`**: Facilitates teardown functionality.
- **`zerokit`**: Simplifies working with zero values.

## Network

- **`pathkit`**: Assists with HTTP request path operations.

- **`netkit`**: Contains networking helpers, like finding open ports or checking port usage (Linux/Darwin).

## Transformation

- **`mapkit`**: Tools for easier map operations.
- **`slicekit`**: Tools for easier slice operations.