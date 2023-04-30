# Adopting a Monolithic Module Design Structure in the `frameless` Project

## Context

The frameless project is a Go-based anti-framework that aims to provide a minimalist and flexible approach
to building a wide range of applications.
In designing the project's package structure, I considered various approaches,
including modular design with different git repositories,
but from maintainability and refactorability, they have significant added cost.

After analyzing the requirements and constraints of the project,
I decided to adopt a monolithic module design that resembles the Linux kernel a bit
or services made by a single team and put together in a mono-repository.

This approach organizes the code into a single, cohesive module that contains multiple sub-packages and sub-modules.

This design choice is not related to monolithic software architecture,
which involves building a single, large system with tightly-coupled components.

## Decision

I decided to adopt a monolithic module design in the frameless project for several reasons:

- Coherent Organization: By having a single, cohesive package that contains multiple sub-packages and modules,
  I can maintain a coherent organization of the codebase.
  This approach allows me to keep related functionality together
  and makes it easier to navigate and understand the codebase.
- Easy Maintenance: By having all ports, default adapters, and packages in a single module,
  I can easily maintain and update them together.
  This simplifies the overall maintenance of the project.
- Minimal Dependencies: Since none of the core packages in the frameless project depend on anything
  other than the standard library, adopting a monolithic module design
  should not cause issues with dependencies in projects that use frameless.

# Consequences

While adopting a monolithic module design has several advantages, it also has some potential drawbacks. One potential
issue is that the monolithic module may become too large and complex, making it difficult to navigate and maintain.
To mitigate this risk, I plan to maintain a clear structure and organization within the core module
and to use descriptive naming conventions for the sub-packages and sub-modules.

- ports/...
- adapters/...
- pkg/...

Another potential issue is that some developers may be unfamiliar with this package design
and may find it difficult to navigate and understand.
To address this, I plan to provide clear documentation and examples of how to use the package
and its sub-packages and modules.

Overall, I believe that adopting a monolithic module design is the best approach for the frameless project,
given its requirements and constraints.
This design choice will allow me to maintain a coherent organization of the codebase,
simplify maintenance and updates, and minimize dependencies.
