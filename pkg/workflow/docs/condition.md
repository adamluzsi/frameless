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
| `workflow.Execute` with a `ConditionID` | a Go func you register by name | yes, via `Input` | **yes**, wherever used |
| `wftemplate.Condition`      | a template string in the definition | current scoped variables | **yes**, wherever used |
| your own type               | your Go code                     | yes, however you like | only if it answers through `Execute#EvaluateWith` |

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

Refer to them from a definition by ID, with a `workflow.Execute` that sets
`ConditionID`:

```go
workflow.If{
	Cond: workflow.Execute{
		ConditionID: "stock-available",
		Input:       []workflow.VarName{"sku", "qty"},
	},
	Then: workflow.Execute{ParticipantID: "reserve", Input: []workflow.VarName{"sku"}},
	Else: workflow.Execute{ParticipantID: "backorder", Input: []workflow.VarName{"sku"}},
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

Variadic functions accept zero optional arguments, individual optional values,
or a matching trailing slice (which takes precedence). Wrong arity, incompatible
types, and nil for a non-nilable parameter produce explicit mapping errors, not
reflection panics. An untyped nil is an individual argument; use a typed nil
slice to explicitly pass a nil variadic slice.

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
| `ConditionID` is not registered  | `ErrConditionNotFound`            | no       |
| An input variable is not set     | `ErrFatal` — "missing input argument" | no    |
| Incompatible input arity/type or invalid nil | `ErrFatal` wrapping `ErrConditionFuncMappingMismatch` | no |
| Invalid or nil condition function | `ErrInvalidConditionFunc` | no |
| `Output` is set, or `ParticipantID` is set too | `ErrInvalidDefinition` | no |
| The condition func returns an error | that error                     | unless fatal |

Registration and mapping errors for conditions remain fatal authoring errors;
they do not use the participant availability-requeue policy.

---

## 3. Not a step

`Execute` implements `Definition` too, but only with a `ParticipantID`. An
`Execute` with a `ConditionID` executed as a step, e.g. in a `Sequence`, fails
with a fatal `ErrInvalidDefinition`: its answer would be neither stored nor
branched on, so asking the question would have no effect on the process. Ask it
where the answer is used, in an `If`, `For` or `Sleep`.

Definitions recorded with the former `workflow::condition` wire tag decode into
`deprecated.ExecuteCondition`, which still evaluates the condition as a step,
under a `workflow::condition` path segment, and discards the answer.

---

## 4. Answers are recorded, not re-asked

Every evaluation of an `Execute` with a `ConditionID` goes through the same
idempotent executor that participants use, and lands in the event history as:

```go
workflow.EventCondition{
	ConditionID: "is-vip",
	Path:        workflow.Path{"…", "if", "is-vip"},
	Input:       []any{"cust-1"},
	Answer:      true,
}
```

Identity is **`ConditionID` + `Path`**, so the same condition used twice is two
independent questions, and every later attempt is a replay:

```go
workflow.Sequence{
	workflow.If{Cond: workflow.Execute{ConditionID: "flip"},
		Then: workflow.Execute{ParticipantID: "first"}},
	workflow.If{Cond: workflow.Execute{ConditionID: "flip"},
		Then: workflow.Execute{ParticipantID: "second"}},
}
```

```
attempt 1 → the func is called twice (two paths)
attempt 2 → the func is called zero times, both answers replayed
attempt 3 → zero times
```

### Who records the answer

Recording is the condition's job. `If`, `For` and `Sleep` only give the
condition its path; whether a replay gets the same answer depends on the
condition itself:

| Condition                                     | Recorded as                                                  |
| --------------------------------------------- | ------------------------------------------------------------ |
| `Execute` with a `ConditionID`                | `ConditionID: <ID>` at `<path>/<ID>`                         |
| `wftemplate.Condition`                        | `ConditionID: "workflow::template::condition"`, at `<path>/workflow::template::condition` |
| your own type answering through `EvaluateWith` | the `ConditionID` you pass it, at `<path>/<ID>` (§6)         |
| your own type that doesn't                    | nothing — it is asked again on every evaluation, replays included |

A template records under a fixed ID, not its expression text. That is safe
because a bound definition is never edited in place ([Definitions][DEFINITION]
§5). A changed expression reaches a process only through a new binding, which
re-roots the path, so it is asked anew there.

Because the ID is fixed, two templates evaluated at the same path share one
recorded answer, just as two `Execute`s with the same `ConditionID` would. The
built-ins never do this, since `If`, `For` and `Sleep` each ask a single
condition at their position. A custom condition that combines several templates
must give each one its own segment with `workflow.WithName`.

Errors and runtime signals are not answers, so they are never recorded.
Reusing an answer requires a durable event. A failed event write/commit or an
enclosing transaction rollback can cause re-evaluation, so conditions should be
side-effect free.

### Upgrading older histories

Older versions evaluated `wftemplate.Condition` live on every attempt, so their
histories hold no recorded template answer. After the upgrade, the first
evaluation at each template position reads **current scoped state**, just as
before, and from then on that answer is replayed. It cannot reconstruct a branch
taken before the upgrade if the variables changed since. Where re-evaluating
would be unsafe, migrate the affected processes explicitly before resuming them.

A `Sleep` that got stuck on a recorded `false` from a registered condition in an
older version is asked again, and wakes up once the condition allows it.

Older versions recorded no `EventSleepCompleted`. A process that passed a
`Sleep` before the upgrade asks that `Sleep`'s condition once more on its next
replay, and if the answer has changed back since, it falls asleep again until
the condition lets it wake up.

General `Vars` reads, including template variable reads, see current scoped
state. A path identifies a step, not a position in time. Only the executor's
cache validation uses the historical inputs at an existing call event.

### Why it works this way

A workflow can be replayed at any moment. If a branch could answer `true` on
Monday and `false` after a crash on Tuesday, the process would take both
branches across its lifetime and leave the world half-done. Freezing the answer
is what makes branching survivable.

### What this obliges you to

| Rule                                    | Because                                                  |
| --------------------------------------- | -------------------------------------------------------- |
| Deterministic given its inputs          | the recorded answer must stay the *right* answer          |
| No side effects                         | evaluation can repeat before the answer is durably committed |
| A recorded answer is final at its path  | it is reused on replay, not refreshed; a `Sleep` asks at a new path in every attempt second |

For a registered condition, changing an input variable *after* the answer was
recorded does **not** re-open the question — the cache compares the input as it stood historically, at the
recorded position. What does invalidate it is a change to the *mapping*: a
different number of `Input` names, or names whose historical values no longer
match what was recorded.

### Conditions in `Sleep`

`Sleep` is where a condition is polled. Every attempt asks it anew, until its
answer lets the `Sleep` wake up — `true` for `Until`, `false` for `While` — and
the wake-up is recorded as an `EventSleepCompleted` at the `Sleep`'s position:

```go
workflow.Sleep{Until: workflow.Execute{ConditionID: "is-approved", Input: []workflow.VarName{"order_id"}}}
```

```
attempt 1, second S   → the func answers false → recorded at sleep/S/is-approved, the process suspends
attempt 2, second S+5 → the func answers false → recorded at sleep/S+5/is-approved, the process suspends
attempt 3, second S+9 → the func answers true  → recorded, EventSleepCompleted, the process continues
replay                → EventSleepCompleted found, the func is not called
```

Each attempt asks its condition under the second it happens in, as unix
seconds: `<path>/sleep/<seconds>`. A recording condition records its answers
there as usual, and so do the recording conditions nested inside it, so an
answer that kept the `Sleep` asleep is never replayed to a later attempt. What
follows from that:

- Attempts within the same second share their answers, so a `Sleep` wakes up
  no sooner than the second after it last found its condition unmet.
- Under a frozen clock a `Sleep` doesn't wake up. Time has to pass for a sleep
  to end.
- Every attempt in a new second adds the answers of its recording conditions to
  the process history.

The completion event is what keeps a woken `Sleep` awake: when the process
replays after a later suspension, the `Sleep` is passed without asking its
condition again, even if the variables it read have changed back since. That
holds for every condition, including one that records nothing (§6). See
[Definitions][DEFINITION] for how `Sleep` and `Suspend{}` fit together.

---

## 5. Template conditions

`wftemplate.Condition` is a `string` holding a Go [`text/template`][TEXT_TEMPLATE]
expression, stored inline in the definition. Its appeal is that an end user can
type the rule into a form — no Go code, no deployment.

```go
workflow.If{
	Cond: wftemplate.Condition(`eq .currency "EUR"`),
	Then: workflow.Execute{ParticipantID: "free-shipping"},
}
```

A reference like `.currency` resolves against current scoped process variables
when the template is evaluated. The answer is recorded like a registered
condition's (§4), so a replay at the same position gets the same
answer; in `Sleep`, every attempt second asks it anew.
A variable that was never set is not an error — it resolves to the zero value, which makes the comparison
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
	return approvals.IsGranted(ctx, id) // a live read whenever Evaluate is called
}
```

This type records nothing, so it is asked again on every evaluation, replays
included: an `If` using it can take the other branch on a replay if the approval
changed in between. To make its answers replay-stable, answer through
`Execute#EvaluateWith`, which records the answer the same way as a
registered condition's:

```go
func (c ApprovalGranted) Evaluate(ctx context.Context, pid workflow.ProcessID) (bool, error) {
	return workflow.Execute{}.EvaluateWith(ctx, pid, "acme::approval-granted", c.evaluate)
}

// evaluate is the live read from above, only asked while no answer is recorded at the position.
func (c ApprovalGranted) evaluate(ctx context.Context, pid workflow.ProcessID) (bool, error) { /* … */ }
```

The `ConditionID` you pass together with the path identifies the recorded
answer, so keep it stable across deployments, namespaced, and distinct from
other conditions that may be evaluated at the same position. Moving a
registered condition's logic into your own type under the same `ConditionID`
replays the answers it already gave. An empty ID is fatal, and so is setting
`ConditionID` or `ParticipantID` on the `Execute` as well (a fatal
`ErrInvalidDefinition`), since those fields belong to `Execute#Evaluate` and
`Execute#Execute`.

Your condition type is also a Definition field, so the same serialisation rules apply: name the
thing you need (`workflow.VarName`), never hold it. Register the type with your
[Codec][CODEC].

---

## 7. Choosing

| You want to…                                        | Use                                  |
| ---------------------------------------------------- | ------------------------------------ |
| Branch on domain logic with a recorded answer        | `If` with an `Execute` condition, or a custom condition answering through `EvaluateWith` |
| Let end users write a rule over variables            | `If` with `wftemplate.Condition`     |
| Wait until something changes                         | `Sleep` with any of them             |
| Re-check on every replay, deliberately               | a custom condition that records nothing |
| Read process variables in a rule                     | `Execute` inputs, a template, or your own type |

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
