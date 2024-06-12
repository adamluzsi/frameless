# framless/pkg

There are two types of addon packages in `frameless`. 
Utility or kit type of package, which meant to support the work of other packges, but itself doesn't create a new value for development.
Tools, 

## Tools

- `tasker`: very powerful task management tool tha enables you to implement easily graceful shutdown into you application all by simply listening to context cancellations.
  - built-in support to use HTTP Server with graceful shutdown
  - support for running tasks concurrently, and managing signaling between them if one part of the system goes down unexpectedly.
  - minimalistic API for creating tasks in your system
  - task scheduling support similar to cron
- `txs`: which allow defining rollback steps in your functions, which makes implementing error handling in a stateful system much 
  - makes rollback part of all function path without the need to develop and maintain a cleanup function for cases where something went wrong with your application's use-case.
  - while it is not a replacement for OnePhaseCommitProtocol solutions, it can help during an error with cleaning up one or more resource that doesn't support transactions on its own.
- `cache`: battle tested caching implementation that allows you to decorate your crud based role interfaces with passthrough caching.
- `logger`: centralised version of the logging package
  - flexible logging that uses the context to pass around logging context related details
  - Can be easily configured to use any logger library.
  - It promotes logging as a singleton application level entity, since majority of the time the application only meant to log to a single logging output
- `devops`: tooling that meant to help improve your application's operationability. It is not a replacement for tools such as Prometheus, but more like an addition to it.
  - `devops/health`: enables you to create a /health endpoint that can dramatically speed up the investigation process during an outage, by providing insights about what part of the system having an issue, similarly to an X-Ray.
- `restapi`: restapi enables you to create restful APIs with ease in a standardised way
- `retry`: retry package that enables you to implement retry mechanism in your tool with various retry strategy.
- `serializers`: contains streaming implementation. 
  - For e.g.: you can consumer or produce jsons streams (application/json) that contain a list, without the need to have every list element in the memory at once.

## Kits

- `iokit`: the missing functionality from `io` package, such as reading all with limit, or reading an io with a keep alive heart beat on it.
- `errorkit`: very powerful error utility package
- `env`: tooling to make working with Environment variables ease, including populating configuration structures
- `contextkit`: makes working with context even easier
- `chankit`: add tooling to channels such as merging multiple channels into a single one
- `containers`: generic container type structure implementation
- `convkit`: enables you to parse strings with ease through a simplified API
- `dtos`: a simplified DataTransferObject mapping API that makes working with DTOs easy in your external gateway layer
- `enum`: a simple enum implementation
- `logging`: Logging implementation to enable structured logging (before slog) and async logging
- `mk`: A package that helps making recursive initialization of Go structures easy. Works like `new` but with the added benefit of calling the `Init` function on it.
- `pointer`: very small library to make working with pointers convinent, mostly to have one-liner syntax sugar for common boilerplates
- `reflectkit`: extra tooling when you need to work with reflection
- `units`: minimalist package that contains commonly used units such as `Megabyte` and such
- `stringcase`: enables to you convert the "case" of string. For example to convert a hashmap key from snake_case to PascalCase
- `teardown`: building brick to enable the creation of teardown functionality
- `zerokit`: makes working with zero values convinent

### network

- `pathkit`: http request path building helper functions to make concating, joining or splitting http URLs or request paths convinent
- `httpkit`: http related tooling
- `netkit`: (small) helper function collection to make working with networking easier, such as finding an open port, or checking if a port is in use (linux/darwin)

### transformation

- `mapkit`: tools to make working with maps easier
- `slicekit`: tools to make working with slices easier
