package workflow

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/errorkit"
	"github.com/adamluzsi/frameless/ports/crud"
	"github.com/adamluzsi/frameless/ports/pubsub"
)

// Participant is the entity that perform tasks in the workflow.
// Participants can be human users, groups of users, or even automated systems.
// The workflow engine needs to manage the assignment of tasks to participants and track their progress.
type Participant interface {
	Do(context.Context) error
}
type ParticipantID string

//func Visit[VS Variables](p ProcessDefinition[VS], visitor func()) {
//	visitor()
//
//	//if task := p.Task(); task != nil {
//	//	p.Visit(func(p Participant[VS]) { Visit[VS](p, visitor) })
//	//}
//}

// ProcessDefinition is the blueprint of your workflow.
// It defines the sequence of tasks that need to be executed,
// the conditions under which they should be executed, and the order in which they should occur.
// This is typically represented in a graphical form using a flowchart or a similar diagram.
type ProcessDefinition struct {
	ID   ProcessDefinitionID
	Task Task
}
type ProcessDefinitionID string

// Task is the individual unit of work that need to be performed in a workflow.
// Each task has a defined start and end point,
// and may have preconditions that need to be met before it can be executed.
type Task interface {
	Visit(visitor func(Task))
}

type TaskID string

type Sequence []Task

func (s Sequence) Visit(fn func(Task)) {
	fn(s)
	for _, t := range s {
		t.Visit(fn)
	}
}

func (s Sequence) Do(fn func(Task) error) error {
	for _, task := range s {
		if err := fn(task); err != nil {
			return err
		}
	}
	return nil
}

func Concurrence() Task { return nil }

//func Sequence() Task { return nil }

type If struct {
	Cond func(context.Context) (bool, error)
	Then Task
	Else Task
}

func (ifcond If) Visit(fn func(Task)) {
	fn(ifcond)
	if ifcond.Then != nil {
		ifcond.Then.Visit(fn)
	}
	if ifcond.Else != nil {
		ifcond.Else.Visit(fn)
	}
}

type While struct {
	Cond  func(context.Context) (bool, error)
	Block Task
}

func (l While) Visit(visitor func(Task)) {
	visitor(l)
	if l.Block != nil {
		l.Block.Visit(visitor)
	}
}

const Break errorkit.Error = "workflow: While -> Break"

// Variables are the data elements that are used and manipulated during the execution of a workflow.
// They can represent inputs and outputs of tasks, intermediate results,
// or any other data that needs to be tracked throughout the workflow.
type Variables any

// Event is the occurrence that can trigger changes in the workflow.
// For example, the completion of a task could be an event that triggers the start of the next task.
type Event any

type EventStream[E Event] interface {
	pubsub.Publisher[E]
	pubsub.Subscriber[E]
	crud.Purger
}

type ConditionCheck[VS Variables] func(ctx context.Context, vs VS) (bool, error)

// Instance is each run of a workflow according to a particular process definition.
// The workflow engine needs to manage multiple instances, each with its own state and variables.
type Instance[VS Variables] struct {
	ID                  InstanceID `ext:"id"`
	ProcessDefinitionID ProcessDefinitionID
	ParticipantID       *ParticipantID
	VS                  Variables
}
type InstanceID string

type InstanceRepository[VS Variables] interface {
	crud.Creator[Instance[VS]]
	crud.Finder[Instance[VS], InstanceID]
	crud.Updater[Instance[VS]]
	crud.Deleter[InstanceID]
}

// Engine is the software that interprets the process definition and controls the execution of the workflow.
// It manages the state of the workflow, the assignment of tasks to participants, and the evaluation of conditions.
//
// Engine is also the (API) interface through which external applications interact with the workflow engine.
// It allows for starting new workflow instances, querying the state of existing instances,
// and performing other operations.
type Engine struct{}

func RegisterParticipant[VS Variables](name ParticipantID, participant Participant[VS]) {

}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type TaskFunc[VS Variables] func(context.Context, VS) error

func (fn TaskFunc[VS]) Do(ctx context.Context, vs VS) error {
	return fn(ctx, vs)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func WaitOnEvent[VS Variables, E Event]() Participant[VS] {
	return Participant[VS]{}
}

type UseParticipant struct {
	ID            TaskID
	ParticipantID ParticipantID
}

func (task UseParticipant) Visit(visitor func(Task)) { visitor(task) }
