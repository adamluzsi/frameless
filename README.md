# frameless

- [GoDoc](https://pkg.go.dev/go.llib.dev/frameless)
- [Architecture decision records](docs/ADR)

Discover `frameless`, the 🇨🇭Swiss Army Knife of Hexagonal Architecture conventions!

Frameless is essential to unleashing your software development process's true capabilities.
It assists in making your application scalable, adaptable, and easy to maintain
by streamlining your project's design and managing code complexity effectively.

Frameless simplifies your software development process by promoting conventions over configurations,
allowing you to concentrate on crucial aspects instead of repeatedly starting from scratch.

Like the iconic Swiss Army Knife, `frameless` is a versatile and indispensable tool ready to tackle any challenge,
enabling you to adapt and thrive in the ever-changing software development landscape.

[`frameless` adopt a monolithic module design reminiscent of the Linux kernel.](docs/ADR/monolithic-project-structure.md)
However, this design choice is not related to monolithic software architecture;
instead, it resembles how mono repositories assist code owners in managing their systems.

`frameless` is made up of three high-level components.

## port

`port` are plain interfaces to make expressing a domain [role interface][:role-interface:] straightforward.
Using ports doesn't have any vendor locking effect. 
You can use almost every port by just copy their functions 
into your [role interface][:role-interface:] in your domain layer. 

Each port interface has its importable contract(s) that help you ensure
that the behaviour is consistent across the implementations.

Some port package contain a small amount of optional helper functions to help streamlining their use.

There is also testing packages to make it easy to write behaviour-driven tests against your implementations
when you need to specify additional expectations towards them from your domain layer.

Using `frameless/port` safe from vendor locking as they just a collection of finely refined inter

## [pkg](pkg/README.md)

Various tooling built either upon using frameless/port or supplies tools often required to develop web services.

- Some notable examples:
    - [`logger` for structured logging](pkg/logger/README.md)
    - [`tasker` for managing background tasks](pkg/tasker/README.md)
    - [`cache` for wrapping repositories (DB, HTTP clients, etc.) with caching to increase reliability and/or speed in the domain layer](pkg/cache/README.md)
    - [`restapi` for building restful HTTP APIs and/or exposing repositories as rest resources.](pkg/restapi/README.md)
      - `restapi/rfc7807` for replying back errors on your API in a structure and extendable way. 
    - `enum` for tag definition based enum value validation
    - `lazyload` to utilise lazy loading techniques
    - `errorutil` to help you work with errors, forward port some features, and make distinction between errors based on their SRP actor.
    - `txs` which allow defining rollback steps in your functions, which makes implementing error handling in a stateful system much easier

## [adapters](adapters/README.md)

`adapters` has Example implementations for the `ports`, especially the `memory` package,
which enables you to do a classicist Test-Driven Development (TDD) testing strategy.

When you import `frameless`, adapters are not automatically imported into your project; you must import them
explicitly. This approach helps maintain a lean and tidy dependency graph while working with `frameless`.

[:role-interface:]: https://martinfowler.com/bliki/RoleInterface.html#:~:text=A%20role%20interface%20is%20defined,of%20these%20patterns%20of%20interaction
[:mono-module-struct-adr:]: docs/ADR/monolithic-project-structure.md
