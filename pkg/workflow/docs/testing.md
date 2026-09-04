# Testing

> Goal: test a whole workflow — domain logic, composition, and infrastructure —
> in a plain `go test` run, with nothing to install or start.

That is a design goal of this package, not a happy accident. Every runtime
dependency has an in-memory implementation in `adapter/memory`, participants are
ordinary functions, and definitions are ordinary values. If a workflow needs a
broker to be testable, something has gone wrong.

There are **three layers**, and each has a different answer:

| What you wrote                  | How you test it                       |
| ------------------------------- | ------------------------------------- |
| A [Participant][PARTICIPANT]    | As a normal Go function. §1           |
| A [Definition][DEFINITION] composition | `wftest` + `adapter/memory`. §2 |
| An adapter (Postgres, Kafka, …) | `wfcontract` contract suites. §3      |

---

## 1. Participants: just functions

```go
func Greet(ctx context.Context, name string) (string, error) {
	return "Hello, " + name + "!", nil
}

func TestGreet(t *testing.T) {
	got, err := Greet(context.Background(), "World")
	assert.NoError(t, err)
	assert.Equal(t, "Hello, World!", got)
}
```

No runtime. No event repository. No fixtures.

This is the whole point of the participant design: your domain logic never
imports the engine, so it never needs the engine to be tested. If you find
yourself booting a `Runtime` to test business rules, move those rules into a
participant and test them here instead.

---

## 2. Compositions: `wftest.LetC`

`wftest.LetC` builds a fully wired, in-memory runtime as `testcase.Var` fields:

```go
func TestOrderWorkflow(t *testing.T) {
	s := testcase.NewSpec(t)

	c := wftest.LetC(s)

	charge, chargeID := wftest.LetParticipant(s, c,
		func(t *testcase.T) func(context.Context, string) error {
			return func(ctx context.Context, orderID string) error { return nil }
		})

	c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
		return workflow.Sequence{
			workflow.SetVar{Name: "order_id", Value: "42"},
			workflow.ExecuteParticipant{
				ID:    chargeID.Get(t),
				Input: []workflow.VarName{"order_id"},
			},
		}
	})

	act := let.Act(func(t *testcase.T) error {
		return c.ActExecute(t)
	})

	s.Then("the process is completed", func(t *testcase.T) {
		assert.NoError(t, act(t))

		assert.True(t, c.IsCompleted(t, c.ProcessID.Get(t)))
	})

	s.When("the charge fails", func(s *testcase.Spec) {
		expErr := let.Error(s)

		charge.Let(s, func(t *testcase.T) func(context.Context, string) error {
			return func(ctx context.Context, orderID string) error {
				return expErr.Get(t)
			}
		})

		s.Then("the error is propagated back", func(t *testcase.T) {
			assert.ErrorIs(t, act(t), expErr.Get(t))
		})
	})
}
```

### What `LetC` did for you

Two things happen without you asking, and both matter:

- `Runtime#Run` is started in the background for each test case, so
  `Schedule`, `Spawn`, and suspension all work.
- `c.Definition` is bound to `c.ProcessID` in a `Before` hook, so a process is
  ready to execute the moment the test body starts.

It also sets a no-retry strategy and a nanosecond wait time, so a failing
participant fails once, immediately, instead of retrying through your test
timeout.

### The `C` fields

Every one is a `testcase.Var`, so every one can be overridden with `.Let(s, …)`
inside a nested spec block.

| Field                    | What it holds                                      |
| ------------------------ | -------------------------------------------------- |
| `ProcessID`              | A unique process ID for this test case.            |
| `Definition`             | The definition under test. Default: a trivial `SetVar`. |
| `Runtime`                | The wired `workflow.Runtime`.                      |
| `Participants`           | The participant map. `LetParticipant` appends here. |
| `Conditions`             | The condition map.                                 |
| `ContextSetup`           | Context decorators applied per execution.          |
| `ErrRuntimeRun`          | The error the background `Run` returned, if any.   |
| `EventRepository`        | `*memory.WorkflowEventRepository` — the state.     |
| `ProcessExecutionQueue`  | `*memory.WorkflowProcessExecutionQueue`            |
| `ProcessChangeBroadcast` | `*memory.WorkflowProcessChangeBroadcast`           |
| `ProcessLocks`           | `*memory.LockerFactory[...]`                       |

### The `C` methods

| Method                                    | Use it to…                                       |
| ----------------------------------------- | ------------------------------------------------ |
| `c.ActExecute(t)`                         | Bind + execute `c.Definition` on `c.ProcessID`.  |
| `c.ActExecuteDefinition(t, ctx, pid, def)` | Same, for an ad-hoc process/definition pair.    |
| `c.ProcessEvents(t, pid)`                 | Read the recorded event history.                 |
| `c.IsCompleted(t, pid)`                   | Check completion **now** (synchronous).          |
| `c.ProcessCompletionIs(t, pid, done)`     | Await completion state (for background work).    |
| `c.ChildrenCompletionAre(t, pid, done)`   | Await the same for every spawned child.          |
| `c.WaitForSpawn(t, parentID)`             | Block until a spawn event appears.               |
| `c.ThenProcessIsCompleted(s, v, act)`     | Declare a ready-made `Then` block.               |
| `c.ThenProcessIsNotCompleted(s, v, act)`  | The negative counterpart.                        |
| `c.LetContext(s)`                         | A runtime-decorated `context.Context` var.       |

`IsCompleted` vs `ProcessCompletionIs` is the distinction worth remembering:
after `ActExecute` the work is already done, so ask directly. After `Schedule`,
`Spawn`, or a suspension, the work is on the background runtime, so *await* it —
`ProcessCompletionIs` retries for up to a second.

### Overriding a participant per scenario

The idiom used in the example above is worth calling out, because it is what
keeps scenario tests small:

```go
charge, chargeID := wftest.LetParticipant(s, c, /* happy path */)

s.When("the charge fails", func(s *testcase.Spec) {
	charge.Let(s, /* failing version */)
	// ...
})
```

This works because `c.Participants`' initialiser resolves `charge` **lazily, at
test time**. The map is built when the runtime is built, so whichever version of
the func var is in scope is the one that gets registered. You never re-register.

Use `LetParticipantWithID` when you need the ID up front (e.g. to reference the
same participant from several places), and `LetParticipantID` alone when you only
need an ID.

### Stubbing a Definition or Condition

`wftest.Stub` implements **both** `Definition` and `Condition`, so one value
covers either slot:

```go
c.Definition.Let(s, func(t *testcase.T) workflow.Definition {
	return wftest.Stub{
		StubExecute: func(ctx context.Context, pid workflow.ProcessID) error {
			return workflow.Suspend{}
		},
	}
})
```

Leave a field nil for a benign default: `Execute` returns `nil`, `Evaluate`
returns `true`.

### Testing a Definition in isolation

You do not have to go through the runtime. `c.LetContext(s)` hands you a context
carrying the runtime, which is everything a `Definition#Execute` needs:

```go
var (
	ctx     = c.LetContext(s)
	process = wftest.LetProcessID(s)
)

s.Test("it executes", func(t *testcase.T) {
	def := workflow.SetVar{Name: "answer", Value: 42}

	assert.NoError(t, def.Execute(ctx.Get(t), process.Get(t)))

	assert.NotEmpty(t, c.ProcessEvents(t, process.Get(t)))
})
```

### Other helpers

| Helper                    | Returns                                             |
| ------------------------- | --------------------------------------------------- |
| `wftest.LetProcessID(s)`  | A unique `ProcessID` var.                            |
| `wftest.LetProcess(s)`    | A fresh `ProcessID` var (no uniqueness bookkeeping). |
| `wftest.LetValue(s)`      | A random `any` — handy for variable payloads.        |
| `wftest.MakeEventID(tb)`  | A valid `EventID`, error already asserted.           |

**Avoid these — they are deprecated:** `wftest.LetProcessWithDefinition` (use
`c.Definition` + `c.ActExecute`), `wftest.StubParticipant` and
`c.LetStubParticipant` (use `wftest.LetParticipant`).

---

## 3. Adapters: don't write those tests yourself

Backing `EventRepository`, `ProcessExecutionQueue`, `ProcessLocks`, or
`ProcessChangeBroadcast` with Postgres, Kafka, or Redis? The behaviour those
interfaces demand is already written down as executable contracts:

```go
func TestWorkflowEventRepository(t *testing.T) {
	var subject memory.WorkflowEventRepository
	wfcontract.EventRepository(&subject).Test(t)
}

func TestWorkflowProcessLocks(t *testing.T) {
	subject := &memory.LockerFactory[workflow.ProcessID, workflow.ProcessLock]{}
	t.Run("implements workflow ProcessLocks", wfcontract.ProcessLocks(subject).Test)
}
```

Each returns a `contract.Contract`. Call `.Test(t)`, or pass the `.Test` method
value to `t.Run` for a named subtest. To fold one into a larger spec, use
`testcase.RunSuite(s, wfcontract.EventRepository(&subject))`.

### The catalogue

| Contract                                     | Subject                            |
| -------------------------------------------- | ---------------------------------- |
| `wfcontract.EventRepository(subject)`         | Your event store.                  |
| `wfcontract.ProcessExecutionQueue(subject, opts...)` | Your scheduler queue.       |
| `wfcontract.ProcessChangeBroadcast(subject, opts...)` | Your change-notification channel. |
| `wfcontract.ProcessLocks(subject, opts...)`   | Your per-process locker factory.   |
| `wfcontract.Definition(mk)`                   | Your own `Definition` type.        |
| `wfcontract.Codec(codec)`                     | Your own wire format. See [Codec][CODEC]. |

### The requirements you would not have guessed

This is why running the contract beats writing your own tests:

- **`ProcessExecutionQueue` must not block on acknowledgement.** The runtime
  publishes to the queue from inside execution. A queue that waits for a
  subscriber to ACK deadlocks the whole engine.
- **`ProcessExecutionQueue` must deliver ordered by `StartTime` ascending.**
  That ordering *is* the scheduling: a process suspended until later must not
  overtake one that is due now.
- **`ProcessChangeBroadcast` must be volatile, not durable.** The runtime both
  publishes and subscribes here. Replaying old change events to a reconnecting
  subscriber would resurrect stale wake-ups.
- **`EventRepository` must reject events with a zero ID, process ID, or
  timestamp.** The history is the state; an event that cannot be ordered or
  attributed corrupts every replay that follows.
- **`EventRepository#FindByProcessID` must return only that process' events,
  ordered by timestamp ascending.** Replay walks that sequence in order. Wrong
  order means the engine reconstructs a state the process was never in.
- **`ProcessLocks` must isolate per `ProcessID`.** Locking process A must never
  block process B, or a single slow workflow stalls the node.

### Contract-testing your own Definition

If you write a `Definition` type, `wfcontract.Definition` checks the two things
the runtime relies on — that it executes, and that re-executing it does not
append duplicate events:

```go
func TestSendInvoice(t *testing.T) {
	wfcontract.Definition(func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
		return SendInvoice{CustomerID: "cust-1"}
	}).Test(t)
}
```

If your definition wraps other definitions, use `c.MakeDefinition(tb)` for the
children — it returns a stub that asserts it was handed the right process ID and
a **non-empty path**:

```go
type Retry struct {
	Definition workflow.Definition
}

func (Retry) Error() string { return "Retry" }

func (d Retry) Execute(ctx context.Context, pid workflow.ProcessID) error {
	ctx = workflow.WithName(ctx, "retry") // ← required
	return d.Definition.Execute(ctx, pid)
}
```

```go
func TestRetry(t *testing.T) {
	wfcontract.Definition(func(tb testing.TB, c wfcontract.DefinitionContext) workflow.Definition {
		return Retry{Definition: c.MakeDefinition(tb)}
	}).Test(t)
}
```

Drop the `WithName` line and the contract fails. That is the point: the path is
how the engine tells one step from another in the event history, so a composite
definition that forwards its context unchanged makes its child indistinguishable
from its sibling — and the idempotency replay then skips the wrong step. Every
built-in composite (`Sequence`, `If`, `Sleep`, `Spawn`) does the same thing.

### Fixture builders

For your own assertions, `wfcontract` exposes the same random builders its
contracts use. All of them take a `testing.TB` and draw from the `testcase`
seed, so failures reproduce under `TESTCASE_SEED`:

| Builder                             | Produces                                  |
| ----------------------------------- | ----------------------------------------- |
| `wfcontract.MakeDefinition(tb)`     | Any registered `Definition`, possibly nested. |
| `wfcontract.MakeLeafDefinition(tb)` | A `Definition` that holds no other one.   |
| `wfcontract.MakeCondition(tb)`      | Any registered `Condition`.               |
| `wfcontract.MakeEvent(tb, pid)`     | An `Event` for the given process.         |

---

## 4. Where to go next

| Your question                                | Read                        |
| -------------------------------------------- | --------------------------- |
| "How do I persist definitions at all?"       | [Codec][CODEC]              |
| "What can I compose?"                        | [Definitions][DEFINITION]   |
| "How should I shape my participants?"        | [Participants][PARTICIPANT] |
| "How do I pause or swap a running workflow?" | [Signals][SIGNAL]           |
| "Start me from the top."                     | [Getting Started][GETTING_STARTED] |

[GETTING_STARTED]: ./getting-started.md
[DEFINITION]: ./definition.md
[PARTICIPANT]: ./participant.md
[CODEC]: ./codec.md
[SIGNAL]: ./signal.md
