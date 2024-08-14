# frameless/port

- [CRUD](./crud/README.md)

Frameless simplifies your software development process by promoting conventions over configurations,
llowing you to concentrate on crucial aspects instead of repeatedly starting from scratch.

It offers Hexagonal architecture-based ports as technology-agnostic components for your domain code,
which are conventions that follow specific signatures for expected actions.
These signatures and ports in Frameless have undergone numerous iterations
and evaluations to achieve a sustainable and practical design solution.

Each hexagonal port in Frameless comes with its interface testing suite, known as a contract.
This approach guarantees that once you are familiar with a specific port's behaviour,
you can expect consistent behaviour from various implementations for that port since they all follow the same contract.
Consequently, it's easier for new team members to learn the concepts and build extra tools on these solid foundations.
Additionally, this approach reduces the mental effort needed to comprehend domain code.

> For instance, by employing the repository pattern with CRUD ports for handling side effects,
> you can effortlessly create generic decorators like caching, retrying, measuring observability,
> injecting faults, and more.
> These enhancements can be done without being concerned about the specific implementation details.
> For a concrete example, check out the frameless/pkg/cache package

Consider these contracts as Test-Driven Development (TDD) supercharged.
While writing tests alongside your implementation,
you can combine your expectations with these behaviour-focused tests at the domain level.
This approach allows the domain layer to define the final behaviour expected from the implementations.

As a side effect of this approach, you'll experience an unmatched level of architectural flexibility,
allowing you to experiment with, replace, or remove adapter implementations as needed.

[For a more inept explanation with examples, please check out this documentation.](intro.md)
