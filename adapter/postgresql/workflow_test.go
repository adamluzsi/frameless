package postgresql_test

import (
	"context"
	"testing"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfcontract"
	"go.llib.dev/testcase/assert"
)

func Test_workflowE2E(t *testing.T) {
	ctx := context.Background()
	c := GetConnection(t)

	events := &postgresql.WorkflowEventRepository{Connection: c}
	assert.NoError(t, events.Migrate(ctx))

	queue := &postgresql.WorkflowQueue{Connection: c}
	assert.NoError(t, queue.Migrate(ctx))
	t.Cleanup(func() {
		_, _ = c.ExecContext(ctx, `DELETE FROM frameless_queue_messages WHERE queue = 'frameless_workflow_queue'`)
	})

	changes := &postgresql.WorkflowNotificationBroadcast{}
	locks := postgresql.WorkflowProcessLocks{Connection: c}
	assert.NoError(t, locks.Migrate(ctx))

	rt := workflow.Runtime{
		Events:             events,
		Queue:              queue,
		Notifications:      changes,
		Locks:              locks,
		NumQueueSubscriber: 1,
		Participants: workflow.Participants{
			"greet": func(ctx context.Context, name string) (string, error) {
				return "Hello, " + name + "!", nil
			},
		},
	}

	pid, err := workflow.MakeProcessID()
	assert.NoError(t, err)

	def := workflow.Sequence{
		workflow.SetVar{Name: "name", Value: "World"},
		workflow.ExecuteParticipant{
			ID:     "greet",
			Input:  []workflow.VarName{"name"},
			Output: []workflow.VarName{"greeting"},
		},
	}

	assert.NoError(t, rt.Bind(ctx, pid, def))
	assert.NoError(t, rt.Execute(ctx, pid))

	completed, err := workflow.IsCompleted(ctx, events, pid)
	assert.NoError(t, err)
	assert.True(t, completed, assert.MessageF(
		"runtime should persist EventCompleted for the process after a successful execution"))

	got, err := iterkit.CollectE(events.FindByProcessID(ctx, pid))
	assert.NoError(t, err)
	assert.NotEmpty(t, got)

	var sawUseDefinition, sawParticipant, sawCompleted bool
	for _, event := range got {
		switch event.(type) {
		case workflow.EventUseDefinition:
			sawUseDefinition = true
		case workflow.EventParticipant:
			sawParticipant = true
		case workflow.EventCompleted:
			sawCompleted = true
		}
	}
	assert.True(t, sawUseDefinition, assert.MessageF("Bind should persist EventUseDefinition"))
	assert.True(t, sawParticipant, assert.MessageF("Execute should persist EventParticipant from participant step"))
	assert.True(t, sawCompleted, assert.MessageF("Execute should persist EventCompleted on natural completion"))

	vmap, err := workflow.Vars{ProcessID: pid, EventsRepository: events}.ToMap(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "World", vmap["name"])
	assert.Equal(t, "Hello, World!", vmap["greeting"])
}

func TestWorkflowProcessLocks(t *testing.T) {
	c := GetConnection(t)
	subject := postgresql.WorkflowProcessLocks{Connection: c}
	assert.NoError(t, subject.Migrate(t.Context()))
	wfcontract.ProcessLocks(subject).Test(t)
}

func TestWorkflowQueue(t *testing.T) {
	c := GetConnection(t)
	subject := &postgresql.WorkflowQueue{Connection: c}
	assert.NoError(t, subject.Migrate(t.Context()))
	wfcontract.Queue(subject).Test(t)
}

func TestWorkflowNotificationBroadcast(t *testing.T) {
	c := GetConnection(t)
	subject := &postgresql.WorkflowNotificationBroadcast{Connection: c}
	wfcontract.NotificationBroadcast(subject).Test(t)
}

func TestWorkflowEventRepository(t *testing.T) {
	c := GetConnection(t)
	subject := &postgresql.WorkflowEventRepository{Connection: c}
	assert.NoError(t, subject.Migrate(t.Context()))
	wfcontract.EventRepository(subject).Test(t)
}
