---
name: frameless-workflow
description: Design, implement, integrate, migrate, and test replay-safe Go workflows with go.llib.dev/frameless/pkg/workflow, or change the package itself. Use when working with workflow definitions (built-in and custom), participants, conditions, runtime wiring, event stores, scheduling, signals, codecs, or wftest/wfcontract, and when adding built-ins, runtime features, or fixes inside pkg/workflow. Applies to any Go project that imports the package.
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

### Think in coordinates and folds

The package answers every state question the same way: **record a fact at a coordinate, and derive everything else by folding the ordered log.** Keep new code in that shape.

- **Coordinates come from `ctx`, not from references.** The `Path` (built by `WithName` and `WithVarScope`) says *which step or execution* recorded a fact. Its `Var` segments, the ones `WithVarScope` adds, are the variable scopes along it, like the blocks of a Go function, so `Path#VarScope()` says *which variable binding* the fact targets. Together with `ProcessID` and the variable name or capability ID, that is the whole identity of a fact. No event references another event's ID.
- **A variable scope is a location.** A scope is a segment of the `Path`, so its identity is where it is. Two scopes of the same name at different locations never share variables, while a replay of the same block is at the same location, so it sees the same scope again. A variable event made on behalf of an enclosing scope (a `Global` declaration, or an assignment that writes through) keeps the names of its location, with the scopes nested in the target scope unmarked, so its `Path#VarScope()` is the target scope.
- **Every block has a scope of its own.** A `For` is a scope (`for`) where `init` runs, each round is a scope nested in it (`[n]`) where `cond` and `post` run, and the `do` of the round is a scope nested in that (`body`). A `ForEach` iteration (`[i]` or the key) is a scope too. An `If` is a scope (`if`) where `cond` is evaluated, and the branch it takes is a scope nested in it (`then` or `else`), so what a branch declares is gone after the `If`; declare a variable before the `If` to keep a value a branch produces. Sibling loops and branches, and repeated rounds, never share variables.
- **The log is ordered by creation.** `EventID` is a UUIDv7. Order-sensitive folds sort by it (`sortEvents`), because timestamps can tie.
- **"Do once and remember" has one primitive:** the idempotent executor behind `Execute`, `Execute#ExecuteWith` and `Execute#EvaluateWith`. Only success is recorded. Signals leave no call record.
- **A recorded event's `EventID` is the only sanctioned segment that is both unique and replay-stable.** It roots the paths of the bound definition (`EventUseDefinition`), and the variable scope a deferred definition runs in (`EventDefer`). A UUID generated during execution differs on every replay.
- **To get a different answer, move to a different coordinate.** `Sleep` asks at `sleep/<unix-second>`, `Replace` re-roots every path, and each loop round gets a new path. Recorded answers are never edited.

A solution can be correct and still be foreign to this model. For example, linking an `EventSetVar` to its declaration through a `DeclarationEventID` would work. But the `Path` of the event already names the variable scope of the binding the name resolved to, and the fold does the rest. A second identity scheme would have to be taught to every fold, replay guard, codec and adapter. The full rationale is in `pkg/workflow/docs/mental-model.md`.

Before changing behavior, identify whether the request concerns:

1. **a participant/condition** — a unit of domain behavior;
2. **a definition** — serializable composition and control flow (built-in or your own type);
3. **runtime infrastructure** — events, queue, locks, notifications, retry, or codec;
4. **a migration** — an already-bound definition or persisted event format;
5. **tests/contracts** — composition tests or an adapter implementation;
6. **the package itself** — a new built-in, runtime feature, or fix inside `pkg/workflow`. Read `pkg/workflow/docs/mental-model.md` first and follow [Change the package itself](#change-the-package-itself).

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
| Application capabilities | `Execute` (a step with `Participant`, a condition with `Condition`) |
| Child processes | `Spawn`, `Join` |
| Process lifecycle | `Defer` (runs a definition when the process ends, LIFO; place it right after the step whose effect it cleans up, like Go's `defer f.Close()`) |

A normal composition wires variable names to capability inputs and results:

```go
workflow.Sequence{
    workflow.SetVar{Name: "order_id", Value: "ORD-42"},
    workflow.Execute{
        Participant: "charge-card",
        Input:       []workflow.VarName{"order_id"},
        Output:      []workflow.VarName{"receipt"},
    },
    workflow.If{
        Cond: workflow.Execute{
            Condition: "requires-receipt",
            Input:     []workflow.VarName{"order_id"},
        },
        Then: workflow.Execute{
            Participant: "email-receipt",
            Input:       []workflow.VarName{"receipt"},
        },
    },
}
```

`Input` and `Output` are **positional**: inputs map to function parameters after `context.Context`; outputs map to non-error results. They are not matched by variable-name text. Missing variables and arity/type mismatches must remain explicit errors, not panics.

Set exactly one of `Participant` and `Condition` on an `Execute`; it decides the role. Anything else fails with a fatal `ErrInvalidDefinition` before anything runs, including a `Condition` executed as a step (its answer would be discarded) and `Output` on a condition. Definitions recorded with the former `workflow::participant` / `workflow::condition` tags decode into `pkg/workflow/deprecated` adapters that delegate to `Execute`; don't use them in new code.

### Definition rules

- Store only serializable data: exported values, IDs, variable names, and nested definitions/conditions.
- A definition may **name** an application capability, but must never hold it. No closures, channels, mutexes, database clients, open connections, or pointers to live application state in a definition.
- For a custom definition, give it a stable namespaced `Error()` string (it doubles as the codec `@type` tag), keep its fields serializable, and give each nested operation an explicit, stable path segment with `workflow.WithName(ctx, "...")`. See [Writing a custom definition](#writing-a-custom-definition).
- Make custom behavior replay-safe. Reuse event-backed package operations (variables, `Execute`, etc.) rather than maintaining hidden per-process state. To keep a side effect in the definition's own code, run it through `workflow.Execute{}.ExecuteWith(ctx, pid, participantID, fn)`, where `fn` has the `Definition#Execute` signature: no registration; the `ParticipantID` argument identifies the call at `<path>/<ParticipantID>`, so keep it stable, namespaced, and distinct at the same position (empty is fatal; also setting the `Participant`/`Condition` fields on the struct is a fatal `ErrInvalidDefinition`); moving a registered participant inline under the same ID replays its recorded calls, except those recorded with an `Output` mapping, which run again; `Output` must be empty (set variables through `workflow.Vars`; a failed attempt discards them), and a returned `Definition` becomes a recorded follow-up like a participant's.
- `Sequence` position contributes to a step's idempotency path. A bound definition's data never changes, but a custom definition that builds its children in Go code changes with every deploy: do not insert or reorder steps there while existing processes may replay it. Append compatible work, or use `Replace` to move a process to a new definition.
- A participant may return a `workflow.Definition` as its `error` result to add recorded follow-up workflow stages. This is successful workflow control flow, not a failure.
- Clean up the way Go's `defer` does: act first, then defer the cleanup. Put the participant that makes the change first, and `workflow.Defer{Definition: <cleanup>}` right after it, so the cleanup is only registered once the change took place and its call is on record. A change whose call fails to be recorded is retried, not cleaned up, so the participant must stay repeatable; it can register an immediate undo with `txkit.OnRollback`. To register the cleanup in the same commit as the call (no gap where a termination between the two steps skips it), the participant can instead return the `Defer` as its follow-up after its change succeeded. Deferred definitions run LIFO at every ending (completion or termination, before the outcome is recorded), so build them from idempotent steps that undo only what the process no longer needs. Never place a `Defer` ahead of the step it cleans up after.

### Variables and conditions

- Variables are a fold over process events. General `Vars` reads see current scoped state, including later writes from previous attempts; paths identify steps, not positions on a timeline. Historical input comparison belongs to the executor's cache validation, not ordinary variable reads. Use workflow operations or `workflow.GetVars(ctx)`; do not create an unrecorded side map as workflow state.
- `DeclareVar` creates a deliberate binding; `SetVar` writes through to the nearest visible binding; `Global: true` declares in the root scope. Use declaration intentionally when shadowing is required. A set/delete records the variable scope of the variable it reached as the `Path#VarScope()` of its event (the scopes nested in it are unmarked on the `Path`), and a scope holds at most one variable of a name, so a later re-declaration elsewhere never takes it over.
- The built-in variable steps (`SetVar`, `DeclareVar`, `DeleteVar`, `Increment`) are path-guarded. On replay, each skips if an event of its kind for that name already exists at its path, so inside a loop it records once per round or iteration. A direct `Vars#Set`/`Vars#Delete` call is only skipped when the value is unchanged. Inside a participant or an `ExecuteWith` function that is enough, because the recorded call is the guard. In plain definition code, a value that differs between attempts (clock, randomness, live reads) appends a new event on every replay, so compute it inside a recorded call.
- `ForEach` `K`/`V` behave like Go's `for range` loop variables: per iteration, shadowing outer variables of the same name, visible to nested loops, and an assignment to them stays within the iteration (it declares them in the iteration scope first). Never rely on a body `SetVar` on `V` to change an outer variable; assign to the collection or to a differently named outer variable instead.
- `For` scopes like Go's three clause loop: `Init` declares the loop variables in the loop's scope, every round is a scope nested in it where `Cond` and `Post` run, and `Do` gets a fresh scope nested in the round's, so a body variable doesn't survive into `Post` or into the next round, while an assignment to the loop variable or an outer variable writes through. `Do` and `Post` are distinct steps, so the same step in both counts as done for neither. A `Defer` in the body sees the variables of its own round.
- A `Defer`'s hook runs at the recorded `Path` of its registration, in a variable scope of its own, outside of any loop body, so it can't see `ForEach` `K`/`V`. Copy the value into a variable the iteration declares first (e.g. through a participant's `Output`) if the cleanup needs it.
- `Spawn#Vars` reads the parent variable from where the `Spawn` is, and declares the child variable in the child's root scope.
- `Join#Collect` is the way back, `{childKey: parentKey}`: once the named child completed, its root-scope `childKey` is assigned to `parentKey` where the `Join` is (writing through to a visible variable, or declaring it in the `Join`'s scope). Only listed variables are collected (nil/empty collects nothing), unset child variables are skipped, and the values are recorded with the `EventJoin`, so a replay never collects again. Declare the parent variable before an `If` when a `Join` inside a branch must deliver it past the `If`.
- Recording a condition's answer is the condition's responsibility, not the control-flow definition's. `Execute` with a `Condition` records at `<path>/<ConditionID>`; `wftemplate.Condition` records under the fixed ID `template-condition`, so two templates at one path share an answer; a custom `Condition` records only if it answers through `workflow.Execute{}.EvaluateWith(ctx, pid, conditionID, fn)`, which records at `<path>/<ConditionID>` (same ID rules as `ExecuteWith`), otherwise it is asked again on every evaluation, replays included. A recorded answer is replayed at the same path, so `If` walks the same branch again after a suspension, and each `For` round keeps its own answer.
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
        Participant: "charge-card",
        Input:       []workflow.VarName{d.Amount},
        Output:      []workflow.VarName{"receipt"},
    }.Execute(ctx, pid)
}
```

A definition that needs to keep its bookkeeping variables out of the
caller's scope adds `WithVarScope`. Reading the clock is non-deterministic, so
each reading happens inside a recorded call, and a replay reuses what the call
wrote. Setting `elapsed` directly from definition code would append a new
value on every replay.

```go
// Timed runs Body and records how long it took, in seconds, under `elapsed`.
// The bookkeeping variables live in their own scope so the caller is free to
// use the same names for something else.
type Timed struct {
    Body workflow.Definition
}

var _ workflow.Definition = Timed{}

func (Timed) Error() string { return "acme::timed" }

func (d Timed) Execute(ctx context.Context, pid workflow.ProcessID) error {
    // a variable scope is a path segment too, so it claims the coordinate on its own
    ctx = workflow.WithVarScope(ctx, "timed") // keep the bookkeeping local

    repo, err := workflow.LookupEventsRepository(ctx)
    if err != nil {
        return err
    }
    vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}

    if err := (workflow.Execute{}).ExecuteWith(ctx, pid, "acme::timed::start", func(ctx context.Context, pid workflow.ProcessID) error {
        return vars.Set(ctx, "started_at", time.Now().UTC().Format(time.RFC3339Nano))
    }); err != nil {
        return err
    }

    if err := d.Body.Execute(ctx, pid); err != nil {
        return err
    }

    return workflow.Execute{}.ExecuteWith(ctx, pid, "acme::timed::stop", func(ctx context.Context, pid workflow.ProcessID) error {
        raw, err := vars.Get(ctx, "started_at")
        if err != nil {
            return err
        }
        startedAt, err := time.Parse(time.RFC3339Nano, raw.(string))
        if err != nil {
            return workflow.ErrFatal.F("invalid started_at: %v", err)
        }
        return vars.Set(ctx, "elapsed", time.Since(startedAt).Seconds())
    })
}
```

`started_at` is stored as a string because variable values must survive the
codec round trip, and plain JSON scalars do.

Three obligations make a custom definition well-behaved:

1. **Stable path identity.** Always call `workflow.WithName(ctx, "...")` with a stable, namespaced segment before you recurse into nested operations. The path is the step's identity in the event log; without a unique segment, your child steps collide with siblings and replay skips the wrong one. When the definition writes bookkeeping variables that should not leak into the caller's scope, call `workflow.WithVarScope(ctx, "...")` instead: it adds a segment that is a variable scope as well, so don't pair it with a `WithName` of the same name.
2. **Serializability.** Every field must round-trip through the codec. The struct holds only names, IDs, and nested definitions/conditions — never closures, channels, or live application state.
3. **Statelessness and replay safety.** A definition may be walked repeatedly. General variable reads use current scoped state, not the state at the step's original execution; use recorded condition answers rather than assuming those reads reproduce the past. Lean on event-backed operations (`SetVar`, `vars.Lookup`, `Execute`, `Execute#ExecuteWith` for side effects the definition runs itself, etc.). Anything non-deterministic (clock, randomness, I/O) belongs inside a recorded call, together with the `vars.Set` that stores its result. If you need to wait on something the process does not own (a payment, a webhook, an external approval), that is a participant's job — return a `RuntimeSignal` from the participant, not from the definition.

### Path identity

A step's idempotency key is its `workflow.Path`, assembled from the context as the definition tree is walked. The first segment is the `EventID` of the current `EventUseDefinition`. Every nested `WithName` and `WithVarScope` contributes a segment, and `Sequence` contributes `sequence/[i]`. The segments `WithVarScope` adds are marked `Var`: they are the variable scopes along the path, and `Path#VarScope()`, the path up to its last `Var` segment, decides which variable bindings a step sees and declares into. Two steps that resolve to the same path look like one step to the runtime — which means a replay short-circuits whichever the engine finds in the log first.

Segments must be distinct among siblings and deterministic from definition data plus recorded facts. Don't use random, time-based, or in-memory counter segments. The only exceptions in the package are recorded `EventID`s and `Sleep`'s deliberate per-second attempt segment.

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

## Change the package itself

Work inside `pkg/workflow` must keep every history that deployed code has already recorded replayable. That covers new built-ins, runtime features, port contracts and fixes. Read `pkg/workflow/docs/mental-model.md` before designing, then:

- **Reach for existing mechanisms first**, in this order:
  1. a coordinate (`WithName`/`WithVarScope`);
  2. an event plus a fold;
  3. the idempotent executor (`ExecuteWith`/`EvaluateWith`);
  4. a `Definition`, for control flow that can be re-derived from state (like `Break`);
  5. a `RuntimeSignal`, to stop, pause or redirect.

  A new event type is justified only for a new *kind* of fact. A new identity scheme is almost never justified.
- **Shape of a built-in definition:**
  - a plain value type;
  - `Execute` begins with `WithName(ctx, "<name>")`, or with `WithVarScope(ctx, "<name>")` instead if it has local bindings;
  - distinct, deterministic child segments;
  - variable writes guarded by path (`setOnce`/`declareOnce`/`deleteOnce`);
  - `ErrFatal`/`ErrInvalidDefinition` for invalid data, and signals returned unwrapped;
  - registered in `wfjson.NewCodec` under `workflow::<name>` with a DTO (events go under `workflow::event::<name>`);
  - covered by `wfcontract.Definition`.
- **Participants do one idempotent effect**, or return a `Definition` of idempotent steps. Bundling several effects in one participant leaves no coordinate for the part that finished before a failure.
- **Ports:** extend the role interface's `wfcontract` suite first, then the adapters. Don't type-assert to a concrete adapter.
- **Compatibility:** built-in path and scope segments, wire tags and event shapes are persisted. Changing any of them, however cosmetic, breaks replay of existing processes.
  - Keep old tags decodable (`jsonkit.CodecRegisterMigration`).
  - Keep old Go shapes in `deprecated`.
  - Never edit the `internal/compatibility-promise` fixtures.

| Tempting solution | Native idiom |
| --- | --- |
| An event field pointing at another event (e.g. `DeclarationEventID`) | Record the coordinates (`Path` with its variable scopes, `Name`) and fold in `EventID` order |
| A step-status flag, table or in-memory cache | A success event at the step's `Path` is the status |
| Updating or deleting an event to change an outcome | Ask at a new coordinate, or `Replace` with a compensating definition |
| A parent-ID field passed down to children | The parent adds a `WithName` segment to `ctx` |
| A fresh UUID or timestamp as a path segment | A recorded event's `EventID` as the segment |
| A new "run once" mechanism for custom logic | `Execute#ExecuteWith` / `Execute#EvaluateWith` under a stable, namespaced ID |
| A runtime special case for one definition type | Behavior in the definition's own `Execute`, driven by `ctx` coordinates |
| Inserting a step into a bound or code-built definition | Append, `Replace`, or migrate |

The current code has a few spots that don't follow the model. Treat them as open questions, and don't build on or around them. See "Known deviations" in `docs/mental-model.md`. In short:

- A deferred definition can't see `ForEach` `K`/`V`.
- `ForEach` adds no `foreach` path segment.
- Built-in `Error()` strings differ from their wire tags.
- The active binding and `Defer` unwinding are ordered by timestamp.
- `Spawn` identity is per name, not per path.

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
| Function signature, caching, follow-up stages, fatal errors, cleanups (`txkit.OnRollback`, `Defer`) | `pkg/workflow/docs/participant.md` |
| Recorded versus live conditions, `Sleep`, and templates | `pkg/workflow/docs/condition.md` |
| Lifecycle signals and their persistence/retry behavior | `pkg/workflow/docs/signal.md` |
| Variable scope, visibility, and child mappings | `pkg/workflow/docs/vars.md` |
| Polymorphic codec registration and compatibility | `pkg/workflow/docs/codec.md` |
| `wftest` helpers and `wfcontract` suites | `pkg/workflow/docs/testing.md` |
| Builder trust boundary and definition evolution | `pkg/workflow/docs/end-user.md` |
| Design idioms and invariants for changing or extending the package itself (coordinates, folds, replay, evolution) | `pkg/workflow/docs/mental-model.md` |

When the task is specifically to write or edit JSON that decodes into a
`workflow.Definition` (or vice versa), load
[`references/definitions-as-json.md`](references/definitions-as-json.md) — it
covers the `@type` tag catalogue, the inner JSON shape for every built-in,
custom-type registration, and a worked end-to-end example.

Use these implementation/test files as concrete, current examples:

- `pkg/workflow/example_test.go`
- `pkg/workflow/runtime.go`, `pkg/workflow/idempotent.go`, `pkg/workflow/path.go`
- `pkg/workflow/vars.go` (variable fold, scope resolution, replay guards), `pkg/workflow/loop.go`, `pkg/workflow/defer.go`
- `pkg/workflow/participant_test.go`, `pkg/workflow/condition_test.go`, `pkg/workflow/schedule_test.go`, `pkg/workflow/spawn_test.go`
- `pkg/workflow/wftest/wftest.go`
- `pkg/workflow/wfcontract/`
- `pkg/workflow/wfjson/wfjson.go`
- `pkg/workflow/wftemplate/template.go`

## Completion checklist

Before finalizing workflow-related changes, verify:

- [ ] Definitions are serializable data; every custom type is codec-registered with a namespaced tag and has a round-trip test.
- [ ] Custom definitions call `workflow.WithName` (or `workflow.WithVarScope` when they need scoped variables) at the top of `Execute`; nested steps have distinct path segments.
- [ ] Steps are replay-safe and retain stable path identity; no insertion or reordering of steps in code-built children that in-flight processes replay.
- [ ] Non-deterministic values (clock, randomness, live reads) are produced inside a recorded call; plain definition code writes variables only through the path-guarded steps.
- [ ] New behavior is expressed with coordinates (the `Path`, with its variable scopes), events plus folds, and the idempotent executor; no event references another event's ID, and no second identity scheme is introduced.
- [ ] Changes inside `pkg/workflow` leave built-in path/scope segments, wire tags, and event shapes unchanged, or come with a migration; `internal/compatibility-promise` fixtures are untouched.
- [ ] Recorded events and bound definitions are never updated or deleted; definition changes go through a new `EventUseDefinition`, and run-time decisions through a participant-returned `Definition`.
- [ ] Process IDs are caller-owned and reused after ambiguous retries.
- [ ] Participant/condition signatures and variable wiring match positionally.
- [ ] Signals use their documented, unwrapped control-flow semantics; pre-signal work is cheap and side-effect free; `Replace` is not treated as an exactly-once external-effect guarantee.
- [ ] External effects tolerate event-write/commit failures and rollback using stable business idempotency keys or a shared transaction/outbox where applicable.
- [ ] Cleanups are deferred right after the step whose change they undo (or returned as that participant's follow-up `Defer`), never ahead of it, and are built from idempotent steps.
- [ ] Follow-up replay resumes suspended work; nested stages are not assumed to have independent top-level commit boundaries.
- [ ] Older histories without recorded template answers have an explicit migration plan where current-state evaluation is unsafe.
- [ ] Runtime adapters are covered by their applicable `wfcontract` suite.
- [ ] Composition tests use `wftest` and check the asynchronous lifecycle correctly.
- [ ] Relevant focused `go test` commands pass.
