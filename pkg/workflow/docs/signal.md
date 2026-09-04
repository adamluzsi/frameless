# Signals

> A **RuntimeSignal** is an `error` value that is not a failure. It is the
> runtime's own control flow — a step's way of saying *"pause me"*, *"we're
> done"*, or *"run this instead"*; and the caller's way of saying *"stop it"*.

```go
type RuntimeSignal interface {
	error
	RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error
}
```

Go has no second return channel for "not a failure, but not finished either", so
the package borrows the error channel and marks the value. The runtime checks
for the marker before it treats anything as a fault.

---

## 1. The built-ins

| Signal                        | Meaning                          | What the runtime does                          |
| ----------------------------- | -------------------------------- | ---------------------------------------------- |
| `workflow.Suspend{}`          | "Come back to me later."         | Re-queues the process. **Not** a failure count. |
| `workflow.Complete{}`         | "This process is finished."      | Records `EventCompleted`.                      |
| `workflow.Replace{Definition}` | "Run this definition instead."   | Appends a new `EventUseDefinition`.            |
| `workflow.Halt{}`             | "Stop asking me about this one." | ACKs the queue entry and drops it. The Process is left inert; the caller resumes it by calling `rt.Schedule` again. |
| `workflow.Terminate{}`        | "Call this process off."         | Records `EventTerminated`. Raised externally by `rt.Terminate`. |

Return one from a participant like any other error:

```go
"wait-for-approval": func(ctx context.Context, orderID string) error {
	approved, err := approvals.IsGranted(ctx, orderID)
	if err != nil {
		return err // a real failure — will be retried
	}
	if !approved {
		return workflow.Suspend{} // a signal — will be re-queued
	}
	return nil
},
```

Signals bubble up through the call stack — through `Sequence`, `If`, nested
definitions — and are handled by the runtime itself, before any other error is
surfaced.

---

## 2. The one rule everything follows from

**A signal is not an error.** Four behaviours fall out of that, and each one
would be a bug if it went the other way.

### It is not retried

`Runtime#withRetry` returns a signal immediately instead of counting it against
the retry budget.

Suspending means "come back later", and coming back later is the *scheduler's*
job — the runtime re-queues a suspended process **without incrementing its
failure count**, precisely because a suspension is not a failure. Burning retry
attempts on it would contradict that, and it is not free: each attempt walks
straight back to the waiting step and asks it again with no delay.

### It does not roll back

Step transactions are **layered**: steps nest, and every nesting level opens its
own transaction around the events it records. There are no checkpoints between
the layers — a rollback is reported upwards as a failure, the level above takes
it for a rainy case and rolls back in turn, all the way to the top.

So rolling back on a signal would not undo "the signal". It would discard every
event recorded by every enclosing step, because one step at the bottom used
control flow. Those events describe work that genuinely happened.

### It is never recorded

This is the surprising one, and it is deliberate.

When a participant returns a signal, **no `EventParticipant` is written**. The
step is not cached, so it is re-executed on the next pass.

> A signal is a *question about right now*. "Should I still be waiting?" can only
> be answered at the moment it is asked. If the runtime cached the answer, a
> `Suspend` would replay as a `Suspend` forever and the process could never wake
> up.

Contrast this with a participant returning a `Definition`, which *is* a result —
the participant finished and produced the next stage — so that call **is**
recorded and replayed. That is the dividing line between the two "happy errors":

| Returned value | Meaning                       | Recorded? | Re-run on next pass? |
| -------------- | ----------------------------- | --------- | -------------------- |
| `Definition`   | "I'm done; here's what's next" | Yes       | No — replayed        |
| `RuntimeSignal` | "Ask me again"                | No        | Yes                  |

### It is handled before other errors

`Runtime#execute` type-asserts the signal, calls `RuntimeSignalExecute`, and — if
that succeeds — loops back and executes the process again with whatever the
signal changed.

---

## 3. What each signal actually does

### Suspend — pause and come back

```go
return workflow.Suspend{}
```

`RuntimeSignalExecute` returns *itself*. That makes `Suspend` terminal for this
pass: it propagates out of `Execute` unchanged, and the scheduler re-queues the
process to be picked up again after `Runtime#WaitTime`.

`workflow.Sleep` is `Suspend` wrapped in a definition, so you rarely raise it
by hand for time-based waiting:

```go
workflow.Sleep{Until: ApprovalGranted{Order: "order_id"}}
```

Reach for a bare `Suspend{}` when the waiting logic lives inside your own
participant.

### Complete — finish early

```go
return workflow.Complete{}
```

Records `EventCompleted` for the process, unless one is already present — so it
is idempotent. The runtime raises this one itself when a definition runs to the
end, which is how normal completion is recorded.

> **Caution:** do not return `Complete` from a participant. A participant's
> signal is never recorded, so the runtime marks the process complete, loops
> back, re-runs the same definition, reaches the same uncached participant, and
> gets `Complete` again — indefinitely. Use `Replace` with an empty
> `workflow.Sequence{}` if you need a definition to stop early.

### Replace — swap the definition

```go
return workflow.Replace{Definition: workflow.Sequence{
	workflow.ExecuteParticipant{ID: "manual-review"},
}}
```

Appends a new `EventUseDefinition`. The runtime then re-executes the process,
which picks up the *latest* definition — the new one.

This is the mechanism behind migrations and human-in-the-loop flows. It is also
why old events are never rewritten: the process' definition history is the
sequence of `EventUseDefinition` entries, and it stays fully auditable.

> Because a step's identity is rooted at the current definition's event ID, a
> `Replace` re-roots every path. The replacement therefore starts from its own
> beginning rather than fast-forwarding through the previous definition's
> recorded steps. See [Definitions][DEFINITION] §5.

### Halt — stop asking

```go
return workflow.Halt{}
```

The no-reschedule cousin of `Suspend`. Both stop the current pass, and neither
is recorded as a step outcome, but `Suspend` says *"ask me again later, on your
schedule"* while `Halt` says *"do not ask again, full stop"*. The runtime
acknowledges the queue entry and walks away; the Process is left **inert** in
the event log — not completed, not retried, not requeued.

Resuming a Halted Process is the *caller's* job: call `rt.Schedule(ctx, pid)`
again with the same `ProcessID`, and the runtime will re-execute the
definition from the beginning. A re-Scheduled Halted Process is indistinguishable
from a freshly bound one: the queue subscriber picks up the new entry, the
definition runs from the top, and the Halt-raising step is asked again. That
is the only way out of a Halt.

The scheduler recognises `Halt{}` by value (`errors.Is(err, Halt{})`), so a
wrapped `Halt` is no longer a Halt — it falls through to the failure path on
purpose, and pinning that here keeps any future "be liberal in what you accept"
change visible. The signal is the contract; the scheduler decides what to do
with it.

> **Caution:** the *inert* state is invisible to the query helpers. `IsCompleted`
> and `IsTerminated` both stay `false` for a Halted Process — a Halted Process
> is neither finished nor called off, it is paused mid-flight on purpose. If
> you want to query "should we resume?", you have to track that out-of-band
> (for example by writing a `SetVar` from the Halt-raising participant before
> it returns).

### Terminate — call off the process

`Terminate{}` is the runtime's *external* terminal signal. Unlike `Suspend`,
`Complete`, and `Replace` — which a participant raises by returning the signal —
`Terminate{}` is meant to be raised from the *caller*: anywhere in your code
that has a `ProcessID` and a reason to stop.

```go
if err := rt.Terminate(ctx, pid); err != nil {
    log.Print(err)
}
```

`rt.Terminate` acquires the process lock, publishes a `ProcessCancel` over the
change broadcast if a worker is mid-flight (so the in-flight participant sees
its `ctx` cancelled and unwinds), and then runs `Terminate{}.RuntimeSignalExecute`,
which appends one `EventTerminated` and returns.

Three properties fall out of the implementation, and each is load-bearing:

| Property                                   | Why it is required                                                                                                                                                                |
| ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Idempotent on a terminated process**     | A duplicate call does not grow a second `EventTerminated`. Otherwise the log stops being a reliable answer to *when* the process was stopped.                                       |
| **Refuses to overwrite a completed process** | A process that ran to its natural end keeps `EventCompleted`; the call returns without writing `EventTerminated`. The two outcomes stay distinct.                                  |
| **Append-only**                            | Termination never removes prior events. The history of a terminated process still reads `Bind` → every step that did record → `EventTerminated`, fully auditable.             |

Once the `EventTerminated` is on the log, the next `rt.Execute(ctx, pid)` short-
circuits in its event scan and returns without re-invoking the definition. That
is what keeps re-running a terminated process safe — there is no "what state was
the participant in?" to reconstruct, because nothing is allowed to run again.

To check the outcome from elsewhere, pair it with the matching query:

```go
done, err := workflow.IsCompleted(ctx, rt.Events, pid)  // ran to its end
ended, err := workflow.IsTerminated(ctx, rt.Events, pid) // was called off
```

A history that holds *both* an `EventCompleted` and an `EventTerminated` — which
can only arise from outside the runtime, since `Complete` and `Terminate` each
refuse to write over the other's outcome — answers `false` from *both* queries.
The two outcomes are not ordered; whichever lands first is the answer the log
gives. See [Getting Started][GETTING_STARTED] §7 for the short version.

> **Caution:** the package's supported path for terminating a process is the
> external `rt.Terminate(ctx, pid)` method, which acquires the process lock and
> publishes a `ProcessCancel` over the change broadcast so an in-flight worker
> unwinds. A participant can technically return `workflow.Terminate{}` — the
> signal satisfies the `RuntimeSignal` interface — but the package does not
> document or test that path. Treat it as undefined behaviour: prefer
> `rt.Terminate` from the caller.

---

## 4. Writing your own

Implement `error` plus `RuntimeSignalExecute`:

```go
type Escalate struct {
	To string
}

var _ workflow.RuntimeSignal = Escalate{}

func (Escalate) Error() string { return "acme::escalate" }

func (sig Escalate) RuntimeSignalExecute(ctx context.Context, rt workflow.Runtime, id workflow.ProcessID) error {
	return workflow.Replace{Definition: workflow.ExecuteParticipant{
		ID:    "notify-oncall",
		Input: []workflow.VarName{"incident_id"},
	}}.RuntimeSignalExecute(ctx, rt, id)
}
```

Two rules to respect:

1. **Change something.** After `RuntimeSignalExecute` returns nil, the runtime
   re-executes the process. If the next pass reaches the same step in the same
   state, you have written an infinite loop. Either change what executes (like
   `Replace`), or be terminal (like `Suspend`, which returns itself).
2. **Do the work in `RuntimeSignalExecute`, not in `Error()`.** `Error()` may be
   called for logging at any time; the runtime calls `RuntimeSignalExecute`
   exactly once per signal.

---

## 5. Signal or error?

| Situation                                    | Return                                      |
| -------------------------------------------- | ------------------------------------------- |
| Transient failure, worth retrying            | the plain `error`                           |
| Permanent failure, retrying cannot help      | `workflow.ErrFatal` (or wrap with it)       |
| Not a failure — the step needs to wait       | `workflow.Suspend{}`                        |
| Not a failure — the step should not be asked again until the caller re-schedules | `workflow.Halt{}`                          |
| Not a failure — the workflow should change   | `workflow.Replace{...}`                     |
| Not a failure — there is follow-up work      | return a `Definition` (see [Participants][PARTICIPANT]) |
| Call the process off from the outside       | `rt.Terminate(ctx, pid)` (see [Getting Started][GETTING_STARTED] §7) |

---

## 6. Where to go next

| Your question                                   | Read                        |
| ----------------------------------------------- | --------------------------- |
| "How do I return one from my code?"             | [Participants][PARTICIPANT] |
| "What does `Sleep` do exactly?"                 | [Definitions][DEFINITION]   |
| "How do I test a suspending workflow?"          | [Testing][TESTING]          |
| "What is that word again?"                      | [Glossary][GLOSSARY]        |

[GLOSSARY]: ./glossary.md
[DEFINITION]: ./definition.md
[PARTICIPANT]: ./participant.md
[TESTING]: ./testing.md
[GETTING_STARTED]: ./getting-started.md
