# frameless/adapters

Adapters are an important concept in the hexagonal architecture.
Adapters provide a way for the application to communicate with external systems
such as databases, web services, or user interfaces.
Adapters are used to implement these ports,
which provide a way for the application to receive input and send output to external systems.

By using adapters, the hexagonal architecture promotes loose coupling between the application's core logic
and external systems, making it easier to test and maintain the application.
Additionally, adapters can be easily swapped out or replaced without affecting the core logic of the application.

The `frameless/adapters/memory` implements all the `frameless/ports` making it great for using it in unit testing.
