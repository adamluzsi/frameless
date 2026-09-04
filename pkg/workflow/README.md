# `workflow`

A lightweight, code-first workflow engine for Go.

- [Glossary][GLOSSARY]
- [Definition][DEFINITION]
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
	workflow.ExecuteParticipant{
		ID:     "summarise",
		Input:  []workflow.VarName{"topic"},
		Output: []workflow.VarName{"summary", "found"},
	},
	workflow.If{
		Cond: workflow.ExecuteCondition{
			ID:    "is-publishable",
			Input: []workflow.VarName{"found"},
		},
		Then: workflow.ExecuteParticipant{
			ID:    "publish",
			Input: []workflow.VarName{"summary"},
		},
	},
}
```

## Idempotent execution

Every [`Participant`][PARTICIPANT], [`Condition`][CONDITION] execution is cached and made idempotent through the utilisation of the `workflow.EventRepository`.

If a result exists in the process' event history,
the runtime will replay it from there instead of running the underlying function again.
This makes retries, crashes, and double-delivery safe by default.
But it does not shield you from incorrect implementation.

If you create a new identifier as part of a participant execution,
and then you make a request with it, but it fails,
and you expect that the same identifier to be retried,
then you probably better of sending back a workflow.Sequence,
which contain the steps which each should be safe to retry in case of a failure.


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

| Field                    | Purpose                                                  | Cheap option                                 |
| ------------------------ | -------------------------------------------------------- | -------------------------------------------- |
| `Participants`           | Resolves a `Participant` by `ID`.                        | `workflow.Participants` (an in-process map). |
| `Conditions`             | Resolves a `Condition` by `ID`.                          | `workflow.Conditions` (an in-process map).   |
| `Events`                 | The event source                                         | Your app's existing relational DB.           |
| `ProcessExecutionQueue`  | Schedules a `ProcessID` for execution.                   | The same DB, or queue you already have.      |
| `ProcessChangeBroadcast` | Notifies worker nodes that the queue changed.            | Any fan-out exchange you already have.       |
| `ProcessLockers`         | Mutual exclusion per process.                            |                                              |
| `Codec`                  | Polymorphic (de)serialisation of definitions and events. | `wfjson.NewCodec()`                          |
| `RetryStrategy`          | Wraps execution in a retry loop.                         | `pkg/resilience`.                            |
| `ContextSetup`           | execution context decoration (logging, tracing, etc.).   |                                              |

---

## Replay and migration

Because state is the event log:

- **Replay.** `Runtime.Execute` reads the latest `EventUseDefinition` and
  re-runs the bound definition. The idempotent executors short-circuit
  already-recorded steps, so replay converges to the latest persisted state
  without re-doing work.
- **Migrations.** Ship a new definition by writing a new
  `EventUseDefinition` for an existing `ProcessID` (e.g. from a
  `workflow.Replace{Definition}` signal or a one-shot migration tool). Old
  events stay in the log for auditability; new executions pick up the new
  definition.
- **Rollback.** Delete or rewrite events to roll a process back to a prior
  state. The same replay loop will then recompute from there.

There is no separate "workflow version" concept — versions are just the
sequence of `EventUseDefinition` entries on a process.

---

## [TODO]

- [ ] optimise suspending with a suspended repository
  - continuous requeueing can be exhausting to the system if the `workflow.Suspend` feature is heavily used
- [ ] add new runtime signal: `workflow.Halt`
  - the ability to halt completely a process, without any rescheduling could be valuable in various system designs
- [ ] add non-testcase specific helper functions to `wftest`

[GLOSSARY]: ./docs/glossary.md
[DEFINITION]: ./docs/definition.md
[PARTICIPANT]: ./docs/participant.md
[CONDITION]: ./docs/condition.md
[VARIABLES]: ./docs/vars.md
[CODEC]: ./docs/codec.md
[END_USER]: ./docs/end-user.md
[TESTING]: ./docs/testing.md
[SIGNAL]: ./docs/signal.md
