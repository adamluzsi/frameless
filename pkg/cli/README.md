# CLI

The `cli` package meant to give you tooling in building command line interface applications.

It also purposefully use an package convention that makes it familiar to the HTTP package's interface.

Terminology similarities between HTTP and CLI:

| HTTP                    | CLI                       | desc                                                                               |
| ----------------------- | ------------------------- | ---------------------------------------------------------------------------------- |
| request path            | command name in args      | defines what handler/command the caller wishes to reach                            |
| request path parameters | command arguments in args | endpoint specific parameters                                                       |
| request body            | STDIN                     | contains the user input data payload                                               |
| response body           | STDOUT                    | the channel in which the application replies back to the caller                    |
| request query string    | flags                     | interaction related meta data or modifiers that expect the affect to be altered    |
| request headers         | env variables             | ~same~                                                                             |
| status code             | exit code                 | code that notifies the caller if request succeeded or failed                       |
| request cancellation    | OS Signal interrupt       | an idiom to notify the software that the response no longer expected by the caller |
