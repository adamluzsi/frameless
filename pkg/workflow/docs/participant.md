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
workflow.ExecuteParticipant{
	ID:     "charge-card",
	Input:  []workflow.VarName{"order_id"},
	Output: []workflow.VarName{"receipt"},
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

### When it goes wrong

| Problem                                | Error                                   |
| -------------------------------------- | --------------------------------------- |
| No participant with that `ID`          | `ErrParticipantNotFound`                |
| `Input`/`Output` count ≠ the signature | `ErrParticipantFuncMappingMismatch`     |
| An `Input` variable is not set         | `ErrFatal` naming the missing variable  |
| The registered value is not a func     | `ErrInvalidParticipantFunc`             |

All of them are explicit errors. A mapping mistake never panics.

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

That is what makes retries, crashes and double-delivery safe by default.

A recorded call is only reused when the step is genuinely the same:

- the same **path** in the definition tree,
- the same **number** of inputs and outputs, and
- the same **input values**, evaluated as of that point in the history.

That last one is subtler than it looks: the comparison uses the values as they
were *when the call was recorded*, not as they are now. Assigning a new value to
an input variable later in the process does not invalidate an earlier recorded
call — which is exactly what keeps a replay stable.

### The trap it does not save you from

The cache protects the *workflow*. It does not protect a half-finished side
effect inside a single participant.

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
// ✅ Each step is retried independently
"place-order": func(ctx context.Context) (string, error) {
	return newOrderID(), nil
},
"submit-order": func(ctx context.Context, id string) error {
	return remote.Submit(ctx, id)
},
```

Now `place-order` is recorded before `submit-order` is attempted, so a retry
reuses the ID it already minted. Returning a `Sequence` (below) achieves the
same thing from inside a single participant.

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
		workflow.ExecuteParticipant{ID: "pick", Input: []workflow.VarName{"order_id"}},
		workflow.ExecuteParticipant{ID: "pack", Input: []workflow.VarName{"order_id"}},
	}
},
```

The runtime records the participant's execution **with the returned definition
attached**, then executes it in place of the step. A `Sequence{A, B, C}` where
`B` does this effectively becomes `Sequence{A, B, B', C}`.

Because the swap is part of the recorded event, a replay is a no-op — the
follow-up is dispatched exactly once, and each of its steps then gets its own
retry and caching behaviour. This is the idiomatic way to make a multi-part
operation individually retryable.

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

**A signal is never recorded.** The step stays uncached, so the next pass calls
your function again and asks the question afresh. That is exactly what you want
for waiting — a cached `Suspend` would replay forever — but it does mean the
work before the signal runs again too. Keep the pre-signal part cheap and
side-effect free.

| You return      | Recorded? | Called again next pass? |
| --------------- | --------- | ----------------------- |
| `nil`           | Yes       | No                      |
| a `Definition`  | Yes       | No                      |
| a `RuntimeSignal` | No      | Yes                     |
| a plain `error` | No        | Yes (retried)           |

---

## 5. Failing

A plain error is treated as transient and goes through `Runtime#RetryStrategy`.

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
`ErrFatal` is how you opt in.

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
[SIGNAL]: ./signal.md
[VARIABLES]: ./vars.md
[CODEC]: ./codec.md
[TESTING]: ./testing.md
[END_USER]: ./end-user.md
