# Custom Definitions

> A custom `Definition` is a first-class citizen of the workflow tree. The
> runtime treats it like any built-in: it walks the value, records each step to
> the event log, and replays from there. There is no separate registration step
> with the engine — the only thing you must do is teach the codec about your
> type, so the persistence adapter can (de)serialize it polymorphically.

This page is the end-to-end view. [Definitions][DEFINITION] §7 shows the
minimal example; this page is the full checklist for a reusable custom
definition that survives serialization, replay, and migrations.

---

## 1. The interface, restated

```go
type Definition interface {
    Execute(ctx context.Context, pid workflow.ProcessID) error
    error
}
```

`Error()` doubles as the codec `@type` tag for your type, so it must be a
stable, namespaced, constant string — `"acme::charge-order"`, never a
formatted message. Pick a tag that a future built-in cannot collide with.

`Execute(ctx, pid)` is where the work happens. The context carries the
runtime, the current process's event repository, the path under construction,
and (optionally) a variable scope. The process ID identifies which process the
runtime is currently executing.

---

## 2. The minimum viable definition

```go
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
    // Contribute a path segment so this step is distinct from its siblings
    // in the event log. Without it, two sibling ChargeOrder values would
    // share a path and the second one would replay the first's events.
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

    // Delegate the actual side effect to a registered participant. Doing the
    // I/O in a participant keeps the domain work out of the definition tree
    // and gives you its event-cache boundary for free.
    return workflow.Execute{
        ParticipantID: "charge-card",
        Input:         []workflow.VarName{d.Amount},
        Output:        []workflow.VarName{"receipt"},
    }.Execute(ctx, pid)
}
```

Three things make this well-behaved:

1. **`WithName`** adds a path segment so the step is distinct from its siblings
   in the event log.
2. **No fields that cannot serialise** — just a `VarName`.
3. **Stateless and replay-safe** — the definition reads what the tree has
   accumulated, writes what it produces, and never assumes the runtime will
   come back later with a different value. If you need to wait on something
   the process does not own (a payment, a webhook, an external approval),
   that is a participant's job — return a `RuntimeSignal` from the
   participant, not from the definition.

---

## 3. Path identity: why `WithName` is not optional

A step's idempotency key is its `workflow.Path`, assembled from the context as
the definition tree is walked. A recorded event looks like this:

```
[01a042a6-b503-7e30-a540-796c969d63cd  sequence  [0]  charge-order]
```

| Segment       | Contributed by                                         |
| ------------- | ------------------------------------------------------ |
| `01a042a6-…`  | the `EventID` of the current `EventUseDefinition`      |
| `sequence`    | the enclosing `Sequence`                               |
| `[0]`         | its index inside that `Sequence`                       |
| `charge-order` | the most recent `WithName(ctx, "charge-order")` call   |

The first segment is the definition's event ID, not the process ID — binding a
new definition re-roots the tree, so a `Replace` starts the replacement from
its own beginning. After that, every nested `WithName` and `WithVarScope`
contributes a segment, and `Sequence` contributes its index.

> **Two steps that resolve to the same path look like one step to the
> runtime.** Replay short-circuits whichever the engine finds in the log
> first, so a custom definition that forwards its context unchanged makes its
> child indistinguishable from its sibling — and the idempotency replay then
> skips the wrong step. The `wfcontract.Definition` suite asserts this for
> every built-in composite (`Sequence`, `If`, `Sleep`, `Spawn`); your custom
> definition should do the same.

`WithVarScope` is the variable analogue: it opens a new variable scope whose
name is part of the variable path. `SetVar` and `DeclareVar` find the nearest
visible binding, so scoping lets a custom definition shadow outer variables
without colliding with them. It is the right tool when your definition is
looped, recursive, or otherwise re-enters a scope its parent might also be
writing to.

A definition that wraps another and records bookkeeping values around it is
the canonical case:

```go
// Timed runs Body and records how long the run took, in seconds, under
// `elapsed`. The bookkeeping variable lives in its own scope so the caller
// is free to use `elapsed` for something else.
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

Use the rule: if your definition writes variables that should not bleed into
the caller's scope — temporary bookkeeping, per-iteration state, anything
local — wrap the writes in `WithVarScope`. Otherwise `WithName` alone is
enough.

---

## 4. Replay safety: lean on event-backed operations

The runtime may replay any `Definition#Execute` call — crash recovery, a retry after a
transient failure, or a scheduler requeue after a `Suspend`. The side effects
on the process must converge to the same final state.

Two rules fall out of that:

- **Never maintain hidden per-process state.** A `map`, a `sync.Map`, a
  pointer stashed in a package variable, or a connection held by the
  definition will diverge between attempts. Read and write through the event
  log via `vars.Lookup` / `vars.Set` / `vars.Delete`, or via the built-in
  definition steps (`SetVar`, `DeleteVar`, `Execute`).
- **Make external effects retry-safe.** A `vars.Set` is replay-safe because
  the event log short-circuits duplicate writes. A call to your payment
  provider is not — mint an idempotency key, persist it via `SetVar` first,
  and pass the same key on the retry. Better still, run the work as a
  participant call, so its own event is the cache boundary — through a
  registered `Participant`, or through `Execute#ExecuteWith` when
  the logic belongs to the definition itself (below).

The same principle governs how you return. A `nil` or non-signal return is
recorded; a `RuntimeSignal` is not. Returning `workflow.Suspend{}` means
"come back later", and the runtime will re-invoke your function on the next
attempt — the work before the signal runs again, so keep it cheap and
side-effect free. Returning a `workflow.Definition` records the call with the
new definition attached and runs it in place of the step; a follow-up stage
is therefore "fire once" by design.

### Your own logic as a participant call

`Execute#ExecuteWith` takes a `ParticipantID` and a function with the signature
of `Definition#Execute`, and runs the function in place of a registered
participant. The call is cached the same way, so a definition can keep its side
effect in its own code, and nothing has to be registered:

```go
func (d ChargeOrder) Execute(ctx context.Context, pid workflow.ProcessID) error {
    ctx = workflow.WithName(ctx, "charge-order")
    return workflow.Execute{
        Input: []workflow.VarName{d.Amount}, // resolved and recorded with the call
    }.ExecuteWith(ctx, pid, "acme::charge-card", d.chargeCard)
}

// chargeCard is only called while no successful call is recorded at the position.
func (d ChargeOrder) chargeCard(ctx context.Context, pid workflow.ProcessID) error {
    repo, err := workflow.LookupEventsRepository(ctx)
    if err != nil {
        return err
    }
    vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}

    amount, _, err := vars.Lookup(ctx, d.Amount) // presence is checked through Input
    if err != nil {
        return err
    }
    receipt, err := payments.Charge(ctx, amount)
    if err != nil {
        return err
    }
    return vars.Set(ctx, "receipt", receipt)
}
```

- **The ID argument identifies the call.** Together with the path, the
  `ParticipantID` you pass identifies the call, so keep it stable across
  deployments, namespaced, and distinct from other participant calls at the
  same position. An empty ID is fatal, and so is setting `ParticipantID` or
  `ConditionID` on the `Execute` as well (a fatal `ErrInvalidDefinition`),
  since those fields belong to `#Execute` and `#Evaluate`.
- **Recorded like `Execute#Execute`.** The call is recorded as an
  `EventParticipant` under that `ParticipantID`, at
  `<path>/participant/<ParticipantID>`, just like a registered participant's.
  Moving a registered participant's logic into `#ExecuteWith` under the same
  `ParticipantID` replays the calls already recorded instead of repeating them —
  but only calls recorded without an `Output` mapping: `Output` is part of a
  call's cache identity, so where the registered participant mapped results
  onto `Output`, your function runs again.
- **Variables instead of `Output`.** The function returns no values, so a
  non-empty `Output` is a fatal `ErrInvalidDefinition`. Set results through
  `workflow.Vars`; those writes are recorded as part of the call, and discarded
  together with a failed attempt. `Input` works as with `#Execute`: a missing
  variable is fatal, and the resolved values are recorded.
- **Only success is recorded.** A failure is recorded as an `EventError` and
  retried on the next execution; a `RuntimeSignal` such as `Suspend` records
  nothing. Returning a `Definition` records it with the call and runs it in
  place of the step. Replays walk that recorded definition without calling
  your function again, which is how a routing definition extends the tree
  once and keeps that extension stable.

Prefer a registered participant when the capability should be part of the
vocabulary that end users compose ([Participants][PARTICIPANT] §7).

For a runnable version with a runtime and no registered participants, where a
suspended process is executed again without charging twice, see
`ExampleExecute_ExecuteWith` in [`example_executewith_test.go`][EXAMPLE_EXECUTE_WITH].

---

## 5. Codec registration

The runtime hands Go values to your `EventRepository`. The in-memory
implementation keeps them as Go values, which is fine for tests but
meaningless once a real persistence adapter is wired in. The adapter has to
turn a `workflow.Definition` into bytes and back, and the codec is the
component that knows the wire format.

`wfjson.NewCodec()` registers every built-in. Your type is not on that list,
so you must add it once to the codec your persistence adapter uses:

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

The tag is what the runtime sees on the wire, so it must match the value your
`Error()` method returns byte-for-byte. Pass the same `*jsonkit.Codec` to your
event/queue/notification adapters, and the system has one consistent view of
the wire format.

When you need to own the wire format — DTOs, snake_case keys, a schema you
must stay compatible with — use `jsonkit.CodecRegister[T]` with a
`jsonkit.ITypeCodec[T]`:

```go
type chargeOrderDTO struct {
    Amount string `json:"amount"`
}

type ChargeOrderCodec struct{}

func (ChargeOrderCodec) Marshal(c *jsonkit.Codec, v ChargeOrder) ([]byte, error) {
    return json.Marshal(chargeOrderDTO{Amount: string(v.Amount)})
}

func (ChargeOrderCodec) Unmarshal(c *jsonkit.Codec, data []byte, p *ChargeOrder) error {
    var dto chargeOrderDTO
    if err := json.Unmarshal(data, &dto); err != nil {
        return err
    }
    p.Amount = workflow.VarName(dto.Amount)
    return nil
}

func NewCodec() *jsonkit.Codec {
    c := wfjson.NewCodec()
    jsonkit.CodecRegister[ChargeOrder](c, "acme::charge-order", ChargeOrderCodec{})
    return c
}
```

Your `Marshal` must emit only the inner JSON — no `@type` field. `jsonkit`
adds the envelope itself; emitting your own produces a duplicate key. For a
nested `Definition` or `Condition` field, marshal it through the passed-in
`*jsonkit.Codec` (into a `json.RawMessage`) so the inner envelope survives.

The same rule applies to your own `Condition` types and to
`wftemplate.Condition` (it is shipped in `wfjson`, so it is already
registered — but the principle stands for any value the codec will encounter
through the `Definition` interface).

> **Pick namespaced tags.** They are written into every stored event, so a
> collision with a future built-in tag would be a data migration, not a
> compile error.

---

## 6. Testing a custom definition

Three layers, each with a different tool:

| Concern | How to test it |
| --- | --- |
| Logic of `Execute` | `wfcontract.Definition(mk)` — exercises success, replay, failure, rollback, signal propagation. |
| Serialization | `wfcontract.Codec(yourCodec)` round-trip; your own test if you added a custom type. |
| Composition | `wftest.LetC(s)` with `c.Definition` set to your type; assert with `c.IsCompleted` after `c.ActExecute`. |

`wfcontract.Definition` is the load-bearing one. It calls your `Execute` in
the same context a real runtime would, then calls it again and asserts that
no duplicate events were appended. For a definition that wraps other
definitions, use `c.MakeDefinition(tb)` inside the contract fixture to get a
child that asserts it was handed the right process ID and a **non-empty
path** — drop the `WithName` line and the contract fails.

```go
func TestChargeOrder(t *testing.T) {
    wfcontract.Definition(func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
        return ChargeOrder{Amount: "amount"}
    }).Test(t)
}
```

For a codec round-trip, register the type on a test codec and exercise it
through the `Definition` interface — that is the path production callers use:

```go
func TestChargeOrderCodec(t *testing.T) {
    wfcontract.Codec(NewCodec()).Test(t)
}
```

If you need behaviour the contracts do not cover (a specific replay order, a
custom signal handling, composition with other definitions), write a
`wftest.LetC`-based scenario test. See [Testing][TESTING] for the patterns.

---

## 7. Migration and evolution

There is no separate "version" concept for a custom definition; versions are
just the sequence of `EventUseDefinition` entries on a process. A change
ships as one of:

- **A new field with a default.** Old definitions deserialize with the zero
  value; old events replay with the new field ignored.
- **A new field without a default.** Make it a pointer or wrap it in an
  `omitempty` codec DTO, so old definitions still decode.
- **A renamed field.** A codec DTO bridges old and new JSON.
- **A new behavior.** Bump the `@type` tag (`acme::charge-order-v2`) and
  keep the old tag registered; the runtime distinguishes them by path.

In every case, run the codec contract test against both the old and the new
shape. The contract catches the silent-corruption cases — codec produces
JSON that decodes but does not re-marshal identically, or a renamed
discriminator that the polymorphic decoder cannot dispatch on.

For processes already in flight, do **not** rename an in-use `@type`. The
persisted event log will not decode. Add the new tag, leave the old one
registered, and migrate via `workflow.Replace{Definition}` (or a one-shot
migration tool that writes a new `EventUseDefinition` for the existing
process).

---

## 8. Where to go next

| Your question                                  | Read                          |
| ---------------------------------------------- | ----------------------------- |
| "What are all the building blocks?"            | [Definitions][DEFINITION]     |
| "Why must definitions be data at all?"         | [End Users][END_USER]         |
| "What can I put in a field?"                   | [Codec][CODEC]                |
| "How do I test a definition?"                  | [Testing][TESTING]            |
| "What is that word again?"                     | [Glossary][GLOSSARY]          |

[DEFINITION]: ./definition.md
[PARTICIPANT]: ./participant.md
[CONDITION]: ./condition.md
[VARIABLES]: ./vars.md
[CODEC]: ./codec.md
[END_USER]: ./end-user.md
[TESTING]: ./testing.md
[GLOSSARY]: ./glossary.md
[EXAMPLE_EXECUTE_WITH]: ../example_executewith_test.go
