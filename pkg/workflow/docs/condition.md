# Conditions

> A **Condition** answers exactly one question: yes or no. It is the yes/no half
> of `If` and `Sleep`, and nothing else.

```go
type Condition interface {
	Evaluate(ctx context.Context, processID ProcessID) (bool, error)
}
```

Anywhere a definition takes a condition — `If#Cond`, `Sleep#Until`,
`Sleep#While` — any implementation of that one method will do.

---

## 1. Three flavours

| Flavour                     | Where the logic lives            | Reads variables      | Answer recorded |
| --------------------------- | -------------------------------- | -------------------- | --------------- |
| `workflow.ExecuteCondition` | a Go func you register by name   | yes, via `Input`     | **yes**         |
| `wftemplate.Condition`      | a template string in the definition | see [§5](#5-template-conditions) | no |
| your own type               | your Go code                     | yes, however you like | no             |

The "answer recorded" column is the one that will surprise you. Read
[§4](#4-answers-are-recorded-not-re-asked) before you pick.

---

## 2. Registered conditions

Register Go functions on the runtime, exactly like [participants][PARTICIPANT]:

```go
rt := workflow.Runtime{
	Conditions: workflow.Conditions{
		"is-vip": func(ctx context.Context, customerID string) (bool, error) {
			return customerID == "cust-1", nil
		},
		"stock-available": func(ctx context.Context, sku string, qty int) (bool, error) {
			return qty <= 10, nil
		},
	},
}
```

Refer to them from a definition by ID:

```go
workflow.If{
	Cond: workflow.ExecuteCondition{
		ID:    "stock-available",
		Input: []workflow.VarName{"sku", "qty"},
	},
	Then: workflow.ExecuteParticipant{ID: "reserve", Input: []workflow.VarName{"sku"}},
	Else: workflow.ExecuteParticipant{ID: "backorder", Input: []workflow.VarName{"sku"}},
}
```

`workflow.Conditions` is a `map[ConditionID]any` — the `any` is what lets you
register any arity. The signature is checked with reflection instead.

### The signature rule

```
func(context.Context, arg1 T1, ...OtherArgs) (bool, error)
```

A `context.Context` first, your own argument types after it, a `bool` first out
and an `error` last. Anything else is `workflow.ErrInvalidConditionFunc`. The
argument types must be serialisable, because they are recorded.

Validate the registry yourself at boot:

```go
if err := rt.Conditions.(workflow.Conditions).Validate(ctx); err != nil {
	log.Fatal(err) // "is-vip: Invalid workflow.Condition#Func signature: …"
}
```

Do this explicitly — `Runtime#Validate` checks the runtime's own dependencies
and does **not** descend into the condition registry, so an unchecked bad
signature only surfaces the first time a process reaches that branch.

### Wiring the inputs

`Input` maps process variables **positionally** onto the arguments after `ctx`:

```go
Input: []workflow.VarName{"sku", "qty"} // ─► func(ctx, sku string, qty int) (bool, error)
```

| Problem                          | Result                            | Retried? |
| -------------------------------- | --------------------------------- | -------- |
| `ID` is not registered           | `ErrConditionNotFound`            | no       |
| An input variable is not set     | `ErrFatal` — "missing input argument" | no    |
| The condition func returns an error | that error                     | yes      |

The first two are fatal on purpose: they are authoring mistakes, and retrying a
typo forever helps nobody.

---

## 3. Also a definition

`ExecuteCondition` implements `Definition` too, so it can sit in a `Sequence`:

```go
workflow.Sequence{
	workflow.ExecuteCondition{ID: "is-vip", Input: []workflow.VarName{"customer_id"}},
	// ...
}
```

Executed as a step it evaluates the condition and records the answer — and
nothing more. It assigns no variable, so the only reason to do this is to pin
an answer into the history at a chosen moment. Reading it back is `If`'s job.

---

## 4. Answers are recorded, not re-asked

Every `ExecuteCondition` evaluation goes through the same idempotent executor
that participants use, and lands in the event history as:

```go
workflow.EventCondition{
	ConditionID: "is-vip",
	Path:        workflow.Path{"…", "if", "condition", "is-vip"},
	Input:       []any{"cust-1"},
	Answer:      true,
}
```

Identity is **`ConditionID` + `Path`**, so the same condition used twice is two
independent questions, and every later attempt is a replay:

```go
workflow.Sequence{
	workflow.If{Cond: workflow.ExecuteCondition{ID: "flip"},
		Then: workflow.ExecuteParticipant{ID: "first"}},
	workflow.If{Cond: workflow.ExecuteCondition{ID: "flip"},
		Then: workflow.ExecuteParticipant{ID: "second"}},
}
```

```
attempt 1 → the func is called twice (two paths)
attempt 2 → the func is called zero times, both answers replayed
attempt 3 → zero times
```

### Why it works this way

A workflow can be replayed at any moment. If a branch could answer `true` on
Monday and `false` after a crash on Tuesday, the process would take both
branches across its lifetime and leave the world half-done. Freezing the answer
is what makes branching survivable.

### What this obliges you to

| Rule                                    | Because                                                  |
| --------------------------------------- | -------------------------------------------------------- |
| Deterministic given its inputs          | the recorded answer must stay the *right* answer          |
| No side effects                         | it runs once, so effects would happen at most once, unpredictably |
| Do not treat it as a live poll          | it is asked once per path, not once per attempt           |

Changing an input variable *after* the answer was recorded does **not** re-open
the question — the cache compares the input as it stood historically, at the
recorded position. What does invalidate it is a change to the *mapping*: a
different number of `Input` names, or names whose historical values no longer
match what was recorded.

### The trap

```go
// WRONG — this suspends for ever.
workflow.Sleep{Until: workflow.ExecuteCondition{ID: "is-approved"}}
```

The first evaluation answers `false`, that `false` is recorded, and every
subsequent attempt replays it. The process re-queues itself until the end of
time without ever calling your function again.

```go
// RIGHT — an uncached condition is asked again on every attempt.
workflow.Sleep{Until: ApprovalGranted{Order: "order_id"}}
```

**`Sleep` needs a condition that re-evaluates.** That means your own type
(§6) — see [Definitions][DEFINITION] for how `Sleep` and `Suspend{}` fit
together.

---

## 5. Template conditions

`wftemplate.Condition` is a `string` holding a Go [`text/template`][TEXT_TEMPLATE]
expression, stored inline in the definition. Its appeal is that an end user can
type the rule into a form — no Go code, no deployment.

```go
workflow.If{
	Cond: wftemplate.Condition(`eq .currency "EUR"`),
	Then: workflow.ExecuteParticipant{ID: "free-shipping"},
}
```

A reference like `.currency` resolves against the process variables, so the
rule reads whatever the process has recorded so far. A variable that was never
set is not an error — it resolves to the zero value, which makes the comparison
simply `false`.

### How it is evaluated

The expression is wrapped, executed against the process variables, and parsed:

```
`gt .total 100.0`  ─►  {{if gt .total 100.0 }}1{{else}}0{{end}}  ─►  "1"  ─►  true
```

Truthiness is therefore **Go template truthiness**: `false`, `0`, `""`, a nil
pointer and any empty array, slice or map all count as false. The rendered `1`
or `0` is then parsed with `strconv.ParseBool`. Anything the template cannot
evaluate becomes an ordinary error, which the runtime treats as retryable.

`Condition#Validate(ctx)` parses without executing — useful when accepting a
definition from an end user, so a malformed expression is rejected at the door
rather than mid-process. It is a **syntax** gate, not a correctness one: an
arity mistake such as `` `eq .total` `` parses cleanly and only fails once the
condition actually runs.

### Custom template functions

Inject them into the execution context:

```go
rt := workflow.Runtime{
	ContextSetup: workflow.ContextSetup{
		func(ctx context.Context) context.Context {
			return wftemplate.ContextWith(ctx, wftemplate.FuncMap{
				"isWeekend": func(t time.Time) bool {
					return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday
				},
			})
		},
	},
}
```

```go
wftemplate.Condition(`isWeekend .placed_at`)
```

`ContextSetup` runs for every process execution, which is what makes the funcs
available on every worker node. `ContextWith` **merges** with any `FuncMap`
already in the context, so layers compose rather than clobber each other — a
per-request map can add to the runtime-wide one.

The point of `FuncMap` is the boundary: your end users get a vocabulary you
chose, not arbitrary Go. `FuncMap#Validate` rejects any entry that is not a
function.

---

## 6. Writing your own

Two obligations: implement `Evaluate`, and be honest about what it reads.

```go
type ApprovalGranted struct {
	Order workflow.VarName
}

var _ workflow.Condition = ApprovalGranted{}

func (c ApprovalGranted) Evaluate(ctx context.Context, pid workflow.ProcessID) (bool, error) {
	repo, err := workflow.LookupEventsRepository(ctx)
	if err != nil {
		return false, err
	}
	vars := workflow.Vars{ProcessID: pid, EventsRepository: repo}

	order, ok, err := vars.Lookup(ctx, c.Order)
	if err != nil || !ok {
		return false, err
	}
	id, ok := order.(string)
	if !ok {
		return false, fmt.Errorf("%s is not a string but %T", c.Order, order)
	}
	return approvals.IsGranted(ctx, id) // a live read, on every attempt
}
```

Because it does not go through the recording executor, this one **is** a live
poll — which is precisely what `Sleep` wants and what `If` usually does not.

It is also a Definition field, so the same serialisation rules apply: name the
thing you need (`workflow.VarName`), never hold it. Register the type with your
[Codec][CODEC].

---

## 7. Choosing

| You want to…                                        | Use                                  |
| ---------------------------------------------------- | ------------------------------------ |
| Branch on domain logic, decided once and for all     | `workflow.ExecuteCondition`          |
| Let end users write a rule, no variables involved    | `wftemplate.Condition`               |
| Poll something until it changes                      | your own `Condition`, inside `Sleep` |
| Read process variables in a rule today               | `ExecuteCondition` or your own type  |

---

## 8. Where to go next

| Your question                                   | Read                        |
| ----------------------------------------------- | --------------------------- |
| "What can I put around a condition?"            | [Definitions][DEFINITION]   |
| "How do I expose my domain logic?"              | [Participants][PARTICIPANT] |
| "Where do the variables come from?"             | [Variables][VARIABLES]      |
| "How does `Suspend{}` actually work?"           | [Signals][SIGNAL]           |
| "How do I persist a custom condition?"          | [Codec][CODEC]              |
| "How do I test one?"                            | [Testing][TESTING]          |

[DEFINITION]: ./definition.md
[PARTICIPANT]: ./participant.md
[VARIABLES]: ./vars.md
[CODEC]: ./codec.md
[TESTING]: ./testing.md
[SIGNAL]: ./signal.md
[TEXT_TEMPLATE]: https://pkg.go.dev/text/template
