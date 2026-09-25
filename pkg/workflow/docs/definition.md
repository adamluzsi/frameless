# Definitions

> A **Definition** is the workflow itself: an ordinary Go value that *describes*
> the steps. It never performs them — the `Runtime` does, one step at a time,
> recording each one in the event history.

If you have not run a workflow yet, start with [Getting Started][GETTING_STARTED].

---

## 1. A definition is data, not code

```go
def := workflow.Sequence{
	workflow.SetVar{Name: "order_id", Value: "ORD-42"},
	workflow.Execute{
		ParticipantID: "charge-card",
		Input:         []workflow.VarName{"order_id"},
		Output:        []workflow.VarName{"receipt"},
	},
	workflow.Execute{
		ParticipantID: "email-receipt",
		Input:         []workflow.VarName{"receipt"},
	},
}
```

Notice what is **not** there: no function values. The steps refer to your
[participants][PARTICIPANT] *by name*, through a `ParticipantID`.

That is the entire design bet. A tree of structs can be built in a UI, posted
over HTTP, stored in your database, diffed, versioned and audited — none of
which requires redeploying your Go binary. See [End Users][END_USER] for why.

---

## 2. The interface

```go
type Definition interface {
	Execute(ctx context.Context, processID ProcessID) error
	error // yes, really
}
```

### Why `error` is embedded

So that a participant can *return* a Definition as its error value, to extend
its own lifecycle with follow-up workflow stages:

```go
workflow.Participants{
	"reserve-stock": func(ctx context.Context, orderID string) error {
		// ... reserve the stock ...

		// Not a failure. "I am finished — and this step needs follow-up stages."
		return workflow.Spawn{
			Name:       "fulfilment",
			Definition: workflow.Execute{ParticipantID: "ship"},
		}
	},
}
```

The runtime persists the participant's call result *with the returned
Definition attached*, then continues with that Definition in place of the step.
In the package's own words, `Sequence{StepA, StepB, StepC}` effectively becomes
`Sequence{StepA, StepB, StepBSubDef, StepC}`.

A cache hit skips the producer function but walks its recorded follow-up
Definition again, reusing nested cached results and resuming suspended work.
The follow-up is not dispatched exactly once, nor does it gain independent
commit boundaries: it runs inside the producer's event transaction. An ordinary
nested failure can roll back newly recorded producer and nested success events.
See [Participants][PARTICIPANT] for retry and external-effect guarantees.

### The price

Every Definition needs an `Error() string`. The convention here is a stable,
namespaced constant — `"workflow::sequence"`, `"workflow::join"` — never a
formatted message, because it doubles as a type tag.

---

## 3. Contract one: it must serialise

Definitions are persisted into the event history as `EventUseDefinition`, so a
process can be saved, loaded, replayed, migrated or inspected. Whatever backs
your `EventRepository` has to turn that value into bytes.

```go
rt := workflow.Runtime{
	Events: &memory.WorkflowEventRepository{},
	Codec:  wfjson.NewCodec(),
}
```

> `adapter/memory` keeps events as Go values, so nothing is serialised in a
> pure in-memory test — a definition holding a closure will pass your unit
> tests and fail the moment you point it at a real database. Run the
> [codec contract][CODEC] to catch that early.

| Safe in a Definition field                   | Breaks serialisation                     |
| -------------------------------------------- | ---------------------------------------- |
| Exported fields of codec-registered types    | Closures, `func` fields                  |
| `VarName`, `ParticipantID`, `ConditionID`    | Channels, mutexes, open connections      |
| Other Definitions / Conditions               | Pointers into live application state     |

The rule of thumb: **a Definition may name a capability, never hold one.** A
`func` field would run fine in-process and then vanish the moment the process
is reloaded on another node. See [Codec][CODEC].

---

## 4. Contract two: it must be idempotent per ProcessID

The runtime may replay any `Definition#Execute` call — crash recovery, a retry after a
transient failure, or a scheduler requeue after a `Suspend`. The side effects
on the process must converge to the same final state.

```go
_ = rt.Bind(ctx, pid, def)
_ = rt.Execute(ctx, pid) // charge-card is called
_ = rt.Execute(ctx, pid) // replays the log — charge-card is NOT called again
```

You rarely implement this yourself. The built-ins lean on the event history:
each step records its outcome as an event, and a replay that finds that event
skips the recorded call and reuses its result. Any returned definition is still
walked. External effects can repeat if their success record is not durable
because of a write/commit failure, crash, or enclosing transaction rollback;
use stable business idempotency keys or a shared transaction/outbox where applicable.

---

## 5. How the runtime tells two steps apart

Every step executes under a **Path**, assembled from the context as the
definition tree is walked. A recorded participant step looks like this:

```
[01a042a6-b503-7e30-a540-796c969d63cd  sequence  [0]  participant  charge-card]
```

| Segment       | Contributed by                                          |
| ------------- | ------------------------------------------------------- |
| `01a042a6-…`  | the `EventID` of the current `EventUseDefinition`        |
| `sequence`    | the enclosing `Sequence`                                |
| `[0]`         | its index inside that `Sequence`                        |
| `participant` | `Execute` with a `ParticipantID`                        |
| `charge-card` | the `ParticipantID` it calls                            |

`If` contributes `if` plus `then`/`else`, `Spawn` contributes `spawn` plus its
`Name`, and so on. The identity of a step is therefore *(which definition) +
(what it is) + (where it sits in the tree)* — no per-process bookkeeping is
needed from you.

Paths identify steps, not positions on a variable timeline. General `Vars`
reads see the current state visible in their variable scope, including later
writes from previous attempts. The executor's historical input comparison for
cached calls does not turn ordinary variable reads into historical snapshots.

> **Warning:** the position is part of the identity. A bound definition's data
> never changes (see below), but a custom definition that assembles its
> children in Go code does change when you ship new code. Inserting a step at
> the front of such a `Sequence` shifts every later index, so those steps look
> brand new to the runtime and **re-run** on an in-flight process. Append
> rather than insert when a process may already be running against the old
> shape.

### A bound definition never changes

Once a definition is bound to a process, it is immutable. Nothing edits it in
place:

- `Bind` is a no-op when the process already has a definition.
- The only way to change it is to append a new `EventUseDefinition`, either
  with a [`Replace`][SIGNAL] signal or with a one-shot migration tool. The
  runtime always executes the latest one.
- The `EventRepository` port says the same thing. It can create and find
  events, but it has no `Updater` or `Saver`, so the history is append-only.

The root segment of every path is the binding's `EventID`, not the process ID
or a version index. A new binding therefore re-roots every path in the tree.
The replacement starts from its own beginning rather than fast-forwarding
through the old definition's recorded steps, and its steps can never collide
with the old ones.

This is why a recorded result can be replayed by its path alone. At a given
path there is only ever the step that was bound there, so no edited version of
it can inherit a stale result. A `wftemplate.Condition`, for example, records
under a fixed ID rather than its expression text.

When the shape of the work can only be decided at run time, let a participant
return a definition ([Participants][PARTICIPANT] §3). It is recorded with the
call and replayed from there, so the tree grows by appending, not by editing.

---

## 6. The catalogue

| Type                          | What it does                                                             |
| ----------------------------- | ------------------------------------------------------------------------ |
| `workflow.Sequence`           | Runs its child definitions in order.                                     |
| `workflow.If`                 | Runs `Then` or `Else` based on a [Condition][CONDITION].                  |
| `workflow.Sleep`              | Waits. Returns `Suspend{}` until its condition says otherwise.            |
| `workflow.SetVar`             | Assigns a process variable.                                              |
| `workflow.DeclareVar`         | Brings a process variable into existence, without assigning a value.     |
| `workflow.DeleteVar`          | Removes a process variable binding.                                      |
| `workflow.Execute`            | Calls a registered Go function: a participant as a step (`ParticipantID`), or a condition (`ConditionID`). Implements *both* interfaces. |
| `workflow.Spawn`              | Launches a sub-workflow as an independent process.                       |
| `workflow.Join`               | Waits for one, or all, spawned children to complete.                     |

### Sequence — run steps in order

```go
workflow.Sequence{
	workflow.SetVar{Name: "order_id", Value: "ORD-42"},
	workflow.Execute{ParticipantID: "charge-card",
		Input:  []workflow.VarName{"order_id"},
		Output: []workflow.VarName{"receipt"}},
	workflow.Execute{ParticipantID: "email-receipt",
		Input: []workflow.VarName{"receipt"}},
}
```

The first child to return an error stops the sequence. That includes
[signals][SIGNAL] such as `Suspend{}` — which is exactly how a mid-sequence
pause works: the sequence unwinds, and the next attempt replays it from the
top, skipping the already-recorded steps until it reaches the pause point.

### If — branch on a condition

```go
workflow.If{
	Cond: workflow.Execute{
		ConditionID: "is-vip",
		Input:       []workflow.VarName{"customer_id"},
	},
	Then: workflow.Execute{ParticipantID: "apply-vip-discount",
		Input: []workflow.VarName{"order_id"}},
	Else: workflow.Execute{ParticipantID: "apply-list-price",
		Input: []workflow.VarName{"order_id"}},
}
```

`Then` and `Else` are both optional — a nil branch simply does nothing. A nil
`Cond` is not: it returns `ErrFatal`, so the process fails without retrying.
Failing loudly beats silently taking the `Else` branch forever.

The branch is chosen by the condition's answer, and keeping that answer on
replay is the condition's job: `Execute` with a `ConditionID` and `wftemplate.Condition`
record it at the `if/<ID>` path, so a replay walks the same branch again, even
after a suspension inside it, and even if the variables the condition read have
changed since. A custom condition that records nothing is asked again on every
replay. See [Conditions][CONDITION].

### Sleep — wait for something

```go
workflow.Sleep{Until: ApprovalGranted{Order: "order_id"}} // wait until it turns true
workflow.Sleep{While: OrderIsPending{Order: "order_id"}}  // wait while it stays true
```

`Sleep` asks its condition on every attempt until the answer lets it wake up.
Until then it returns `Suspend{}`, a [signal][SIGNAL] the runtime recognises:
the process is re-queued rather than failed, and comes back after
`Runtime#WaitTime`. The wake-up is recorded as an `EventSleepCompleted`, so once
a `Sleep` woke up, a replay passes it without asking its condition again.

Attempts are told apart by the second they happen in: the conditions an attempt
asks record their answers under `sleep/<unix seconds>`, and attempts within the
same second share them. A `Sleep` under a frozen clock doesn't wake up.

| Field   | Continues when      |
| ------- | ------------------- |
| `Until` | the condition is true  |
| `While` | the condition is false |

> **Gotcha:** `While` wins if both are set, and a `Sleep{}` with neither set
> suspends forever. See [Conditions][CONDITION] for how conditions are recorded
> inside a `Sleep`.

### DeclareVar / SetVar / DeleteVar — move data between steps

```go
workflow.Sequence{
	workflow.DeclareVar{Name: "trace_id", Global: true},
	workflow.SetVar{Name: "attempt", Value: 1},
	workflow.DeleteVar{Name: "card_number"},
}
```

`SetVar` records an `EventDeclareVar` the first time the name comes into
existence in the current scope, then an `EventSetVar` for the assignment.

`DeclareVar` does only that first half — it brings a name into existence and
assigns nothing. Reach for it when you want the declaration to be deliberate:
to shadow a binding an enclosing scope owns, or, with `Global: true`, to put the
name in the **root scope** so every step of the process can see it.

`DeleteVar` records an `EventDeleteVar`. All three are guarded so that a replay
does not apply the mutation twice. Details in [Variables][VARIABLES].

### Execute — call your code

```go
workflow.Execute{
	ParticipantID: "charge-card",
	Input:         []workflow.VarName{"order_id"}, // ─► func(ctx, orderID string)
	Output:        []workflow.VarName{"receipt"},  // ─► (receipt string, error)
}
```

`Input` maps positionally onto the arguments after `ctx`; `Output` onto the
results before the trailing `error`. A missing input variable is an `ErrFatal`,
while incompatible arity/types or output counts return a nonfatal
`ErrParticipantSignatureMismatch{ID, Cause}` wrapping
`ErrParticipantFuncMappingMismatch`. Variadic inputs accept zero optional values,
individual values, or a matching trailing slice. Invalid mappings return errors,
not panics. Missing or incompatible registrations wait for availability rather
than retrying locally; see [Participants][PARTICIPANT].

The ID that is set decides what `Execute` calls, so set exactly one of them:

| Set             | It is a      | Use it as                               |
| --------------- | ------------ | --------------------------------------- |
| `ParticipantID` | `Definition` | a step, e.g. in a `Sequence`            |
| `ConditionID`   | `Condition`  | a condition, e.g. `If#Cond`, `Sleep#Until` |

Any other combination fails with a fatal `ErrInvalidDefinition`, before
anything is called or recorded. That includes a `ConditionID` executed as a
step, since an answer that is neither stored nor branched on has no effect, and
`Output` on a condition, since an answer is not stored in variables.

> Definitions recorded with the former `workflow::participant` and
> `workflow::condition` wire tags still decode, into the adapters in
> `pkg/workflow/deprecated`, which delegate to `Execute`, so a call or answer
> recorded by one is replayed by the other at the same position.

### Spawn — launch a sub-workflow

```go
workflow.Spawn{
	Name: "fulfilment",
	Definition: workflow.Execute{ParticipantID: "ship",
		Input:  []workflow.VarName{"order"},
		Output: []workflow.VarName{"tracking"}},
	Vars: workflow.VarMapping{"order_id": "order"},
}
```

A spawn creates a **separate process** with its own ID, its own event history
and its own variables. `Name` must be unique among a process' children; it is
what a later `Join` refers to.

`Vars` is a `VarMapping`, which reads **parent → child**: the parent's
`order_id` lands in the child as `order`. Parent variables that are not set yet
are skipped silently, so pair the spawn with a `SetVar` earlier in the
definition if the value must be there.

> **Why it is transactional:** the spawn event, the child's
> `EventUseDefinition` and the forwarded variables are written in one
> transaction, and the child is enqueued only *after* that commits. Enqueue
> first and a worker could pick the child up, find an empty history, conclude
> it was never bound, record `EventCompleted` — and silently drop the spawn.

### Join — wait for the children

```go
workflow.Join{SpawnName: "fulfilment"} // wait for one named child
workflow.Join{}                        // wait for every child spawned so far
```

`Join` returns `Suspend{}` until the child (or every child) has recorded
`EventCompleted`, then records an `EventJoin` and lets the sequence continue.
An unknown `SpawnName` is an error, not a silent pass.

> **Not implemented yet:** `Join#Collect` is declared, validated and
> serialised, but `Join#Execute` does not copy anything — child variables do
> not reach the parent today. Have the child write to a shared store, or read
> the child's variables yourself via its `EventSpawn#ChildID`.

---

## 7. Writing your own

Implement two methods and you are a first-class citizen of the tree:

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

1. **`WithName`** adds a path segment, so nested steps get distinct identities.
2. **No fields that cannot serialise** — just a `VarName`.
3. **Stateless and replay-safe** — the definition reads what the tree has
   accumulated, writes what it produces, and never assumes the runtime will
   come back later with a different value. If you need to wait on something
   the process does not own (a payment, a webhook, an external approval),
   that is a participant's job — return a `RuntimeSignal` from the
   participant, not from the definition.

Register it with your [Codec][CODEC] before you rely on persistence, and reach
for the [contract tests][TESTING] to prove the round-trip.

For the full checklist — path identity, scoping, codec registration,
migration, and testing — see [Custom Definitions][CUSTOM_DEFINITION]. Its §4
also shows how to keep the side effect in the definition's own code, without
registering a participant, through `Execute#ExecuteWith`.

---

## 8. Where to go next

| Your question                                       | Read                            |
| --------------------------------------------------- | ------------------------------- |
| "How do I write the yes/no part?"                   | [Conditions][CONDITION]         |
| "How do I expose my domain logic?"                  | [Participants][PARTICIPANT]     |
| "How do scoping and `Global` really work?"          | [Variables][VARIABLES]          |
| "How do I pause, stop or swap a running workflow?"  | [Signals][SIGNAL]               |
| "How do I persist a definition to my own database?" | [Codec][CODEC]                  |
| "How do I write a reusable custom definition?"      | [Custom Definitions][CUSTOM_DEFINITION] |
| "How do I test a definition?"                       | [Testing][TESTING]              |
| "What is that word again?"                          | [Glossary][GLOSSARY]            |

[GETTING_STARTED]: ./getting-started.md
[GLOSSARY]: ./glossary.md
[PARTICIPANT]: ./participant.md
[CONDITION]: ./condition.md
[VARIABLES]: ./vars.md
[CODEC]: ./codec.md
[END_USER]: ./end-user.md
[TESTING]: ./testing.md
[SIGNAL]: ./signal.md
[CUSTOM_DEFINITION]: ./custom-definition.md
