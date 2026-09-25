# `workflow`

A lightweight, code-first workflow engine for Go.

- [Glossary][GLOSSARY]
- [Definition][DEFINITION]
- [Custom Definition][CUSTOM_DEFINITION]
- [Participant][PARTICIPANT]
- [Signals][SIGNAL]
- [Condition][CONDITION]
- [Variables][VARIABLES]
- [Codec][CODEC]
- [Workflow Builder Actor][END_USER]
- [local development testing support][TESTING]

--- 

[**Getting Started Guide**](./docs/getting-started.md)

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

---

## Example

[check out the examples file here](./example_test.go)

### Definition

```go
_ = workflow.Sequence{
	workflow.SetVar{Name: "topic", Value: "go.llib.dev/frameless"},
	workflow.Execute{
		ParticipantID: "summarise",
		Input:         []workflow.VarName{"topic"},
		Output:        []workflow.VarName{"summary", "found"},
	},
	workflow.If{
		Cond: workflow.Execute{
			ConditionID: "is-publishable",
			Input:       []workflow.VarName{"found"},
		},
		Then: workflow.Execute{
			ParticipantID: "publish",
			Input:         []workflow.VarName{"summary"},
		},
	},
}
```

## Idempotent execution

Successful [`Participant`][PARTICIPANT] calls and [`Condition`][CONDITION] answers are
cached in `workflow.EventRepository`. `workflow.Execute` is the entry point for
both: with a `ParticipantID` it is a step, with a `ConditionID` it is a
condition. A custom definition can cache its own logic the same way through
`Execute#ExecuteWith`, under a `ParticipantID` it passes, without registering a
participant. Recording an answer is
the condition's job: `Execute` and `wftemplate.Condition` do it, and a custom
condition can do the same through `Execute#EvaluateWith`. A `Sleep` asks its
condition anew on every attempt, under the second the attempt happens in, and
records its wake-up as an `EventSleepCompleted`, so a woken `Sleep` is passed on
replay.

When a matching result exists in the durable event history, replay reuses it
instead of calling the underlying function again. A cached participant that
returned a definition still walks that follow-up definition, so suspended work
can resume using its steps' recorded results.

**This is not exactly-once execution of external effects.** A remote operation
may succeed before its event is durably recorded; an event write or commit
failure, crash, or enclosing transaction rollback can cause the call to repeat.
Use stable business idempotency keys and retryable operations, or a shared
transaction/outbox where applicable.

Splitting work into top-level `Sequence` steps gives separate event transaction
boundaries. A participant-returned `Sequence` still runs inside the producer's
transaction: an ordinary nested failure can roll back the producer's event and
earlier nested success events. It is not a substitute for independent top-level
commit boundaries. See [Participants][PARTICIPANT] for details.


---

## What the runtime asks of you

The `workflow` package follows strictly the SOLID principles,
and therefore it uses role interfaces to express the external dependencies.

You can run the entire workflow engine on your own choice of attached services,
be it a lightweight on-device SQLite solution,
or a full blown `Kafka`/`RabbitMQ` setup,
or just something between with a jack-of-all-trade `PostgreSQL`.

It is your choice, `workflow` is a code first tool, not a resource dependent solution.
However, whatever will be your choice, you need to be compliant
with contracts defined in the built-in `wfcontract` package.
But these tests are pre-written for you using interface testing suites (contract testing).

| Field            | Purpose                                                  | Cheap option                                 |
| ---------------- | -------------------------------------------------------- | -------------------------------------------- |
| `Participants`   | Resolves a `Participant` by `ID`.                        | `workflow.Participants` (an in-process map). |
| `Conditions`     | Resolves a `Condition` by `ID`.                          | `workflow.Conditions` (an in-process map).   |
| `Events`         | The event source                                         | Your app's existing relational DB.           |
| `Queue`          | Schedules a `ProcessID` for execution.                   | The same DB, or queue you already have.      |
| `Notifications`  | Notifies worker nodes that the queue changed.            | Any fan-out exchange you already have.       |
| `Locks`          | Mutual exclusion per process.                            |                                              |
| `Codec`          | Polymorphic (de)serialisation of definitions and events. | `wfjson.NewCodec()`                          |
| `RetryStrategy`  | Wraps retryable execution failures in a retry loop.       | `pkg/resilience`.                            |
| `ParticipantWarningInterval` | Warns periodically while waiting for a compatible participant. | Default: 5 failures; values ≤ 0 use the default. |
| `ContextSetup`   | execution context decoration (logging, tracing, etc.).   |                                              |

---

## Replay and migration

Because state is the event log:

- **Replay.** `Runtime.Execute` reads the latest `EventUseDefinition` and
  re-runs the bound definition. The idempotent executors short-circuit
  already-recorded calls while walking their recorded follow-up definitions.
  Work without a durable success record may run again.
- **Migrations.** Ship a new definition by writing a new
  `EventUseDefinition` for an existing `ProcessID` (e.g. from a
  `workflow.Replace{Definition}` signal or a one-shot migration tool). Old
  events stay in the log for auditability; new executions pick up the new
  definition.
- **Undoing work.** Committed history is never rewritten, so undoing moves
  forward: `Replace` the definition with one that compensates, such as a
  refund, and continues from there. A transaction rollback only discards
  events that were never committed.

There is no separate "workflow version" concept — versions are just the
sequence of `EventUseDefinition` entries on a process. A bound definition is
never edited in place: a change is always a new binding, which re-roots every
step path, and the `EventRepository` port is append-only by design. See
[Definitions][DEFINITION] §5.

`wftemplate.Condition` answers used to be evaluated live, so older histories
hold no recorded template answer. The first post-upgrade evaluation at each
template position uses current scoped variables, not a reconstructed past
branch, and that answer is replayed from then on. Explicitly migrate affected
processes where that would be unsafe; see [Conditions][CONDITION].

---

## [TODO]

- [ ] optimise suspending with a suspended repository
  - continuous requeueing can be exhausting to the system if the `workflow.Suspend` feature is heavily used
- [ ] add non-testcase specific helper functions to `wftest`

[GLOSSARY]: ./docs/glossary.md
[DEFINITION]: ./docs/definition.md
[CUSTOM_DEFINITION]: ./docs/custom-definition.md
[PARTICIPANT]: ./docs/participant.md
[CONDITION]: ./docs/condition.md
[VARIABLES]: ./docs/vars.md
[CODEC]: ./docs/codec.md
[END_USER]: ./docs/end-user.md
[TESTING]: ./docs/testing.md
[SIGNAL]: ./docs/signal.md
