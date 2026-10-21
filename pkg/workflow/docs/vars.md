# Variables

> Process variables are how data moves between steps. They look like a map, but
> they are **a fold over the event history** — every read replays the log, every
> write appends to it.

```go
workflow.Sequence{
	workflow.SetVar{Name: "order_id", Value: "ORD-42"},
	workflow.Execute{
		Participant: "charge-card",
		Input:       []workflow.VarName{"order_id"}, // read
		Output:      []workflow.VarName{"receipt"},  // write
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

Workflow variables behave the same way. The variable scopes are segments of the
workflow `Path`, the ones marked as `var`, the way the blocks of a Go function
are places in its source, and a binding is visible from anywhere within the
scope it was declared in. So:

- A **declaration** decides which scope a name lives in. Every declaration is a
  variable of its own.
- A **set** finds the nearest visible declaration and writes through to it —
  which may well be an outer scope.
- A **delete** removes that same binding. A deleted binding keeps shadowing the
  outer ones, so the name is simply gone from that scope.

A set or a delete records the variable scope of the variable it reached as the
variable scope of its `Path` (`Path#VarScope()`, the path up to its last `var`
segment). The event is recorded at the location of the step that made it, but
the scopes nested in the reached variable's scope are not marked as `var` on it.
A scope holds at most one variable of a name, as re-declaring in the same scope
replaces it, so the name and the scope tell exactly which variable was meant,
and a re-declaration elsewhere never steals it:

```go
x := 1
{
	x := 2 // a new variable in the nested scope
	_ = x
}
x = 3 // still the outer x; the nested x keeps 2
```

The assignments are recorded with the scope of the variable they reached. The
nested `x := 2` is made in the nested scope, and names it:

```json
{"@type": "workflow::event::var::set", "path": [{"name": "block", "var": true}, {"name": "set-var"}], "name": "x", "value": 2}
```

while the outer `x = 3` is made outside of the block, in the root scope, so it
reaches the outer `x` only:

```json
{"@type": "workflow::event::var::set", "path": [{"name": "set-var"}], "name": "x", "value": 3}
```

Had the block assigned to the outer `x` instead (`x = 2`), the assignment
would be recorded at the block's location, with the block not marked as `var`,
so its variable scope is the root: `"path": [{"name": "block"}, {"name": "set-var"}]`.

> **Declaring is a step of its own.** `workflow.DeclareVar{Name: "x"}` brings a
> name into existence in the current scope and assigns nothing. That is what
> makes shadowing deliberate: a plain `SetVar` writes *through* to the nearest
> visible binding, even one an enclosing scope owns, whereas declaring first
> creates a fresh binding here, so the assignments that follow stay local.

### Loop variables

The variables of a loop are scoped the way Go scopes them. Every block has a
scope of its own: the body of a loop gets a fresh one on every iteration, or
round, and two loops side by side never share one.

`For` is a scope of its own, where `Init` runs, so the loop variable is visible
to the whole loop and gone after it. Every round is a scope nested in it, where
`Cond` and `Post` run, and the body, `Do`, is a fresh scope nested in the
round's:

```go
for i := 0; i < 3; i++ { // i belongs to the loop
	x := i                // x belongs to this round's body
	defer see(x)          // a deferred step sees the x of its own round
}
// neither i nor x is visible here
```

What a round declares in the body is gone by the next round, and `Post`
doesn't see it either. An assignment to `i`, or to a variable from outside of
the loop, still writes through.

`ForEach`'s `K` and `V` work like the variables of Go's `for range` loop: every
iteration has its own, in the iteration's scope.

```go
item := "outer"
for _, item := range items { // shadows the outer item
	item = "changed"          // stays within this iteration
}
_ = item // "outer"
```

- They **shadow** an outer variable of the same name inside the loop body.
- An **assignment** to them declares them in the iteration's scope first, so
  it never writes through to the outer variable, and the next iteration starts
  fresh. That holds on every round of an enclosing `For` too.
- A **nested loop** still sees the iteration variables of the loops around it.
- `V` is looked up from the collection on every read, so a body that assigns to
  the collection variable sees the new element.

The iteration variables themselves are not recorded; only an assignment or a
deletion made to them adds events.

### The `Global` escape hatch

```go
workflow.DeclareVar{Name: "trace_id", Global: true}
```

`Global: true` declares the name in the **root scope**, rather than in the
current variable scope, so it stays visible from every scope in the process.
The declaration is still recorded at the location of the step, with none of
its path segments marked as `var`.

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
	Definition: workflow.Execute{Participant: "ship", Input: []workflow.VarName{"order"}},
	Vars:       workflow.VarMapping{"order_id": "order"},
}
```

The parent's `order_id` is written into the child as `order`. A child gets its
own process, its own history and its own variables — nothing else is shared.
The parent's variable is looked up from where the `Spawn` is, even from within
a loop body, while the child's is declared in the child's **root scope**, where
every step of the child sees it.

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
