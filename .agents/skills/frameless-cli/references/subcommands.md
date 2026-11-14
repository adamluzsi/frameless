# Sub-commands: command groups with `ServeMux.Sub`

This is a complete, verified example of a command group: `llmate task add write the readme` puts a task up for the next daemon, and `llmate task list` shows what is waiting. It has the adapter, `main`, and a `go.llib.dev/testcase` spec that runs the real routing table in-process.

The pattern in one place:

```go
var mux cli.ServeMux
taskmux := mux.Sub("task")
// so `llmate task add write the readme` puts a task up for the next daemon.
taskmux.Handle("add", TaskAddCommand{Tasks: tasks})
```

Each mux level takes one word from `r.Args` and routes on it. The root mux takes `task` and hands the rest to `taskmux`. `taskmux` takes `add` and runs the full pipeline (parse, defaults, validation, help, middlewares) for `TaskAddCommand`, with `write the readme` left as its input.

Replace `example.com/llmate` with your module path.

## `llmatecli/task.go`

```go
// Package llmatecli is the CLI adapter of llmate, an app that hands work to LLM daemons.
package llmatecli

import (
	"context"
	"fmt"
	"strings"

	"go.llib.dev/frameless/pkg/cli"
)

// Task is a unit of work that the next free daemon picks up.
type Task struct {
	Description string
	Priority    string
}

// TaskQueue is the domain port the task commands delegate to.
// The daemons consume what is enqueued here.
type TaskQueue interface {
	Enqueue(ctx context.Context, t Task) error
	Pending(ctx context.Context) ([]Task, error)
}

// NewMux wires every command with its dependencies.
// main and the tests both use it, so the tests exercise the real routing table.
func NewMux(tasks TaskQueue) *cli.ServeMux {
	var mux cli.ServeMux
	mux.WithSummary("hand work to LLM daemons")

	taskmux := mux.Sub("task")
	taskmux.WithSummary("manage the task queue")
	// so `llmate task add write the readme` puts a task up for the next daemon.
	taskmux.Handle("add", TaskAddCommand{Tasks: tasks})
	taskmux.Handle("list", TaskListCommand{Tasks: tasks})

	return &mux
}

type TaskAddCommand struct {
	Priority string   `flag:"priority,p" default:"normal" enum:"low,normal,high," desc:"how urgent the task is"`
	Words    []string `arg:"0:" desc:"the task, in plain words"`

	Tasks TaskQueue
}

func (TaskAddCommand) Summary() string { return "put a task up for the next daemon" }

func (cmd TaskAddCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	task := Task{
		Description: strings.Join(cmd.Words, " "),
		Priority:    cmd.Priority,
	}
	if err := cmd.Tasks.Enqueue(r.Context(), task); err != nil {
		cli.Error(w, err.Error(), cli.ExitCodeError)
		return
	}
	_, _ = fmt.Fprintf(w, "queued: %s\n", task.Description)
}

type TaskListCommand struct {
	Tasks TaskQueue
}

func (TaskListCommand) Summary() string { return "show the tasks waiting for a daemon" }

func (cmd TaskListCommand) ServeCLI(w cli.ResponseWriter, r *cli.Request) {
	tasks, err := cmd.Tasks.Pending(r.Context())
	if err != nil {
		cli.Error(w, err.Error(), cli.ExitCodeError)
		return
	}
	for _, t := range tasks {
		_, _ = fmt.Fprintf(w, "[%s] %s\n", t.Priority, t.Description)
	}
}
```

## `cmd/llmate/main.go`

```go
package main

import (
	"context"

	"go.llib.dev/frameless/pkg/cli"

	"example.com/llmate/llmatecli"
	"example.com/llmate/taskqueue"
)

func main() {
	// taskqueue.New is whichever TaskQueue adapter the daemons read from.
	cli.Main(context.Background(), llmatecli.NewMux(taskqueue.New()))
}
```

## Routing rules

- Name the sub-mux variable after its group, like `taskmux := mux.Sub("task")`. `Sub` returns a `*cli.ServeMux`, so `WithSummary`, `WithDescription`, `Handle` and `Sub` all work on it.
- `Sub` creates the sub-mux on the first call and returns the same one afterwards. Several functions, even several packages, can register commands into one group:

  ```go
  // taskcli/mux.go
  func Register(taskmux *cli.ServeMux, tasks TaskQueue) {
  	taskmux.WithSummary("manage the task queue")
  	taskmux.Handle("add", TaskAddCommand{Tasks: tasks})
  	taskmux.Handle("list", TaskListCommand{Tasks: tasks})
  }

  // llmatecli/mux.go
  taskcli.Register(mux.Sub("task"), tasks)
  ```

- A pattern with spaces is a path, so these pairs are the same:
  - `mux.Sub("task queue")` and `mux.Sub("task").Sub("queue")`
  - `mux.Handle("task add", h)` and `mux.Sub("task").Handle("add", h)`

  Prefer `Sub` when the group needs a summary, because only the sub-mux carries one.
- A name is either a command or a group of sub-commands, never both. Like `http.ServeMux`, registration panics on a misuse, and the message gives the file and line of both registrations:
  - `Handle` of a name that is already a command: `the "add" command is registered twice`;
  - `Handle` of a group's name, like `mux.Handle("task", ListCommand{})` next to `mux.Sub("task")`: `"task" is a group of sub-commands, so it can't be a command as well`;
  - `Sub`, or a `Handle` path like `"status verbose"`, through a name that is already a command: `"status" is a command, so it can't be a group of sub-commands as well`;
  - an empty pattern, a nil handler, or a nil `cli.HandlerFunc`.

  So a `git remote` / `git remote add` style, where the group itself runs something, is not possible. Register an explicit sub-command, like `task list`, instead.
  These panics fire while the mux is built, so a test that builds it through `NewMux` catches them without a dedicated case.
- Mounting a separately built `*cli.ServeMux` with `mux.Handle("task", taskmux)` works too, the way a handler is mounted on an `http.ServeMux`. To the parent, though, `task` is then a command, so a later `mux.Sub("task")` or `mux.Handle("task add", h)` panics. Prefer `Sub`, and pass the group to a `Register` function as above.
- Give every sub-mux a `WithSummary`. The parent's command list shows it as ` - task: manage the task queue`; without one, the line is just ` - task`.
- Argument order is: command path, then the command's flags, then its positionals.
  - `llmate task add -p high write the readme` is right.
  - `llmate -p high task add ...` fails with code 2 and `command is unknown: -p`, because a mux reads its first argument as a command name.
  - A mux has no flags of its own. To share flags across a group, embed a struct in each command (see "Declaring inputs" in `SKILL.md`). To share setup, like opening a client, use a `cli.Middleware` config field.
- Help, on stdout with code 0:
  - `llmate -h` lists `task` with its summary;
  - `llmate task -h` lists the sub-commands with their summaries;
  - `llmate task add -h` shows the command's options and arguments, and nothing runs.
- A group without a sub-command (`llmate task`), or with an unknown one (`llmate task nope`), is a bad request: code 2, with the group's own sub-commands on stderr. For an unknown name, a blank line follows the sub-commands, and stderr ends with `command is unknown: nope`.

```text
Usage: llmate task

manage the task queue

Commands:
 - add: put a task up for the next daemon
 - list: show the tasks waiting for a daemon
```

## Free-text input with `arg:"0:"`

`TaskAddCommand.Words` takes every word after the flags, so the user doesn't have to quote the task.

- Each word is kept as it is. Variadic args are not split on `,` the way list flags and env values are, so `fix the -v flag, then docs` stays intact.
- A variadic arg is required by default: `llmate task add` with no words is code 2 (`missing arguments for ...Words`), and nothing is queued. Add `default:"..."` or `optional:"true"` to allow it.
- After the first word, nothing is parsed as a flag. `llmate task add write the readme -p high` queues `write the readme -p high` at `normal` priority, without an error. Put flags first.
- If the text starts with a dash, put `--` before it: `llmate task add -- -v is broken`.

## `llmatecli/task_test.go`

Test sub-commands through the root `NewMux`, with the full command path in the args, so routing is covered as well.

```go
package llmatecli_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"go.llib.dev/frameless/pkg/cli"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"

	"example.com/llmate/llmatecli"
)

// FakeTaskQueue is a hand-written, in-memory fake of the TaskQueue port.
type FakeTaskQueue struct {
	Err error

	Tasks  []llmatecli.Task
	CtxErr error
}

func (q *FakeTaskQueue) Enqueue(ctx context.Context, t llmatecli.Task) error {
	q.CtxErr = ctx.Err()
	if q.Err != nil {
		return q.Err
	}
	q.Tasks = append(q.Tasks, t)
	return nil
}

func (q *FakeTaskQueue) Pending(ctx context.Context) ([]llmatecli.Task, error) {
	q.CtxErr = ctx.Err()
	if q.Err != nil {
		return nil, q.Err
	}
	return slices.Clone(q.Tasks), nil
}

func TestNewMux(t *testing.T) {
	s := testcase.NewSpec(t)

	var (
		queue = let.Var(s, func(t *testcase.T) *FakeTaskQueue {
			return &FakeTaskQueue{}
		})
		subject = let.Var(s, func(t *testcase.T) *cli.ServeMux {
			return llmatecli.NewMux(queue.Get(t))
		})
		ctx = let.Context(s)
	)

	serve := func(t *testcase.T, args ...string) *cli.ResponseRecorder {
		rr := &cli.ResponseRecorder{}
		req := &cli.Request{Args: slices.Clone(args), Body: strings.NewReader("")}
		t.OnFail(func() {
			t.Logf("args: %q\ncode: %d\nstdout:\n%s\nstderr:\n%s", args, rr.Code, rr.Out.String(), rr.Err.String())
		})
		cli.ServeCLI(subject.Get(t), rr, req.WithContext(ctx.Get(t)))
		return rr
	}

	s.Describe("task add", func(s *testcase.Spec) {
		args := let.Var(s, func(t *testcase.T) []string {
			return []string{"write", "the", "readme"}
		})

		act := func(t *testcase.T) *cli.ResponseRecorder {
			// the command path comes first, then the command's flags, then its positional args
			return serve(t, append([]string{"task", "add"}, args.Get(t)...)...)
		}

		s.Then("the words become one task for the next daemon", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Equal(t, rr.Out.String(), "queued: write the readme\n")
			assert.Empty(t, rr.Err.String())
			assert.Equal(t, queue.Get(t).Tasks, []llmatecli.Task{
				{Description: "write the readme", Priority: "normal"},
			})
		})

		s.When("the priority flag is given before the words", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-p", "high", "write", "the", "readme"}
			})

			s.Then("the task is queued with that priority", func(t *testcase.T) {
				act(t)
				assert.Equal(t, queue.Get(t).Tasks, []llmatecli.Task{
					{Description: "write the readme", Priority: "high"},
				})
			})
		})

		s.When("a flag comes after the first word", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"write", "the", "readme", "-p", "high"}
			})

			s.Then("it is part of the task, not a flag", func(t *testcase.T) {
				act(t)
				assert.Equal(t, queue.Get(t).Tasks, []llmatecli.Task{
					{Description: "write the readme -p high", Priority: "normal"},
				})
			})
		})

		s.When("the task starts with a dash", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"--", "-v", "is", "broken"}
			})

			s.Then("-- ends the flags, so the dash word is kept", func(t *testcase.T) {
				act(t)
				assert.Equal(t, queue.Get(t).Tasks, []llmatecli.Task{
					{Description: "-v is broken", Priority: "normal"},
				})
			})
		})

		s.When("no words are given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return nil })

			s.Then("it is a bad request and nothing is queued", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.Contains(t, rr.Err.String(), "Words")
				assert.Empty(t, queue.Get(t).Tasks)
			})
		})

		s.When("the priority is not an accepted value", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string {
				return []string{"-p", "asap", "write", "the", "readme"}
			})

			s.Then("it is rejected as a bad request and nothing is queued", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.Contains(t, rr.Err.String(), "high") // accepted values are listed
				assert.Empty(t, queue.Get(t).Tasks)
			})
		})

		s.When("the queue fails", func(s *testcase.Spec) {
			expErr := let.Error(s)
			queue.Let(s, func(t *testcase.T) *FakeTaskQueue {
				return &FakeTaskQueue{Err: expErr.Get(t)}
			})

			s.Then("the error goes to stderr with the general error exit code", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeError)
				assert.Equal(t, rr.Err.String(), expErr.Get(t).Error()+"\n")
				assert.Empty(t, rr.Out.String())
			})
		})

		s.When("the context is cancelled", func(s *testcase.Spec) {
			ctx.Let(s, func(t *testcase.T) context.Context {
				c, cancel := context.WithCancel(ctx.Super(t))
				cancel()
				return c
			})

			s.Then("the cancellation reaches the queue", func(t *testcase.T) {
				act(t)
				assert.ErrorIs(t, queue.Get(t).CtxErr, context.Canceled)
			})
		})

		s.When("help is requested", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return []string{"-h"} })

			s.Then("the command help goes to stdout and nothing is queued", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeOK)
				assert.Contains(t, rr.Out.String(), "-priority")
				assert.Empty(t, queue.Get(t).Tasks)
			})
		})
	})

	s.Describe("task list", func(s *testcase.Spec) {
		queue.Let(s, func(t *testcase.T) *FakeTaskQueue {
			return &FakeTaskQueue{Tasks: []llmatecli.Task{
				{Description: "write the readme", Priority: "high"},
				{Description: "fix the flaky test", Priority: "low"},
			}}
		})

		act := func(t *testcase.T) *cli.ResponseRecorder {
			return serve(t, "task", "list")
		}

		s.Then("the pending tasks are printed", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Equal(t, rr.Out.String(), "[high] write the readme\n[low] fix the flaky test\n")
		})
	})

	s.Describe("task", func(s *testcase.Spec) {
		args := let.Var(s, func(t *testcase.T) []string {
			return []string{"-h"}
		})

		act := func(t *testcase.T) *cli.ResponseRecorder {
			return serve(t, append([]string{"task"}, args.Get(t)...)...)
		}

		s.Then("help lists the sub-commands with their summaries", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Contains(t, rr.Out.String(), " - add: put a task up for the next daemon")
			assert.Contains(t, rr.Out.String(), " - list: show the tasks waiting for a daemon")
		})

		s.When("no sub-command is given", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return nil })

			s.Then("it is a bad request with the sub-commands of the group on stderr", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.Contains(t, rr.Err.String(), " - add: put a task up for the next daemon")
				assert.Contains(t, rr.Err.String(), " - list: show the tasks waiting for a daemon")
				assert.Empty(t, queue.Get(t).Tasks)
			})
		})

		s.When("the sub-command is unknown", func(s *testcase.Spec) {
			args.Let(s, func(t *testcase.T) []string { return []string{"nope"} })

			s.Then("it is a bad request that names the unknown sub-command, and nothing runs", func(t *testcase.T) {
				rr := act(t)
				assert.Equal(t, rr.Code, cli.ExitCodeBadRequest)
				assert.Contains(t, rr.Err.String(), "command is unknown: nope")
				assert.Contains(t, rr.Err.String(), " - add: put a task up for the next daemon")
				assert.Empty(t, queue.Get(t).Tasks)
			})
		})
	})

	s.Describe("root", func(s *testcase.Spec) {
		act := func(t *testcase.T) *cli.ResponseRecorder {
			return serve(t, "-h")
		}

		s.Then("help lists the sub-mux with its summary", func(t *testcase.T) {
			rr := act(t)
			assert.Equal(t, rr.Code, cli.ExitCodeOK)
			assert.Contains(t, rr.Out.String(), " - task: manage the task queue")
		})
	})
}
```

## Scenarios per command group

| Scenario | Args | Assert |
| --- | --- | --- |
| Each sub-command, happy path | full path: `task add write the readme` | code 0, exact stdout, the fake got the bound values |
| Flags of a sub-command | `task add -p high ...` | the flag reaches the handler |
| Free text | words with dashes and commas, `--` before a leading dash | the text is kept verbatim |
| Group help | `task -h` | code 0, every sub-command with its summary |
| Root help | `-h` | code 0, the group with its summary |
| Missing or unknown sub-command | `task`, `task nope` | code 2, the group's sub-commands on stderr, `command is unknown: nope`, nothing ran |
| Registration conflicts | none, `NewMux` runs in every test | a duplicate or a command/group clash panics and fails the whole suite |
