# Postgresql adapters for [frameless](https://go.llib.dev/frameless)

Welcome to the PostgreSQL adapters for frameless!

This package provides a set of adapters that allow you to use PostgreSQL database in your application through the frameless ports.

## Features

* Repository implementation for CRUD operations (Create, Read, Update, Delete)
* Shared Locker implementation for locking across application instances
* Message queueing system with publish/subscribe functionality
* Support for transactional queries using the `postgresql.Connection`

## Example Usage

Here are some brief examples of how to use this package:

```go
repo := postgresql.Repository[domain.Ent, domain.EntID]{...}

// Create an entity in the repository
err := repo.Create(ctx, &ent)

// Find an entity by ID in the repository
ent, found, err := repo.FindByID(ctx, id)

// Update an entity in the repository
err := repo.Update(ctx, &ent)

// Delete an entity from the repository
err := repo.DeleteByID(ctx, id)

// Publish a message to a queue
err := queue.Publish(ctx, msg)

// Subscribe to a queue and receive messages
it, err := queue.Subscribe(ctx)
for it.Next() {
    msg := it.Value()
    // process message
}
```

## Getting Started

To get started with this package, simply import it into your Go project:

```go
import "go.llib.dev/frameless/adapter/postgresql"
```

Take a look at the documentation for more information on how to use each feature.

We hope you find this package useful! If you have any questions or issues, please don't hesitate to reach out.

## Tasker Integration

This package also provides an implementation for the `frameless/pkg/tasker` package, allowing you to store and manage scheduled tasks in a PostgreSQL database.
