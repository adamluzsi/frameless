# Variables

> Process variables are how data moves between steps. They look like a map, but
> they are **a fold over the event history** — every read replays the log, every
> write appends to it.

```go
workflow.Sequence{
	workflow.SetVar{Name: "order_id", Value: "ORD-42"},
	workflow.ExecuteParticipant{
		ID:     "charge-card",
		Input:  []workflow.VarName{"order_id"}, // read
		Output: []workflow.VarName{"receipt"},  // write
	},
}
```

Most of the time that is all you need: `SetVar` puts a value in, `Input` reads
it, `Output` writes results back. The rest of this page is for when you need to
reach them from Go code, or when scoping surprises you.

---

## 1. Reading and writing from your own code

### Inside a participant or definition

```go
"charge-card": func(ctx context.Context) error {
	vars, err := workflow.GetVars(ctx)
	if err != nil {
		return err
	}

	orderID, ok, err := vars.Lookup(ctx, "order_id")
	if err != nil || !ok {
		return err
	}

	return vars.Set(ctx, "receipt", "RCPT-1")
},
```

`GetVars` resolves the current process and its event repository from the
context. It only works inside an execution.

### From outside

```go
vars := workflow.Vars{ProcessID: pid, EventsRepository: rt.Events}
m, err := vars.ToMap(ctx) // map[order_id:ORD-42 receipt:RCPT-1]
```

Useful for assertions in tests, for an admin UI, or for reading a child
process' variables after a `Spawn`.

### The API

| Method                    | Returns                          | Note                                       |
| ------------------------- | -------------------------------- | ------------------------------------------ |
| `Lookup(ctx, name)`       | `(any, bool, error)`             | `false` means *not declared here*          |
| `Get(ctx, name)`          | `(any, error)`                   | `Lookup` without the `bool`                |
| `Set(ctx, name, val)`     | `error`                          | Appends; no-op if the value is unchanged   |
| `Delete(ctx, name)`       | `error`                          | No-op if the name is not visible           |
| `All(ctx)`                | `iter.Seq2[VarBinding, error]`   | Bindings, sorted by name                   |
| `Keys(ctx)`               | `iter.Seq2[VarName, error]`      | Names only                                 |
| `ToMap(ctx)`              | `(map[VarName]any, error)`       | The flat snapshot                          |

> `Lookup` returning `true` with a `nil` value is a real state: the variable was
> *declared* but never *assigned*. Declaration and assignment are separate
> events.

---

## 2. Why it is not a key-value store

Three event types make up the whole model:

| Event             | Analogous Go                   |
| ----------------- | ------------------------------ |
| `EventDeclareVar` | `var x string` — brings the name into existence in a scope |
| `EventSetVar`     | `x = "foo"` — assigns a value   |
| `EventDeleteVar`  | removes the binding             |

Because they are ordinary events, variables participate in replay, audit and
debugging exactly like every other state change. You can see *when* a value
changed, *where* in the definition tree it changed, and what it was before.

The fold walks events in **EventID order**. Event IDs are UUID v7, so they sort
by creation time — the ordering never depends on the wall clock in the
`Timestamp` field, which may be coarse or frozen.

---

## 3. Scoping works like Go

The mental model is exactly Go's block scoping.

**Assigning to a name declared further out writes through to it:**

```go
var x = "foo"
if true {
	x = "bar" // same variable
}
_ = x // "bar"
```

**An explicit re-declaration shadows it instead:**

```go
var x = "foo"
if true {
	x := "bar" // a new variable
	_ = x
}
_ = x // still "foo"
```

Workflow variables behave the same way. A `VarScope` is a path of scope names,
and a binding is visible from anywhere whose scope has the declaring scope as a
prefix. So:

- A **declaration** decides which scope a name lives in.
- A **set** finds the nearest visible declaration and writes through to it —
  which may well be an outer scope.
- A **delete** removes that same binding.

Once a name has been re-declared in a scope you cannot see, later assignments
belong to *that* hidden binding and are invisible from here, until the name is
declared again somewhere you can see.

> **Declaring is a step of its own.** `workflow.DeclareVar{Name: "x"}` brings a
> name into existence in the current scope and assigns nothing. That is what
> makes shadowing deliberate: a plain `SetVar` writes *through* to the nearest
> visible binding, even one an enclosing scope owns, whereas declaring first
> creates a fresh binding here, so the assignments that follow stay local.

### The `Global` escape hatch

```go
workflow.DeclareVar{Name: "trace_id", Global: true}
```

`Global: true` drops the current variable scope before declaring, so the name
lands in the **root scope** and stays visible from every scope in the process.

Use it for cross-cutting values — a correlation ID, a tenant — that every step
should see regardless of where it sits in the tree.

---

## 4. Writes are idempotent

The runtime may replay any step. Variable mutations are guarded so a replay does
not apply them twice:

- `SetVar` / `DeclareVar` / `DeleteVar` record the **Path** of the step that performed them.
  A replay that finds a matching *(event type, path, name)* triple in the
  history skips the write entirely.
- `Vars#Set` additionally short-circuits when the value is already equal to what
  is stored, so it appends nothing.

The guard on `DeleteVar` matters more than it looks: without it, a replay would
delete whatever the variable holds *at that point*, which is wrong as soon as a
later step reassigned it — the deletion would remove more than it originally
did.

The guard on `DeclareVar` is sharp for the same reason. A declaration folds into
a *fresh binding that carries no value*, so a replayed declaration would not
merely duplicate an event — it would erase whatever the steps after it assigned.

---

## 5. Passing variables to a sub-workflow

`VarMapping` is `map[parentVarKey]childVarKey`. It reads **parent → child**:

```go
workflow.Spawn{
	Name:       "fulfilment",
	Definition: workflow.ExecuteParticipant{ID: "ship", Input: []workflow.VarName{"order"}},
	Vars:       workflow.VarMapping{"order_id": "order"},
}
```

The parent's `order_id` is written into the child as `order`. A child gets its
own process, its own history and its own variables — nothing else is shared.

Parent variables that are not set yet are **skipped silently**, so put a
`SetVar` before the `Spawn` if the value must be there.

> **Not implemented yet:** `Join#Collect` is declared and serialised, but
> `Join#Execute` does not copy anything back. To get a result out of a child
> today, read the child's variables directly via its `EventSpawn#ChildID`, or
> have the child write to a store the parent can see.

---

## 6. What can go in a variable

Values are persisted in the event history, so they must survive whatever your
[Codec][CODEC] and `EventRepository` do to them. Strings, numbers, booleans and
registered struct types are fine. Closures, channels and live connections are
not.

Keep them small. A variable is copied into every event that touches it, and the
history is append-only.

---

## 7. Where to go next

| Your question                                  | Read                        |
| ---------------------------------------------- | --------------------------- |
| "What writes variables?"                       | [Definitions][DEFINITION]   |
| "How do Input/Output map onto my func?"        | [Participants][PARTICIPANT] |
| "How do I read a variable in a rule?"          | [Conditions][CONDITION]     |
| "What can be serialised?"                      | [Codec][CODEC]              |
| "How do I assert on variables in a test?"      | [Testing][TESTING]          |

[DEFINITION]: ./definition.md
[PARTICIPANT]: ./participant.md
[CONDITION]: ./condition.md
[CODEC]: ./codec.md
[TESTING]: ./testing.md
