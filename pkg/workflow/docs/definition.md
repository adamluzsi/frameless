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
	workflow.ExecuteParticipant{
		ID:     "charge-card",
		Input:  []workflow.VarName{"order_id"},
		Output: []workflow.VarName{"receipt"},
	},
	workflow.ExecuteParticipant{
		ID:    "email-receipt",
		Input: []workflow.VarName{"receipt"},
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
			Definition: workflow.ExecuteParticipant{ID: "ship"},
		}
	},
}
```

The runtime persists the participant's call result *with the returned
Definition attached*, then continues with that Definition in place of the step.
In the package's own words, `Sequence{StepA, StepB, StepC}` effectively becomes
`Sequence{StepA, StepB, StepBSubDef, StepC}`.

Because the swap is recorded in the same event, a replay of that step is a
no-op — the follow-up is dispatched exactly once.

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

The runtime may replay any `Execute` call — crash recovery, a retry after a
transient failure, or a scheduler requeue after a `Suspend`. The side effects
on the process must converge to the same final state.

```go
_ = rt.Bind(ctx, pid, def)
_ = rt.Execute(ctx, pid) // charge-card is called
_ = rt.Execute(ctx, pid) // replays the log — charge-card is NOT called again
```

You rarely implement this yourself. The built-ins lean on the event history:
each step records its outcome as an event, and a replay that finds that event
skips the work and reuses the recorded result.

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
| `participant` | `ExecuteParticipant`                                    |
| `charge-card` | the `ParticipantID` it calls                            |

`If` contributes `if` plus `then`/`else`, `Spawn` contributes `spawn` plus its
`Name`, and so on. The identity of a step is therefore *(which definition) +
(what it is) + (where it sits in the tree)* — no per-process bookkeeping is
needed from you.

> **Warning:** the position is part of the identity. Inserting a step at the
> front of a `Sequence` shifts every later index, so those steps look brand new
> to the runtime and **re-run** on an in-flight process. Append rather than
> insert when a process may already be running against the old shape.

Note the root segment: it is the definition's event ID, not the process ID.
Binding a *new* definition therefore re-roots every path in the tree, so a
[`Replace`][SIGNAL] starts the replacement from the beginning rather than
fast-forwarding through the old definition's recorded steps.

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
| `workflow.ExecuteParticipant` | Calls one of your registered Go functions.                               |
| `workflow.ExecuteCondition`   | Evaluates a registered condition. Implements *both* interfaces.           |
| `workflow.Spawn`              | Launches a sub-workflow as an independent process.                       |
| `workflow.Join`               | Waits for one, or all, spawned children to complete.                     |

### Sequence — run steps in order

```go
workflow.Sequence{
	workflow.SetVar{Name: "order_id", Value: "ORD-42"},
	workflow.ExecuteParticipant{ID: "charge-card",
		Input:  []workflow.VarName{"order_id"},
		Output: []workflow.VarName{"receipt"}},
	workflow.ExecuteParticipant{ID: "email-receipt",
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
	Cond: workflow.ExecuteCondition{
		ID:    "is-vip",
		Input: []workflow.VarName{"customer_id"},
	},
	Then: workflow.ExecuteParticipant{ID: "apply-vip-discount",
		Input: []workflow.VarName{"order_id"}},
	Else: workflow.ExecuteParticipant{ID: "apply-list-price",
		Input: []workflow.VarName{"order_id"}},
}
```

`Then` and `Else` are both optional — a nil branch simply does nothing. A nil
`Cond` is not: it returns `ErrFatal`, so the process fails without retrying.
Failing loudly beats silently taking the `Else` branch forever.

### Sleep — wait for something

```go
workflow.Sleep{Until: ApprovalGranted{Order: "order_id"}} // wait until it turns true
workflow.Sleep{While: OrderIsPending{Order: "order_id"}}  // wait while it stays true
```

`Sleep` evaluates its condition on every attempt. If it must keep waiting it
returns `Suspend{}`, a [signal][SIGNAL] the runtime recognises: the process is
re-queued rather than failed, and comes back after `Runtime#WaitTime`.

| Field   | Continues when      |
| ------- | ------------------- |
| `Until` | the condition is true  |
| `While` | the condition is false |

> **Gotcha:** `While` wins if both are set, and a `Sleep{}` with neither set
> suspends forever. Also, do **not** put a `workflow.ExecuteCondition` inside a
> `Sleep` — its answer is cached in the event history, so the first `false` is
> replayed for eternity. See [Conditions][CONDITION].

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

### ExecuteParticipant — call your code

```go
workflow.ExecuteParticipant{
	ID:     "charge-card",
	Input:  []workflow.VarName{"order_id"}, // ─► func(ctx, orderID string)
	Output: []workflow.VarName{"receipt"},  // ─► (receipt string, error)
}
```

`Input` maps positionally onto the arguments after `ctx`; `Output` onto the
results before the trailing `error`. A missing input variable is an `ErrFatal`,
and an arity mismatch is `ErrParticipantFuncMappingMismatch` — both explicit
errors, never a panic. See [Participants][PARTICIPANT].

### ExecuteCondition — as a definition

```go
workflow.ExecuteCondition{ID: "is-vip", Input: []workflow.VarName{"customer_id"}}
```

`ExecuteCondition` satisfies `Definition` as well as `Condition`. Executed as a
step it evaluates the condition and records the answer as an `EventCondition` —
and that is all it does. It assigns nothing to a variable, so its only use as a
step is to pin an answer into the history early. Read it back with `If`.

### Spawn — launch a sub-workflow

```go
workflow.Spawn{
	Name: "fulfilment",
	Definition: workflow.ExecuteParticipant{ID: "ship",
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
type AwaitApproval struct {
	Approver workflow.VarName
}

var _ workflow.Definition = AwaitApproval{}

func (AwaitApproval) Error() string { return "acme::await-approval" }

func (d AwaitApproval) Execute(ctx context.Context, pid workflow.ProcessID) error {
	ctx = workflow.WithName(ctx, "await-approval") // contribute a path segment

	repo, err := workflow.LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}

	approver, ok, err := vars.Lookup(ctx, d.Approver)
	if err != nil {
		return err
	}
	if !ok {
		return workflow.Suspend{} // nobody signed off yet — come back later
	}
	return vars.Set(ctx, "approved_by", approver)
}
```

Three things make this well-behaved:

1. **`WithName`** adds a path segment, so nested steps get distinct identities.
2. **No fields that cannot serialise** — just a `VarName`.
3. **Idempotent** — `Vars#Set` is a no-op when the value is already there, and
   returning `Suspend{}` leaves no partial state behind.

Register it with your [Codec][CODEC] before you rely on persistence, and reach
for the [contract tests][TESTING] to prove the round-trip.

---

## 8. Where to go next

| Your question                                       | Read                        |
| --------------------------------------------------- | --------------------------- |
| "How do I write the yes/no part?"                   | [Conditions][CONDITION]     |
| "How do I expose my domain logic?"                  | [Participants][PARTICIPANT] |
| "How do scoping and `Global` really work?"          | [Variables][VARIABLES]      |
| "How do I pause, stop or swap a running workflow?"  | [Signals][SIGNAL]           |
| "How do I persist definitions to my own database?"  | [Codec][CODEC]              |
| "How do I test a definition?"                       | [Testing][TESTING]          |
| "What is that word again?"                          | [Glossary][GLOSSARY]        |

[GETTING_STARTED]: ./getting-started.md
[GLOSSARY]: ./glossary.md
[PARTICIPANT]: ./participant.md
[CONDITION]: ./condition.md
[VARIABLES]: ./vars.md
[CODEC]: ./codec.md
[END_USER]: ./end-user.md
[TESTING]: ./testing.md
[SIGNAL]: ./signal.md
