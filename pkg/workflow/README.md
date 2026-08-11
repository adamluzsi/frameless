# `workflow`

A lightweight, code-first workflow engine for Go.

## [TODO]

- [ ] optimise suspending with a suspended repository
  - continuous requeueing can be exhausting to the system if the `workflow.Suspend` feature is heavily used
- [ ] add new runtime signal: `workflow.Halt`
  - the ability to halt completely a process, without any rescheduling could be valuable in various system designs
- [ ] add non-testcase specific helper functions to `wftest`

## **Primary goals**:

- enable a system's end-users to do workflow composition for themselves.
  - Serializable workflow definitions, store them in your system however you like
  - transfer them back and forth between your Front-End and Back-End
- highly fault tolerant workflow executions
  - explicit test coverage to ensure workflow system resilience
- conditionally suspend and offload workflows until they can be continued
- spawn sub workflows and idiomatically join them back
  - built-in concurrency idioms in the workflow engine
- code-first workflow engine API design
  - definitions are ordinary Go values
  - participants are ordinary Go functions
  - unit testing is fully possibly in-memory as part of your go project's testing suite
- event sourcing based state management
  - easy to replay, debug, reconstruct, analyse
  - auditable, append only approach
  - easy to optimise for large quantity in your storage due to its timeseries nature
    - for example if you use PostgreSQL, then you can use BRIN index
- independent from its attached dependencies
  - the workflow engine is not dependent on a particular external resource, you are free to use the tools that you are the most familiar, or your system already using it
  - reduce system complexity through being possible to reuse already utilised resources
- easy of testability
  - comes with a testing package (`wftest`), and due to its design principles, the workflow engine compositions are fully testable through in-memory unit-testing

## Glossary

- **Definition**
  - **maintained by the end-users**
  - a composite value that describes how a workflow execution should play out
  - Serialisable, can be stored in attached resources (repositories)
  - Idempotent, definitions by nature are idempotent in their execution
- **Participant**
  - **maintained by the developers**
  - domain logic provided by the system towards the workflow definition designers
    - for example, workflow definition builders are typically the end-users
  - These code paths are potentially non-idempotent
    - in case of an unexpected failure, they are expected to be repeatable
    - if the failure is final, and unrecoverable, `workflow.ErrFatal` could be returned to bypass retry attempts.
  - primary way to extend the available actionable options in a `workflow.Definition`
  - its execution is documented in the **Process**' event history
  - it can return a `workflow.Definition` to delegate a multi-step sub-flow to the engine,
    so each step is executed, and in case of failure retried independently.
- **Condition**
  - **maintained by the developers**
  - its purpose is to define complex conditional statements, usable by the definitions
  - akin to **Participants** it is a logic provided to workflow definition builders
- **Process**
  - **maintained by the workflow engine**
  - an instance of a workflow execution, identified by a `workflow.ProcessID` (UUID v7)
  - its state is the sum of its `workflow.Event`s
    - for example, it might have **Variables** in the form of `workflow.VarEvent` events.

---

## Example

[check out the exammples file here](./example_test.go)

### Definition

```go
_ = workflow.Sequence{
	workflow.SetVar{Key: "topic", Value: "go.llib.dev/frameless"},
	workflow.ExecuteParticipant{
		ID:     "summarise",
		Input:  []workflow.VarKey{"topic"},
		Output: []workflow.VarKey{"summary", "found"},
	},
	workflow.If{
		Cond: wftemplate.Condition(`eq .found true`),
		Then: workflow.ExecuteParticipant{
			ID:    "publish",
			Input: []workflow.VarKey{"summary"},
		},
	},
}
```

## built-in

### Definitions

| Type                          | Effect                                                                                                  |
| ----------------------------- | ------------------------------------------------------------------------------------------------------- |
| `workflow.Sequence`           | Run child definitions in order.                                                                         |
| `workflow.If`                 | Branch on a `Condition`.                                                                                |
| `workflow.Sleep`              | Pause until `While` becomes false or `Until` becomes true. Emits `Suspend{}` as a `RuntimeSignal`.      |
| `workflow.SetVar`             | Set a workflow variable (recorded as `SetVariableFromDefinitionEvent`).                                 |
| `workflow.ExecuteParticipant` | Call a registered participant by `ID`, with `Input`/`Output` variable mappings.                         |
| `workflow.ExecuteCondition`   | Call a registered condition by `ID`; usable as both `Condition` and `Definition`.                       |
| `workflow.Spawn`              | Launch a sub-workflow. Persistence is transactional so the child is visible atomically with the parent. |
| `workflow.Join`               | Wait for one or all spawned children to complete; optionally copy child variables back into the parent. |

### Conditions

Conditions return `(bool, error)`. Two flavours ship out of the box:

- `workflow.ExecuteCondition{ID, Input}` — call a registered condition
- `wftemplate.Condition` — Go `text/template` based expressions
  - evaluated against the current process variables.
  - Custom functions are added with `wftemplate.ContextWith(ctx, FuncMap{...})`.

### Variables

`ProcessVars(pid)` returns a `Vars` view backed by the event log.
The `workflow.Runtime` must be present in the context provided to the operations.

```go
ctx = rt.Context(ctx)
vars := workflow.ProcessVars(pid)
v, ok, err := vars.Lookup(ctx, "topic")
_ = vars.Set(ctx, "topic", "go")
m, err := vars.ToMap(ctx)
```

`Vars` is intentionally not a key-value store — it is a fold over `VarEvent` entries.
This means variables are first-class events that participate in replay and audit just like any other state change.

## Idempotent execution

Every `Participant`, `Condition` execution is cached and made idempotent through the utilisation of the `workflow.EventRepository`.

If a result exists in the process' event history,
the runtime will replay it from there instead of running the underlying function again.
This makes retries, crashes, and double-delivery safe by default.
But it does shield you from incorrect implementation.

If you create a new identifier as part of a participant execution,
and then you make a request with it, but it fails,
and you expect that the same identifier to be retried,
then you probably better of sending back a workflow.Sequence,
which contain the steps which each should be safe to retry in case of a failure.

### Runtime signals

A participant may return a `RuntimeSignal` (an `error` implementing
`RuntimeSignalExecute`). Signals are "happy errors":

- `workflow.Suspend` — pause and re-enqueue the process. Emitted by
  `workflow.Sleep` and `workflow.Join` when waiting on an external
  condition.
- `workflow.Complete` — explicitly mark the process as completed.
  Recorded as `EventCompleted`.
- `workflow.Replace{Definition}` — swap the process's current definition for
  a new one. Useful for migrations and human-in-the-loop flows.

Signals bubble up through the call stack and are handled by the runtime
itself before any other error is surfaced.

---

## What the runtime asks of you

The `Runtime` struct is a plain value composed of interfaces. There are no
constructor flags that pull in a database driver or a broker client. To run
the engine inside your application you supply:

| Field             | Interface                                                              | Purpose                                                                                | Cheap option                                       |
| ----------------- | ---------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | -------------------------------------------------- |
| `Participants`    | `ParticipantRepository`                                                | Resolves a `Participant` by `ID`.                                                      | `workflow.Participants` (an in-process map).       |
| `Conditions`      | `ConditionRepository`                                                  | Resolves a `Condition` by `ID`.                                                        | `workflow.Conditions` (an in-process map).         |
| `EventRepository` | `EventRepository`                                                      | Persists events; supports `BeginTx` / `CommitTx` / `RollbackTx` and `FindByProcessID`. | Your app's existing relational DB.                 |
| `ProcessQueue`    | `ProcessQueue`                                                         | Schedules a `ProcessID` for execution.                                                 | The same DB, or an in-memory queue.                |
| `ProcessLockers`  | `ProcessLockers` (`guard.LockerFactory[ProcessID, NonBlockingLocker]`) | Mutual exclusion per process.                                                          | An in-memory locker factory; Redis if distributed. |
| `Codec`           | `Codec`                                                                | Polymorphic (de)serialisation of definitions and events.                               | `wfjson.Codec{}` (provided).                       |
| `RetryStrategy`   | `resilience.RetryStrategy`                                             | Wraps execution in a retry loop.                                                       | The default in `pkg/resilience`.                   |
| `ContextSetup`    | `func(context.Context) context.Context`                                | Per-execution context decoration (logging, tracing, etc.).                             | Any function you want.                             |

You can run the entire engine on SQLite for development and prototyping,
then point `EventRepository` and `ProcessQueue` at Postgres in production
by changing two assignments. Kafka or RabbitMQ are valid options for the
queue if you outgrow the database-backed one. Nothing about the workflow
definitions or participants has to change between deployments — only the
backing service implementations do.

---

## Serialisation (`Codec`)

A `Codec` marshals and unmarshals `Definition`, `Condition`, and `Event`
polymorphically. The `wfjson` subpackage provides a JSON implementation
backed by `jsonkit`'s type-tagged envelopes.

A codec must faithfully round-trip every registered concrete type. The
`wfcontract.Codec` contract enforces this against any
`workflow.Codec`:

```go
testcase.RunSuite(t, wfcontract.Codec(myCodec))
```

---

## Replay and migration

Because state is the event log:

- **Replay.** `Runtime.Execute` reads the latest `UseDefinitionEvent` and
  re-runs the bound definition. The idempotent executors short-circuit
  already-recorded steps, so replay converges to the latest persisted state
  without re-doing work.
- **Migrations.** Ship a new definition by writing a new
  `UseDefinitionEvent` for an existing `ProcessID` (e.g. from a
  `workflow.Replace{Definition}` signal or a one-shot migration tool). Old
  events stay in the log for auditability; new executions pick up the new
  definition.
- **Rollback.** Delete or rewrite events to roll a process back to a prior
  state. The same replay loop will then recompute from there.

There is no separate "workflow version" concept — versions are just the
sequence of `UseDefinitionEvent` entries on a process.

---

## Testing

This package ships two test helpers you should prefer over bespoke test
scaffolding:

- `wfcontract` — contract suites that lock in behaviour for codecs,
  participants, conditions, definitions, etc.
- `wftest` — runtime fixtures (in-memory event repository, process queue,
  locker factory, codec, and helpers like `ProcessEvents`, `WaitForSpawn`,
  `ActExecute`, `ProcessCompletionIs`).

Use them whenever you add a new component that claims to implement a
`workflow` interface.
