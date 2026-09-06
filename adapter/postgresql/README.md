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

## Workflow Integration

This package also provides implementations for the components required to back the `pkg/workflow` runtime from PostgreSQL:

| Component | Type | Purpose |
| --- | --- | --- |
| `WorkflowEventRepository` | `workflow.EventRepository` | Append-only event log |
| `WorkflowQueue` | `workflow.Queue` | Durable execution-request queue |
| `WorkflowNotificationBroadcast` | `workflow.NotificationBroadcast` | Cross-process notifications (PostgreSQL `LISTEN`/`NOTIFY`) |
| `WorkflowLockerFactory` | `workflow.ProcessLocks` | Per-process locks |

The runtime is built to be adapter-agnostic, so wiring these four pieces together produces a workflow engine that runs against any PostgreSQL database. See `Test_workflowE2E` for an end-to-end example.

### Storage decisions

The event log is intentionally minimal:

- **Append-only.** No updates, no deletes, no retention. Inserts only.
- **`PRIMARY KEY (process_id, event_id)`.** Heap-organized by `(process_id, event_id)` so `FindByProcessID` lands on a contiguous run for one process. This is the primary access path.
- **`UNIQUE INDEX (event_id)`.** Exists so any generator bug that emits a duplicate event ID is caught by the database. `FindByID` uses this index.
- **No auxiliary lookup table.** An earlier draft carried a `frameless_workflow_process_starts` table that routed `FindByProcessID` through a `first_event_id` lower bound. It was removed: the composite primary key already serves the hot path, the lower bound is almost always true for live processes (UUIDv7 is monotonic), and the table duplicated information that the primary key already enforced.
- **No BRIN index.** BRIN's unit of work is the block range (default 128 pages ≈ 1 MB). Per-process histories are far smaller than that and BRIN's selectivity on `process_id` would be poor — a process's rows are scattered across the table. BRIN is only useful for wide range scans, which `WorkflowEventRepository` does not run.
- **No partitioning.** Considered and deferred. The schema is forward-compatible: partitioning by `process_id` can be added without API changes when measured workload demands it. Hash partitioning would co-locate a process's rows, but it constrains the primary key (`event_id` would have to be local to the partition) and the primary access path is already index-driven, so the benefit is small until the table outgrows single-host indexes.
- **`occurred_at`, not `timestamp`.** `timestamp` is a parser-known keyword in PostgreSQL; tools that emit unquoted SQL trip on it. `occurred_at` is unambiguous.

The decision points above were chosen so that the table behaves correctly at small scale (the common deployment) without painting us into a corner at large scale. Anyone outgrowing the layout can replace the event repository with their own implementation — the runtime does not care.
