# Getting Started

> Goal: a workflow running on your machine in about five minutes, with no
> database, no broker, and no config file.

**The whole package in one sentence:** you write ordinary Go functions
(**Participants**), your users compose them into a serialisable value
(**Definition**), and the engine (**Runtime**) executes that value one step at a
time, recording every step as an event so it can safely resume after a crash.

---

## 1. Hello, workflow

Copy this into a `main.go` and run it. Everything is in-memory, so there is
nothing to install or start. About fifty lines, with one of everything: an
engine, two participants, a three-step workflow, and a `main` that runs it.

The four `Runtime` fields at the top — `Events`, `ProcessExecutionQueue`,
`ProcessChangeBroadcast`, `ProcessLockers` — are the *plumbing*: where the
event log lives, where queued executions wait, how workers learn something
changed, and how a process is locked to a single worker at a time. The
`memory.*` values are stand-ins for whatever you run in production (Postgres,
Kafka, Redis, etc.). You only swap those four fields when you deploy; the
workflow code never changes.

```go
package main

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/adapter/memory"
	"go.llib.dev/frameless/pkg/workflow"
)

func main() {
	ctx := context.Background()

	// 1. The engine, wired to in-memory dependencies.
	rt := workflow.Runtime{
		Events:                 &memory.WorkflowEventRepository{},
		ProcessExecutionQueue:  &memory.WorkflowProcessExecutionQueue{},
		ProcessChangeBroadcast: &memory.WorkflowProcessChangeBroadcast{},
		ProcessLockers:         &memory.LockerFactory[workflow.ProcessID, workflow.ProcessLock]{},

		// 2. The capabilities you expose. Just Go functions.
		Participants: workflow.Participants{
			"greet": func(ctx context.Context, name string) (string, error) {
				return "Hello, " + name + "!", nil
			},
			"print": func(ctx context.Context, msg string) error {
				fmt.Println(msg)
				return nil
			},
		},
	}

	// 3. The workflow itself. An ordinary Go value.
	def := workflow.Sequence{
		workflow.SetVar{Name: "name", Value: "World"},
		workflow.ExecuteParticipant{
			ID:     "greet",
			Input:  []workflow.VarName{"name"},
			Output: []workflow.VarName{"greeting"},
		},
		workflow.ExecuteParticipant{
			ID:    "print",
			Input: []workflow.VarName{"greeting"},
		},
	}

	// 4. Give it an identity, bind the definition, run it.
	pid, err := workflow.MakeProcessID()
	if err != nil {
		panic(err)
	}
	if err := rt.Bind(ctx, pid, def); err != nil {
		panic(err)
	}
	if err := rt.Execute(ctx, pid); err != nil {
		panic(err)
	}
}
```

The `print` participant is what produces the line below — the engine called it
with the result of `greet`.

```
Hello, World!
```

That is a complete workflow engine. No other setup exists.

---

## 2. What just happened

Read the `Sequence` top to bottom — that is the execution order:

| Step                             | Effect                                                         |
| -------------------------------- | -------------------------------------------------------------- |
| `SetVar{Name: "name"}`           | Writes `name = "World"` into the process' variables.           |
| `ExecuteParticipant{ID: "greet"}` | Reads `name`, calls your `greet` func, stores the result in `greeting`. |
| `ExecuteParticipant{ID: "print"}` | Reads `greeting`, calls your `print` func. Returns no value, so no `Output`. |

### Where is the state?

In the event log. Every step above appended events to `rt.Events` (the first
field on `Runtime`), and that log **is** the process state — there is no
separate "process status" anywhere. You can read it back:

```go
vars := workflow.Vars{ProcessID: pid, EventsRepository: rt.Events}
m, err := vars.ToMap(ctx) // map[name:World greeting:Hello, World!]
```

This is why the engine can survive a crash: re-running `rt.Execute(ctx, pid)`
replays the log, skips the steps that already have events, and resumes exactly
where it stopped. Try adding a second `rt.Execute(ctx, pid)` to the example —
`Hello, World!` is **not** printed twice.

### How `Input` and `Output` work

`Input` and `Output` are the wiring between the variables and your function's
signature. The rule is positional, not by name:

```go
ExecuteParticipant{
    ID:     "greet",
    Input:  []workflow.VarName{"name"},     //  → 1st arg after ctx
    Output: []workflow.VarName{"greeting"}, //  ← 1st result before error
}
//   ↳ maps onto: func(ctx context.Context, name string) (greeting string, error)
```

`Input` is matched positionally to the parameters after `ctx`; `Output` to the
results before the trailing `error`. A mismatch — say, two `Output` names for a
function that returns only one value — is caught at execution time with an
explicit error, not a panic.

---

## 3. Three moving parts, three owners

The package draws a hard line between three roles. Keeping them straight is 90%
of understanding the design.

| Part           | Authored by            | What it is                                    |
| -------------- | ---------------------- | --------------------------------------------- |
| **Participant** | Developers, you        | A Go function. The system's capabilities.     |
| **Definition** | Workflow builders      | A serialisable value. The composition.        |
| **Runtime**    | This package           | Executes definitions, records events, retries. |

The "workflow builders" persona is whoever composes workflows in your system —
it could be you writing Go, or it could be an ops engineer editing JSON, or a
non-developer clicking through a builder UI. The package deliberately does not
assume that person can redeploy your binary; that is the whole reason a
`Definition` is a value and not code.

The point of the split: **definitions are data**, so they can be built in a UI,
sent over HTTP, stored in your database, versioned, and audited — without
redeploying your Go binary. See [End Users][END_USER] for why this shapes the
whole API.

---

## 4. Branch on a yes/no

There are two moving parts here, and a first-time reader often conflates them:

- **Condition** — the *test* (a Go function or a template expression that
  answers yes/no). Registered on the runtime, just like a Participant.
- **`If`** — a *Definition* that runs one branch or the other based on a
  Condition's answer. Composable, like `Sequence`.

Register the test as a Go function:

```go
rt.Conditions = workflow.Conditions{
	"is-vip": func(ctx context.Context, customerID string) (bool, error) {
		return customerID == "cust-1", nil
	},
}
```

Then branch with `If`. `Input` works the same way it does for Participants —
positionally onto the Condition's parameters:

```go
def := workflow.Sequence{
	workflow.SetVar{Name: "customer_id", Value: "cust-1"},
	workflow.If{
		Cond: workflow.ExecuteCondition{
			ID:    "is-vip",
			Input: []workflow.VarName{"customer_id"},
		},
		Then: workflow.ExecuteParticipant{ID: "greet",
			Input:  []workflow.VarName{"customer_id"},
			Output: []workflow.VarName{"greeting"}},
		Else: workflow.ExecuteParticipant{ID: "print",
			Input: []workflow.VarName{"customer_id"}},
	},
}
```

When the rule is Go logic you own, this is the form to reach for. When the rule
is something a workflow builder should be able to edit without a redeploy —
"if the order is over $100, ask for approval" — swap the registered Go function
for an inline template expression against the process variables:

```go
workflow.If{
	Cond: wftemplate.Condition(`gt .order_total 100`),
	Then: workflow.ExecuteParticipant{ID: "ask-for-approval",
		Input: []workflow.VarName{"order_id"}},
}
```

`ExecuteCondition` for logic you own, `wftemplate.Condition` for rules the
builder should be able to edit. [Conditions][CONDITION] covers the trade-off
in full.

---

## 5. Hand the work to a worker

`rt.Execute` runs the process right here, on the calling goroutine, and
returns when it is done. That is perfect for tests and for a request handler
that wants a synchronous answer, but it gives you nothing across two practical
boundaries:

- **A crash.** If the goroutine running `Execute` dies mid-step, the work
  vanishes. Nothing has picked the process up to finish it.
- **A pause.** The engine has no concept of "do this later" while the call is
  blocked on your participant. A long-running step *is* a long-running call.

Both problems have the same shape: the work needs to leave the calling
goroutine and live somewhere that survives the call. The queue already exists
— `ProcessExecutionQueue`, the second field on `Runtime` — but until now
nothing has read from it. That is what `rt.Run` does. It is a worker: it
blocks, reads entries off the queue, and runs each one. Call it once at
startup and let it run for the lifetime of the process.

```go
// In your application's startup, on every node that should execute workflows:
go func() {
    if err := rt.Run(ctx); err != nil {
        log.Fatal(err)
    }
}()
```

`Run` returns when `ctx` is cancelled — that is how you stop the worker.

Now the producer side. Anywhere in your application, instead of calling
`Execute`, you call `Schedule`. `Schedule` does not run the process; it puts
an entry on the queue and returns, so a free worker can pick it up:

```go
// Anywhere in your application.
pid, _ := workflow.MakeProcessID()
_ = rt.Bind(ctx, pid, def)   // records the definition
_ = rt.Schedule(ctx, pid)   // publishes the entry; returns immediately
```

That is the whole change from §1: `Execute` becomes `Schedule`, and a worker
loop is running somewhere consuming the queue. The two roles can live in the
same process or in different ones — `Schedule` is just a publisher, `Run` is
just a consumer.

### The one rule to keep straight

> **`ProcessID` is yours, not the runtime's.** The runtime refuses a zero
> `ProcessID` on purpose. If your `Schedule` call times out and you retry,
> *reuse the same ID* — otherwise you have silently started a second process
> with the same definition. Owning the ID is what makes the retry safe; the
> engine has no other way to tell your retry from a brand-new submission.

The plumbing you wired in §1 stays. Because `memory.*` implements the same
interfaces as a Postgres- or Kafka-backed implementation, swapping those four
fields in `Runtime` is the only code change when you go to deploy the same
workflow against real infrastructure.

---

## 6. Pausing until later

A participant can put itself back on the queue by returning a signal instead of
`nil`. The simplest is `Suspend{}`:

```go
"wait-for-approval": func(ctx context.Context, id string) error {
	approved, err := db.IsApproved(ctx, id)
	if err != nil {
		return err        // a real failure — the runtime will retry
	}
	if !approved {
		return workflow.Suspend{} // not a failure — put me back on the queue
	}
	return nil
},
```

A `Suspend{}` is an `error`, but it is not a *failure*: the runtime recognises
it and re-enqueues the process instead of counting it against the retry budget.
The worker picks it up again after `Runtime#WaitTime` and walks the definition
from the top, replaying the steps that already recorded until it reaches the
suspending one.

### When you want to stop asking altogether

For waiting on something the runtime cannot poll on its own — a manual review,
a customer reply — use `workflow.Halt{}` instead. It stops the current pass
without requeueing: the queue entry is acknowledged and dropped, and the
Process is left inert in the log. Resuming it is the caller's job, by calling
`rt.Schedule(ctx, pid)` again with the same `ProcessID`.

`Suspend` says *"ask me again later, on your schedule"*; `Halt` says *"do not
ask again, full stop"*. See [Signals][SIGNAL] for the full catalogue, plus
`workflow.Replace{Definition}` (swap the running definition) and
`workflow.Sleep` (a `Suspend` wrapped around a condition).

---

## 7. Calling off a process

`Suspend{}` and `Halt{}` are both participant-raised signals. Sometimes the
*caller* on the outside needs to say *"stop"* — cancel a long-running
onboarding, abort a checkout that timed out, withdraw a submission the user
gave up on. That is `rt.Terminate`:

```go
if err := rt.Terminate(ctx, pid); err != nil {
    log.Print(err)
}
```

`rt.Terminate` writes a single `EventTerminated` entry to the log and returns.
Three guarantees fall out of how the runtime treats it:

| Guarantee                                | What it means in practice                                                                                                              |
| ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| **Idempotent**                           | Calling it on an already-terminated process is a no-op — no second `EventTerminated`, the log stays a reliable answer to *when* it stopped. |
| **Refuses to overwrite a completed process** | If the process ran to its natural end first, the call does nothing. The two outcomes — *finished* and *called off* — stay distinct. |
| **Cancels in-flight work**               | If a worker is mid-step, the process lock is acquired; the runtime publishes a `ProcessCancel` over the change broadcast, and the in-flight participant sees its `ctx` cancelled and unwinds. |

Once terminated, the process keeps its full event history — `Bind`, every step
that did record, and the trailing `EventTerminated`. Re-running `rt.Execute` on
it is a no-op: the runtime sees the terminal event on the first scan of the log
and returns immediately, without re-invoking the definition. That is what makes
"called off" a safe, auditable outcome.

### How it compares to Suspend and Halt

| Caller says      | API                                | Process ends up…       | Resumed by                |
| ---------------- | ---------------------------------- | ---------------------- | ------------------------- |
| "ask me again"   | return `Suspend{}` from a participant | requeued, retrying     | the worker, after `WaitTime` |
| "stop asking"    | return `Halt{}` from a participant | inert in the log       | `rt.Schedule(ctx, pid)`   |
| "call this off"  | `rt.Terminate(ctx, pid)` from outside | `EventTerminated` on the log | never — it is terminal    |

To read the outcome from elsewhere, ask the log:

```go
done, err := workflow.IsCompleted(ctx, rt.Events, pid)  // ran to its natural end
ended, err := workflow.IsTerminated(ctx, rt.Events, pid) // was called off via Terminate
```

Both answers stay `false` for a Halted Process — a Halt is intentionally not
recorded as an outcome; "is this process done?" is not the right question for
one. See [Signals][SIGNAL] for the full signal catalogue.

---

## 8. Where to go next

Pick the one that matches your next question:

| Your question                                          | Read                             |
| ------------------------------------------------------ | -------------------------------- |
| "What are all the building blocks I can compose?"      | [Definitions][DEFINITION]        |
| "How do I expose my domain logic properly?"            | [Participants][PARTICIPANT]      |
| "What is that word again?"                             | [Glossary][GLOSSARY]             |
| "How do variables and scoping actually work?"          | [Variables][VARIABLES]           |
| "How do I pause, stop, or swap a running workflow?"    | [Signals][SIGNAL]                |
| "How do I write conditions?"                           | [Conditions][CONDITION]          |
| "How do I test all of this?"                           | [Testing][TESTING]               |
| "How do I persist definitions to my own database?"     | [Codec][CODEC]                   |
| "Why is it designed this way?"                         | [End Users][END_USER]            |

[GLOSSARY]: ./glossary.md
[DEFINITION]: ./definition.md
[PARTICIPANT]: ./participant.md
[CONDITION]: ./condition.md
[VARIABLES]: ./vars.md
[CODEC]: ./codec.md
[END_USER]: ./end-user.md
[TESTING]: ./testing.md
[SIGNAL]: ./signal.md
