---
name: frameless-workflow
description: Design, implement, integrate, migrate, and test replay-safe Go workflows with go.llib.dev/frameless/pkg/workflow. Use when working with workflow definitions (built-in and custom), participants, conditions, runtime wiring, event stores, scheduling, signals, codecs, or wftest/wfcontract. Applies to any Go project that imports the package.
compatibility: Go 1.25+; imports go.llib.dev/frameless/pkg/workflow.
license: MIT (matches go.llib.dev/frameless)
metadata:
  project: go.llib.dev/frameless
  package: pkg/workflow
  scope: Global
---

# Frameless Workflow

Use this skill for work involving `go.llib.dev/frameless/pkg/workflow` in any Go project — designing, implementing, integrating, migrating, or testing workflows. Treat the package as an **event-sourced, replaying workflow engine**, not as a conventional in-memory task runner.

## Start with the model

Keep these responsibilities separate:

| Concern | Owner | Form |
| --- | --- | --- |
| Capability / domain logic | Application developer | Registered Go `Participant` or `Condition` |
| Workflow composition | Builder or application | Serializable `workflow.Definition` value |
| Execution, replay, retries, scheduling | Engine integration | `workflow.Runtime` and its ports |

A process has a caller-owned, non-zero `workflow.ProcessID`. Its state is the append-only event history; it is not a mutable process-status object. The runtime may replay a process after a retry, crash, or duplicate delivery, so changes must converge when executed again for the same process ID.

Everything recorded is immutable, the bound definition included. Aim for appending, never editing:

- `Bind` never overwrites, and the `EventRepository` port has no `Updater` or `Saver`. Do not add update or delete paths, and do not edit stored events or definitions.
- To move a process to another definition, append a new `EventUseDefinition` with `Replace` or a migration tool. The root path segment is that binding's `EventID`, so the new definition's steps and recorded answers never collide with the old ones.
- To decide the shape of the work at run time, use a routing participant that returns a `workflow.Definition`. The call is recorded with that definition attached, and the definition runs in place of the step, extending the tree. On replay the producer is not called again: the recorded definition is walked under the original path, so it is as durable as the bound definition, and its finished steps are reused while unfinished ones resume.

Before changing behavior, identify whether the request concerns:

1. **a participant/condition** — a unit of domain behavior;
2. **a definition** — serializable composition and control flow (built-in or your own type);
3. **runtime infrastructure** — events, queue, locks, notifications, retry, or codec;
4. **a migration** — an already-bound definition or persisted event format;
5. **tests/contracts** — composition tests or an adapter implementation.

Read the relevant package documentation listed in [Documentation map](#documentation-map). When prose and implementation disagree, use the public source and its focused tests as the behavior to preserve, then update documentation if the task warrants it.

## Author definitions as durable data

A `workflow.Definition` is:

```go
type Definition interface {
    Execute(context.Context, workflow.ProcessID) error
    error
}
```

It is an ordinary Go value that the runtime may walk repeatedly, reusing durable outcomes from the event history while resuming unfinished work. Reach for built-ins first, then add your own when the catalogue does not express what you need.

### Built-in catalogue

| Group | Built-ins |
| --- | --- |
| Composition and control | `Sequence`, `If`, `Sleep`, `For`, `ForEach`, `Break` |
| Variables | `SetVar`, `DeclareVar`, `DeleteVar`, `Increment` |
| Application capabilities | `Execute` (a step with `ParticipantID`, a condition with `ConditionID`) |
| Child processes | `Spawn`, `Join` |

A normal composition wires variable names to capability inputs and results:

```go
workflow.Sequence{
    workflow.SetVar{Name: "order_id", Value: "ORD-42"},
    workflow.Execute{
        ParticipantID: "charge-card",
        Input:         []workflow.VarName{"order_id"},
        Output:        []workflow.VarName{"receipt"},
    },
    workflow.If{
        Cond: workflow.Execute{
            ConditionID: "requires-receipt",
            Input:       []workflow.VarName{"order_id"},
        },
        Then: workflow.Execute{
            ParticipantID: "email-receipt",
            Input:         []workflow.VarName{"receipt"},
        },
    },
}
```

`Input` and `Output` are **positional**: inputs map to function parameters after `context.Context`; outputs map to non-error results. They are not matched by variable-name text. Missing variables and arity/type mismatches must remain explicit errors, not panics.

Set exactly one ID on an `Execute`; it decides the role. Anything else fails with a fatal `ErrInvalidDefinition` before anything runs, including a `ConditionID` executed as a step (its answer would be discarded) and `Output` on a condition. Definitions recorded with the former `workflow::participant` / `workflow::condition` tags decode into `pkg/workflow/deprecated` adapters that delegate to `Execute`; don't use them in new code.

### Definition rules

- Store only serializable data: exported values, IDs, variable names, and nested definitions/conditions.
- A definition may **name** an application capability, but must never hold it. No closures, channels, mutexes, database clients, open connections, or pointers to live application state in a definition.
- For a custom definition, give it a stable namespaced `Error()` string (it doubles as the codec `@type` tag), keep its fields serializable, and give each nested operation an explicit, stable path segment with `workflow.WithName(ctx, "...")`. See [Writing a custom definition](#writing-a-custom-definition).
- Make custom behavior replay-safe. Reuse event-backed package operations (variables, `Execute`, etc.) rather than maintaining hidden per-process state. To keep a side effect in the definition's own code, run it through `workflow.Execute{}.ExecuteWith(ctx, pid, participantID, fn)`, where `fn` has the `Definition#Execute` signature: no registration; the `ParticipantID` argument identifies the call at `<path>/participant/<ParticipantID>`, so keep it stable, namespaced, and distinct at the same position (empty is fatal; also setting the `ParticipantID`/`ConditionID` fields on the struct is a fatal `ErrInvalidDefinition`); moving a registered participant inline under the same ID replays its recorded calls, except those recorded with an `Output` mapping, which run again; `Output` must be empty (set variables through `workflow.Vars`; a failed attempt discards them), and a returned `Definition` becomes a recorded follow-up like a participant's.
- `Sequence` position contributes to a step's idempotency path. A bound definition's data never changes, but a custom definition that builds its children in Go code changes with every deploy: do not insert or reorder steps there while existing processes may replay it. Append compatible work, or use `Replace` to move a process to a new definition.
- A participant may return a `workflow.Definition` as its `error` result to add recorded follow-up workflow stages. This is successful workflow control flow, not a failure.

### Variables and conditions

- Variables are a fold over process events. General `Vars` reads see current scoped state, including later writes from previous attempts; paths identify steps, not positions on a timeline. Historical input comparison belongs to the executor's cache validation, not ordinary variable reads. Use workflow operations or `workflow.GetVars(ctx)`; do not create an unrecorded side map as workflow state.
- `DeclareVar` creates a deliberate binding; `SetVar` writes through to the nearest visible binding; `Global: true` declares in the root scope. Use declaration intentionally when shadowing is required.
- Recording a condition's answer is the condition's responsibility, not the control-flow definition's. `Execute` with a `ConditionID` records at `<path>/<ConditionID>`; `wftemplate.Condition` records under the fixed ID `workflow::template::condition`, so two templates at one path share an answer; a custom `Condition` records only if it answers through `workflow.Execute{}.EvaluateWith(ctx, pid, conditionID, fn)`, which records at `<path>/<ConditionID>` (same ID rules as `ExecuteWith`), otherwise it is asked again on every evaluation, replays included. A recorded answer is replayed at the same path, so `If` walks the same branch again after a suspension, and each `For` round keeps its own answer.
- Older histories hold no recorded template answer, since templates used to be evaluated live: the first post-upgrade evaluation at each template position uses current scoped state and cannot reconstruct the old branch. Explicitly migrate affected processes where that is unsafe. Answer reuse requires a durable event; evaluation can repeat after event-store failure or enclosing rollback.

- `Sleep` asks its condition anew on every attempt, under `sleep/<unix seconds>` of the attempt, until the answer lets it wake up (`true` for `Until`, `false` for `While`); recording conditions, nested ones included, record per attempt second, so attempts within one second share answers and a frozen clock never wakes a `Sleep`. The wake-up is recorded as `EventSleepCompleted`, and a woken `Sleep` is passed on replay without asking its condition again. Any condition flavour fits `Sleep`.
- `wftemplate.Condition` is appropriate for builder-editable template expressions over current scoped process variables. It plugs into any `Condition`-typed slot (`If#Cond`, `For#Cond`, `Sleep#Until`, `Sleep#While`). Keep its function map intentionally limited and supply it through `Runtime.ContextSetup`.

## Writing a custom definition

Implement two methods (`Error()` and `Execute(ctx, pid)`) and the runtime treats your type like any built-in. There is no separate registration step with the engine — the only required step is codec registration, so the persistent adapter can polymorphically (de)serialize it.

```go
package acme

import (
    "context"
    "go.llib.dev/frameless/pkg/workflow"
)

// ChargeOrder is a named, reusable unit: take an amount from one variable,
// pass it to a registered participant, and record the receipt.
// The body could also be written as a workflow.Sequence{}, but a named type
// travels through the codec with a stable @type tag, which makes it auditable
// and reviewable in a UI.
type ChargeOrder struct {
    Amount workflow.VarName
}

var _ workflow.Definition = ChargeOrder{}

func (ChargeOrder) Error() string { return "acme::charge-order" }

func (d ChargeOrder) Execute(ctx context.Context, pid workflow.ProcessID) error {
    // 1. Contribute a path segment so this step is distinct from its siblings
    //    in the event log. See "Path identity" below.
    ctx = workflow.WithName(ctx, "charge-order")

    repo, err := workflow.LookupEventsRepository(ctx)
    if err != nil {
        return err
    }
    vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}

    amount, ok, err := vars.Lookup(ctx, d.Amount)
    if err != nil {
        return err
    }
    if !ok {
        // Missing input is an authoring error, not a workflow-level condition.
        return workflow.ErrFatal.F("missing input variable %q", d.Amount)
    }

    // 2. Delegate the side effect to a registered participant. Doing the I/O
    //    in a participant keeps the domain work out of the definition tree
    //    and gives you its event-cache boundary for free. To keep the I/O in
    //    this type instead, pass it to Execute#ExecuteWith.
    return workflow.Execute{
        ParticipantID: "charge-card",
        Input:         []workflow.VarName{d.Amount},
        Output:        []workflow.VarName{"receipt"},
    }.Execute(ctx, pid)
}
```

A definition that needs to keep its bookkeeping variables out of the
caller's scope adds `WithVarScope`:

```go
// Timed runs Body and records how long it took, in seconds, under `elapsed`.
// The bookkeeping variable lives in its own scope so the caller is free to
// use `elapsed` for something else.
type Timed struct {
    Body workflow.Definition
}

var _ workflow.Definition = Timed{}

func (Timed) Error() string { return "acme::timed" }

func (d Timed) Execute(ctx context.Context, pid workflow.ProcessID) error {
    ctx = workflow.WithName(ctx, "timed")
    ctx = workflow.WithVarScope(ctx, "timed") // keep `elapsed` local

    startedAt := time.Now()
    if err := d.Body.Execute(ctx, pid); err != nil {
        return err
    }

    repo, err := workflow.LookupEventsRepository(ctx)
    if err != nil {
        return err
    }
    vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}
    return vars.Set(ctx, "elapsed", time.Since(startedAt).Seconds())
}
```

Three obligations make a custom definition well-behaved:

1. **Stable path identity.** Always call `workflow.WithName(ctx, "...")` with a stable, namespaced segment before you recurse into nested operations. The path is the step's identity in the event log; without a unique segment, your child steps collide with siblings and replay skips the wrong one. Use `workflow.WithVarScope(ctx, "...")` alongside it when the definition writes bookkeeping variables that should not leak into the caller's scope.
2. **Serializability.** Every field must round-trip through the codec. The struct holds only names, IDs, and nested definitions/conditions — never closures, channels, or live application state.
3. **Statelessness and replay safety.** A definition may be walked repeatedly. General variable reads use current scoped state, not the state at the step's original execution; use recorded condition answers rather than assuming those reads reproduce the past. Lean on event-backed operations (`SetVar`, `vars.Set`, `vars.Lookup`, `Execute`, `Execute#ExecuteWith` for side effects the definition runs itself, etc.). If you need to wait on something the process does not own (a payment, a webhook, an external approval), that is a participant's job — return a `RuntimeSignal` from the participant, not from the definition.

### Path identity

A step's idempotency key is its `workflow.Path`, assembled from the context as the definition tree is walked. The first segment is the `EventID` of the current `EventUseDefinition`; every nested `WithName` and `WithVarScope` contributes a segment, and `Sequence` contributes its index. Two steps that resolve to the same path look like one step to the runtime — which means a replay short-circuits whichever the engine finds in the log first.

That is why `Sequence` index matters, why `WithName` is required in a custom definition, and why you must never insert or reorder steps in a definition that existing processes may replay: a new step at index 2 re-runs everything from index 2 onward. Append, or use `Replace` to move a process onto a new definition.

### Codec registration

The runtime hands Go values to your `EventRepository`, and the `adapter/memory` implementation keeps them as Go values. Real persistence (PostgreSQL, Kafka, …) goes through your codec, and the codec must know how to reconstruct a polymorphic value of the interface type `workflow.Definition` or `workflow.Condition`. Register your type with a stable namespaced tag:

```go
import (
    "go.llib.dev/frameless/pkg/jsonkit"
    "go.llib.dev/frameless/pkg/workflow/wfjson"
)

func NewCodec() *jsonkit.Codec {
    c := wfjson.NewCodec()
    jsonkit.CodecRegisterTypeID[ChargeOrder](c, "acme::charge-order")
    return c
}
```

Use `jsonkit.CodecRegister[T]` instead of `CodecRegisterTypeID[T]` when you need to own the wire format (DTOs, snake_case keys, schema constraints). See [Codec][CODEC] for the full pattern and the `wfcontract.Codec` round-trip suite.

The same rule applies to `wftemplate.Condition`, to your own `Condition` types, and to any persisted workflow value: register it once on the codec the persistence adapter uses, add a round-trip test, and the engine treats it as a first-class citizen.

## Implement participants and conditions

Register developers' capabilities in the runtime rather than embedding function values in definitions:

```go
rt.Participants = workflow.Participants{
    "charge-card": func(ctx context.Context, orderID string) (Receipt, error) {
        // domain logic
    },
}

rt.Conditions = workflow.Conditions{
    "requires-receipt": func(ctx context.Context, orderID string) (bool, error) {
        // decision logic
    },
}
```

Required signatures are:

```go
func(context.Context, args...) (results..., error) // Participant
func(context.Context, args...) (bool, error)       // registered Condition
```

`Participants` and `Conditions` are typed as `map[...]any` because the registered value can have any arity; the signature is checked with reflection at the call site. Variadic calls accept zero optional arguments, individual optional values, or a matching trailing slice (which takes precedence). Wrong arity, incompatible types, nil for non-nilable parameters, and nil functions produce explicit errors, not reflection panics. An untyped nil is an individual argument; a typed nil slice can supply a nil variadic slice.

Participant invocation incompatibilities return nonfatal `ErrParticipantSignatureMismatch{ID, Cause}` (in `wferror.go`), preserving `ErrInvalidParticipantFunc` or `ErrParticipantFuncMappingMismatch` through `errors.Is`/`errors.As`. Like `ErrParticipantNotFound`, the wrapper skips tight local retry, returns from `Execute`, preserves earlier work, and produces no failed participant event or `EventError`. The scheduler requeues after `WaitTime` and increments `ExecutionRequest.FailureCount`. `Runtime.ParticipantWarningInterval` warns every N such scheduling failures (default 5; values ≤ 0 use the default), with no hard drop limit or capability-routing guarantee. Validate IDs and signatures independently. Missing/invalid conditions and condition mapping errors remain fatal.

### What a participant can return

Participant outcomes differ in whether the runtime records the call; cache reuse below assumes a matching, durably committed event:

| Returned as `error` | Recorded as an event? | Called again on next pass? | Notes |
| --- | --- | --- | --- |
| `nil` | **Yes** | No — replayed from the event | Successful results are cached, not external effects. |
| A `workflow.Definition` (because `Definition` embeds `error`) | **Yes** | Producer: no; follow-up: walked again | Recorded nested steps reuse their results; unfinished/suspended work resumes under the original path. |
| `Suspend{}` / `Halt{}` | **No** | Yes, when execution resumes | Pre-signal work repeats. |
| `Replace{Definition: def}` | **Yes**, new definition | The replacement runs after its event commits | Not an exactly-once guarantee for preceding effects. |
| A plain operational `error` | **No** success event | Yes — through `RetryStrategy` | Fatal and participant availability errors have separate handling. |

A cached producer does not imply an exactly-once follow-up dispatch. Returned definitions are walked on initial execution and cache hits, inside the producer's event transaction. Suspension and participant availability errors preserve earlier work if that transaction commits. An ordinary nested failure can roll back newly recorded producer and earlier nested success events; these are not independent top-level commit boundaries. Use separate top-level steps when independent event commits are needed.

External effects may repeat even after a participant returns successfully: its effect can precede a failed event write/commit, crash, or enclosing rollback. Require stable business idempotency keys and retryable operations, or a shared transaction/transactional outbox where applicable. Splitting steps or returning a definition does not eliminate this window. Keep pre-signal work cheap and side-effect free.

Use `workflow.ErrFatal` for a permanent authoring, validation, or domain error that must not consume retry attempts. `Execute` returns it without local retry; the scheduler warns and ACKs/drops that queue entry without killing the worker. The process remains incomplete and nonterminated and may be manually `Schedule`d again after the cause is addressed. Return ordinary errors for retryable operational failures.

## Choose lifecycle and signals deliberately

Use one caller-owned process ID for every retry of a create, bind, or scheduling request:

```go
pid, err := workflow.MakeProcessID()
if err != nil { /* handle */ }

if err := rt.Bind(ctx, pid, definition); err != nil { /* handle */ }
err = rt.Execute(ctx, pid) // synchronous
```

For durable/background execution, start `rt.Run(ctx)` at application startup and use `rt.Schedule(ctx, pid)` after binding. `Runtime.Spawn(ctx, pid, definition)` performs the bind-and-schedule lifecycle for a new root process.

### Signal catalogue

A `RuntimeSignal` is an `error` value that is not a failure. The runtime type-asserts it before treating anything as a fault, so a wrapped signal falls through to the failure path on purpose — keep the value unwrapped.

| Signal | Use | Behavior |
| --- | --- | --- |
| `Suspend{}` | Pause until a later attempt | Scheduler requeues after `WaitTime`; the signaling participant is re-run (not cached). |
| `Halt{}` | Stop requeueing intentionally | Process is inert until the caller schedules its same ID again. Identified by value, so do not wrap. |
| `Replace{Definition: def}` | Move to a new definition | Appends a new `EventUseDefinition` and runs the replacement from its beginning. The signal itself is **recorded** — the call site becomes idempotent on replay. |
| `Complete{}` | Runtime's normal completion mechanism | Do not return this from a participant; use `Replace` with an empty sequence for deliberate early completion. |
| `Terminate{}` | Cancel a process externally | Prefer `rt.Terminate(ctx, pid)` so locking and in-flight cancellation notification occur. |

`Replace` records a new definition event and re-executes the process from the replacement's beginning. Once durably committed, that event directs later execution to the replacement rather than the original participant. It is useful for migrations and human-in-the-loop flows, but does not make preceding external effects "fire once": those still need to tolerate retries if persistence fails.

## Wire production runtime ports

Keep definitions and participants independent of persistence or messaging products. Wire those products into `workflow.Runtime` role interfaces:

| Runtime field | Responsibility |
| --- | --- |
| `Events` | Append-only source of truth for process history |
| `Queue` | Durable process scheduling |
| `Notifications` | Volatile worker wake-up and cancellation broadcast |
| `Locks` | Non-blocking mutual exclusion per `ProcessID` |
| `Participants`, `Conditions` | Application capability registries |
| `Codec` | Polymorphic workflow value serialization configuration |

Also consider `RetryStrategy`, `WaitTime`, `ParticipantWarningInterval`, `BindGracePeriod`, `NumQueueSubscriber`, and `ContextSetup` for tracing/logging decoration.

Use `wfjson.NewCodec()` for built-in workflow values. Register every application-specific definition, condition, or persisted workflow value with a stable namespaced type tag in the codec used by the persistence adapter, then add codec round-trip tests. Do not rely solely on in-memory tests: memory adapters retain Go values and do not prove serialization works.

Implement custom event, queue, notification, lock, or codec adapters against the supplied contracts. Preserve per-process ordering and replay semantics; do not substitute uncontracted assumptions about acknowledgements, locking, or event storage. The required `wfcontract` suites:

| Concern | Contract |
| --- | --- |
| Event store | `wfcontract.EventRepository` |
| Scheduler queue | `wfcontract.Queue` |
| Change-notification channel | `wfcontract.NotificationBroadcast` |
| Per-process locker | `wfcontract.ProcessLocks` |
| Your own `Definition` | `wfcontract.Definition` |
| Your wire format | `wfcontract.Codec` |

Each contract documents the rules you would not have guessed (e.g. `Queue` must not block on acknowledgement, `EventRepository#FindByProcessID` must return events ordered by timestamp ascending). Run them in CI and the adapter is held to the same standard as the in-memory implementation.

## Test at the right layer

Use the project's `go.llib.dev/testcase` style for package tests.

1. **Participant domain logic:** test it as an ordinary Go function without booting a runtime.
2. **Definition composition:** use `wftest.LetC(s)`. It provides in-memory ports, starts a background runtime, binds `c.Definition` to `c.ProcessID`, disables retries, and uses a very short suspension wait.
3. **Adapters and custom infrastructure:** run the relevant executable contract from the table above.

Register test participants using `wftest.LetParticipant` or `wftest.LetParticipantWithID`. After `c.ActExecute(t)`, assert immediately with `c.IsCompleted`. After `Schedule`, `Spawn`, or suspension, await asynchronous work with `c.ProcessCompletionIs`, `c.ChildrenCompletionAre`, or `c.WaitForSpawn`.

For a custom definition, cover at least successful execution, replay/idempotency, failure rollback, signal propagation where applicable, serialization, and a distinct execution path for each nested child. The `wfcontract.Definition` suite handles the first four for free; pair it with a codec round-trip test for the fifth, and a composition test that exercises every nested branch for the sixth.

## Documentation map

Read the smallest relevant source first; do not load every document by default.

| Need | Read |
| --- | --- |
| Concepts, runtime dependencies, replay/migrations | `pkg/workflow/README.md` |
| End-to-end setup, `Bind`/`Execute`/`Schedule`/`Run` | `pkg/workflow/docs/getting-started.md` |
| Vocabulary, roles, ports, event/path model | `pkg/workflow/docs/glossary.md` |
| Built-ins, custom definitions, path identity, spawn/join | `pkg/workflow/docs/definition.md` |
| Authoring a reusable custom `Definition` end-to-end | `pkg/workflow/docs/custom-definition.md` |
| Function signature, caching, follow-up stages, fatal errors | `pkg/workflow/docs/participant.md` |
| Recorded versus live conditions, `Sleep`, and templates | `pkg/workflow/docs/condition.md` |
| Lifecycle signals and their persistence/retry behavior | `pkg/workflow/docs/signal.md` |
| Variable scope, visibility, and child mappings | `pkg/workflow/docs/vars.md` |
| Polymorphic codec registration and compatibility | `pkg/workflow/docs/codec.md` |
| `wftest` helpers and `wfcontract` suites | `pkg/workflow/docs/testing.md` |
| Builder trust boundary and definition evolution | `pkg/workflow/docs/end-user.md` |

When the task is specifically to write or edit JSON that decodes into a
`workflow.Definition` (or vice versa), load
[`references/definitions-as-json.md`](references/definitions-as-json.md) — it
covers the `@type` tag catalogue, the inner JSON shape for every built-in,
custom-type registration, and a worked end-to-end example.

Use these implementation/test files as concrete, current examples:

- `pkg/workflow/example_test.go`
- `pkg/workflow/runtime.go`, `pkg/workflow/idempotent.go`, `pkg/workflow/path.go`
- `pkg/workflow/participant_test.go`, `pkg/workflow/condition_test.go`, `pkg/workflow/schedule_test.go`, `pkg/workflow/spawn_test.go`
- `pkg/workflow/wftest/wftest.go`
- `pkg/workflow/wfcontract/`
- `pkg/workflow/wfjson/wfjson.go`
- `pkg/workflow/wftemplate/template.go`

## Completion checklist

Before finalizing workflow-related changes, verify:

- [ ] Definitions are serializable data; every custom type is codec-registered with a namespaced tag and has a round-trip test.
- [ ] Custom definitions call `workflow.WithName` (and `workflow.WithVarScope` when they need scoped variables) at the top of `Execute`; nested steps have distinct path segments.
- [ ] Steps are replay-safe and retain stable path identity; no insertion or reordering of steps in code-built children that in-flight processes replay.
- [ ] Recorded events and bound definitions are never updated or deleted; definition changes go through a new `EventUseDefinition`, and run-time decisions through a participant-returned `Definition`.
- [ ] Process IDs are caller-owned and reused after ambiguous retries.
- [ ] Participant/condition signatures and variable wiring match positionally.
- [ ] Signals use their documented, unwrapped control-flow semantics; pre-signal work is cheap and side-effect free; `Replace` is not treated as an exactly-once external-effect guarantee.
- [ ] External effects tolerate event-write/commit failures and rollback using stable business idempotency keys or a shared transaction/outbox where applicable.
- [ ] Follow-up replay resumes suspended work; nested stages are not assumed to have independent top-level commit boundaries.
- [ ] Older histories without recorded template answers have an explicit migration plan where current-state evaluation is unsafe.
- [ ] Runtime adapters are covered by their applicable `wfcontract` suite.
- [ ] Composition tests use `wftest` and check the asynchronous lifecycle correctly.
- [ ] Relevant focused `go test` commands pass.
