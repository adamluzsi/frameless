package postgresql_test

import (
	"context"
	"strings"
	"testing"

	"go.llib.dev/frameless/adapter/postgresql"
	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/workflow"
	"go.llib.dev/frameless/pkg/workflow/wfcontract"
	"go.llib.dev/frameless/port/pubsub/pubsubcontract"
	"go.llib.dev/testcase"
	"go.llib.dev/testcase/assert"
	"go.llib.dev/testcase/let"
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
	s := testcase.NewSpec(t)
	var (
		stateless = let.Var(s, func(t *testcase.T) bool { return false })
		poolSize  = let.Var(s, func(t *testcase.T) int32 { return 8 })
		subject   = let.Var(s, func(t *testcase.T) *postgresql.WorkflowNotificationBroadcast {
			return &postgresql.WorkflowNotificationBroadcast{
				Connection:         queueV2Connection(t, poolSize.Get(t)),
				Name:               "wf_" + strings.ReplaceAll(t.Random.UUID(), "-", ""),
				StatelessSubscribe: stateless.Get(t),
			}
		})
	)

	var tests = func(t *testcase.T) {
		testcase.RunSuite(testcase.NewSpec(t.TB),
			wfcontract.NotificationBroadcast(subject.Get(t), pubsubcontract.Config[workflow.Notification]{
				MakeContext: workflowNotificationContext,
			}))
	}

	s.Test("satisfies the notification contract using LISTEN by default", tests)

	s.When("StatelessSubscribe is enabled", func(s *testcase.Spec) {
		stateless.LetValue(s, true)

		s.Test("satisfies the notification contract without LISTEN", tests)

		s.Context("stateless life-cycle", func(s *testcase.Spec) {
			poolSize.LetValue(s, 1)
			workflowNotificationSpec(s, subject, poolSize)
		})
	})
}

func TestWorkflowEventRepository(t *testing.T) {
	c := GetConnection(t)
	subject := &postgresql.WorkflowEventRepository{Connection: c}
	assert.NoError(t, subject.Migrate(t.Context()))
	wfcontract.EventRepository(subject).Test(t)
}
