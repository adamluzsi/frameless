# Participants

> A **Participant** is your domain logic: an ordinary Go function, registered
> under a name, that a workflow definition can call. It is how you publish
> capabilities to whoever composes workflows.

```go
rt := workflow.Runtime{
	Participants: workflow.Participants{
		"charge-card": func(ctx context.Context, orderID string) (string, error) {
			return payments.Charge(ctx, orderID)
		},
	},
}
```

```go
workflow.Execute{
	ParticipantID: "charge-card",
	Input:         []workflow.VarName{"order_id"},
	Output:        []workflow.VarName{"receipt"},
}
```

Nothing about the function is workflow-specific. It takes a context, it takes
arguments, it returns results and an error. That is the point — your domain
logic stays testable and reusable without the engine.

---

## 1. The signature

```
func(context.Context, arg1 T1, arg2 T2, ...) (Result1, Result2, ..., error)
```

- The first parameter must be `context.Context`.
- Everything after it is filled from `Input`, positionally.
- Everything returned before the trailing `error` is written to `Output`,
  positionally.
- All argument and result types must be serialisable by the [Codec][CODEC] —
  they are recorded in the event history.

```go
Input:  []workflow.VarName{"order_id"}  //  ─┐
Output: []workflow.VarName{"receipt"}   //  ─┼─► func(ctx, orderID string) (receipt string, error)
```

| Function                                       | `Input` | `Output` |
| ---------------------------------------------- | ------- | -------- |
| `func(ctx) error`                              | 0       | 0        |
| `func(ctx, string) error`                      | 1       | 0        |
| `func(ctx, string) (string, error)`            | 1       | 1        |
| `func(ctx, string, int) (string, bool, error)` | 2       | 2        |

Variadic functions accept zero optional arguments, individual optional values,
or one matching trailing slice. For
`func(ctx context.Context, id string, labels ...string) error`, `Input` can name
just `id`, `id` plus individual label
variables, or `id` plus a `[]string` variable. A matching trailing slice takes
precedence. An untyped `nil` is an individual argument, not an omitted slice;
a typed nil slice can supply a nil variadic slice. Wrong arity, incompatible
types, and nil passed to a non-nilable parameter return errors, not panics.

### When it goes wrong

| Problem                                | Error                                   |
| -------------------------------------- | --------------------------------------- |
| No participant with that `ParticipantID` (or no `Participants` repository) | `ErrParticipantNotFound{ID: id}` — nonfatal availability error |
| Incompatible `Input` arity/type or `Output` count | `ErrParticipantSignatureMismatch{ID, Cause}` wrapping `ErrParticipantFuncMappingMismatch` |
| An `Input` variable is not set         | `ErrFatal` naming the missing variable  |
| Invalid registered function (including a nil function) | `ErrParticipantSignatureMismatch{ID, Cause}` wrapping `ErrInvalidParticipantFunc` |

All of them are explicit error values. A mapping mistake never panics.

### Missing or incompatible registration waits for availability

`workflow.ErrParticipantNotFound{ID: id}` retains its type, name and missing `ID`.
It is a **plain, nonfatal availability error**: it does not implement
`workflow.RuntimeSignal` and does not match `errors.Is(err, workflow.Suspend{})`.
Participant lookup returns it when an uncached step's ID is not registered,
including when no `Participants` repository is configured.

`workflow.ErrParticipantSignatureMismatch{ID: id, Cause: err}`, defined in
`wferror.go`, likewise reports a **nonfatal availability error** when this node's
registration cannot satisfy the invocation. `Cause` preserves the legacy
invalid-function or mapping error for `errors.Is`/`errors.As`; `ErrIsFatal`
recognises the wrapper as nonfatal even when its cause is `ErrInvalidParticipantFunc`.

For both errors, the runtime skips tight local retries and returns the error
from `Runtime.Execute`. The scheduler in `Runtime.Run` requeues after `WaitTime`
and **increments `ExecutionRequest.FailureCount`**. The idempotent executor
preserves earlier work, including nested follow-up successes, and records
neither a failed `EventParticipant` nor an `EventError` for the unavailable
invocation. A later attempt reuses completed steps and tries lookup again.

`Runtime.ParticipantWarningInterval` logs a warning every N scheduling failures
of this kind (default **5**; zero or negative values use that default). There is
no hard failure-count drop limit and no capability-routing guarantee. A typo or
incompatible deployment can therefore wait indefinitely; validate IDs and
signatures independently. `Suspend`, unlike these errors, preserves `FailureCount`.

Missing conditions remain fatal (`ErrConditionNotFound`). See
[End Users][END_USER] for shared execution and independent ID validation.

---

## 2. Calls are cached

This is the most important thing to internalise.

When a participant returns successfully, the runtime records an
`EventParticipant` holding the `ParticipantID`, the `Path`, the `Input` values
and the `Output` values. On a later execution of the same process, a step that
finds its matching event **does not call your function** — it replays the
recorded output.

```go
_ = rt.Execute(ctx, pid) // charge-card runs
_ = rt.Execute(ctx, pid) // charge-card does NOT run; its receipt is replayed
```

This reuse depends on the event being durably committed. It is not an
exactly-once guarantee for effects outside the event repository.

A recorded call is only reused when the step is genuinely the same:

- the same **path** in the definition tree,
- the same **number** of inputs and outputs, and
- the same **input values**, evaluated as of that point in the history.

That last one is subtler than it looks: the comparison uses the values as they
were *when the call was recorded*, not as they are now. Assigning a new value to
an input variable later in the process does not invalidate an earlier recorded
call — which is exactly what keeps a replay stable.

### The trap it does not save you from

The cache protects recorded workflow results, not external effects. Even a
participant that returns `nil` may run again if its remote effect succeeds but
the event write or commit fails, the process crashes before persistence, or an
enclosing event transaction rolls back. Make external operations idempotent and
safe to retry with a **stable business idempotency key** (not a fresh ID on each
attempt), or coordinate the effect and event record through a shared transaction
or transactional outbox where applicable.

```go
// ⚠️ Fragile
"place-order": func(ctx context.Context) (string, error) {
	id := newOrderID()                   // step 1
	return id, remote.Submit(ctx, id)    // step 2 — if this fails...
},
```

If `Submit` fails, nothing is recorded, and the retry calls the whole function
again — minting a **new** `id`. If you needed the same ID on the retry, split
the work so each part is separately recorded:

```go
// Use these as separate top-level Sequence steps.
"place-order": func(ctx context.Context) (string, error) {
	return newOrderID(), nil
},
"submit-order": func(ctx context.Context, id string) error {
	return remote.Submit(ctx, id)
},
```

As separate top-level `Sequence` steps, `place-order` commits before
`submit-order` is attempted, so a later retry reuses the recorded ID. The submit
operation still needs to tolerate repetition if its own success event cannot
be committed. Returning a `Sequence` from a participant does **not** provide
the same independent commit boundaries.

---

## 3. Returning more work

A participant may return a `Definition` **as its error value** — that is why
`Definition` embeds `error`. It means "I'm finished, and this step needs
follow-up stages":

```go
"reserve-stock": func(ctx context.Context, orderID string) error {
	if err := stock.Reserve(ctx, orderID); err != nil {
		return err
	}
	return workflow.Sequence{
		workflow.Execute{ParticipantID: "pick", Input: []workflow.VarName{"order_id"}},
		workflow.Execute{ParticipantID: "pack", Input: []workflow.VarName{"order_id"}},
	}
},
```

The runtime records the participant's execution **with the returned definition
attached**, then executes it in place of the step. A `Sequence{A, B, C}` where
`B` does this effectively becomes `Sequence{A, B, B', C}`.

On a cache hit, the runtime skips the producer function but **walks the recorded
follow-up definition again**, under the original path. Recorded nested steps
reuse their results, and unfinished or suspended steps resume; the outer
sequence continues only when the follow-up succeeds. Dispatch is not exactly once.

The follow-up still runs inside the producer's event transaction. `Suspend`
and participant availability errors preserve earlier work when that transaction
commits. An ordinary nested failure can instead roll back the producer's newly
recorded event and earlier nested success events from that transaction. Those
calls may run again, even though their external effects already happened.
Use separate top-level steps when you need independent commit boundaries, and
keep external effects safe to retry in either layout.

---

## 4. Returning a signal

A participant may also return a [signal][SIGNAL] — `Suspend`, `Replace` — to
steer the runtime rather than report a result:

```go
"wait-for-approval": func(ctx context.Context, orderID string) error {
	approved, err := approvals.IsGranted(ctx, orderID)
	if err != nil {
		return err
	}
	if !approved {
		return workflow.Suspend{}
	}
	return nil
},
```

**`Suspend` and `Halt` are not recorded as successful calls.** The step stays
uncached, so the next pass calls your function again and asks the question afresh. That is exactly what you want
for waiting — a cached `Suspend` would replay forever — but it does mean the
work before the signal runs again too. Keep the pre-signal part cheap and
side-effect free.

| You return      | Recorded? | Called again next pass? |
| --------------- | --------- | ----------------------- |
| `nil`           | Yes       | No                      |
| a `Definition`  | Yes       | Producer: no; follow-up: walked again |
| `Suspend` / `Halt` | No    | Yes, if execution resumes |
| `Replace`       | Yes, new definition | The replacement runs after its event commits |
| a plain `error` | No        | Yes (retried)           |

The cache rows assume a matching, durably committed event. `Replace` records
a new definition; it does not make any preceding external effect exactly once.

---

## 5. Failing

Ordinary operational errors go through `Runtime#RetryStrategy`. Missing or
incompatible participant registrations are handled separately, without immediate retry.

When retrying cannot possibly help — a validation failure, a rejected payment,
a malformed definition — say so, and the runtime stops immediately:

```go
"charge-card": func(ctx context.Context, orderID string) (string, error) {
	receipt, err := payments.Charge(ctx, orderID)
	if errors.Is(err, payments.ErrCardDeclined) {
		return "", workflow.ErrFatal.F("card declined for %s", orderID)
	}
	return receipt, err
},
```

`workflow.ErrIsFatal(err)` is the check the runtime performs; wrapping with
`ErrFatal` is how you opt in. `Runtime.Execute` returns the error without local
retry. In scheduled execution, the worker logs a warning and ACKs/drops that
queue entry rather than requeueing it or stopping the worker. This does **not**
complete or terminate the process: it remains incomplete and nonterminated,
and you can call `Schedule` again with the same ID after addressing the cause.

---

## 6. Reaching the process from inside

Your function receives the execution context, so it can reach the process:

```go
"audit": func(ctx context.Context) error {
	pid, _ := workflow.ProcessIDFromContext(ctx)

	vars, err := workflow.GetVars(ctx)
	if err != nil {
		return err
	}
	return vars.Set(ctx, "audited_at", time.Now().Format(time.RFC3339))
},
```

Prefer `Input`/`Output` mapping when you can — it keeps the data flow visible in
the definition, which is the part your end users read. Reach for
[`GetVars`][VARIABLES] when the set of variables is dynamic.

---

## 7. Designing the vocabulary

`Participants` is a public API surface for whoever composes workflows. A few
things follow from that:

- **Name them for the domain**, not for the implementation. `charge-card`, not
  `call-stripe-v2`.
- **Assume they will be composed in an order you did not anticipate.** That is
  the whole point of end-user composition, and it is why idempotency and
  `ErrFatal` matter.
- **Keep them coarse enough to be meaningful** and fine enough to be reusable.
- **They are the trust boundary.** A definition can only *name* the
  participants you registered — it cannot express arbitrary code. That is what
  makes accepting end-user-authored definitions safe. See [End Users][END_USER].
- **Not every side effect belongs in the vocabulary.** A step private to one
  custom definition can run as that definition's own code through
  `Execute#ExecuteWith`, cached like any participant call. See
  [Custom Definitions][CUSTOM_DEFINITION] §4.

---

## 8. Testing them

A participant is a plain Go function. Test it as one — no runtime, no event
repository, no fixtures:

```go
func TestChargeCard(t *testing.T) {
	got, err := chargeCard(context.Background(), "ORD-42")
	assert.NoError(t, err)
	assert.Equal(t, "RCPT-1", got)
}
```

Test the *composition* separately, with `wftest`. See [Testing][TESTING].

---

## 9. Where to go next

| Your question                                     | Read                      |
| ------------------------------------------------- | ------------------------- |
| "What can call my participant?"                   | [Definitions][DEFINITION] |
| "How do I pause or redirect from inside one?"     | [Signals][SIGNAL]         |
| "How do the variables work?"                      | [Variables][VARIABLES]    |
| "What can I put in an argument?"                  | [Codec][CODEC]            |
| "How do I test the whole workflow?"               | [Testing][TESTING]        |
| "Why is it designed this way?"                    | [End Users][END_USER]     |

[DEFINITION]: ./definition.md
[CUSTOM_DEFINITION]: ./custom-definition.md
[SIGNAL]: ./signal.md
[VARIABLES]: ./vars.md
[CODEC]: ./codec.md
[TESTING]: ./testing.md
[END_USER]: ./end-user.md
