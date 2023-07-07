package workflow

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/zerokit"
)

// Variables are the data elements that are used and manipulated during the execution of a workflow.
// They can represent inputs and outputs of tasks, intermediate results,
// or any other data that needs to be tracked throughout the workflow.
//
// The reason why the Variables type is not a generic type is because
type Variables map[VariableKey]any
type VariableKey string

// Event is the occurrence that can trigger changes in the workflow.
// For example, the completion of a task could be an event that triggers the start of the next task.
type Event any

type ConditionCheck[VS Variables] func(ctx context.Context, vs VS) (bool, error)

// Engine is the software that interprets the process definition and controls the execution of the workflow.
// It manages the state of the workflow, the assignment of tasks to participants, and the evaluation of conditions.
//
// Engine is also the (API) interface through which external applications interact with the workflow engine.
// It allows for starting new workflow instances, querying the state of existing instances,
// and performing other operations.
type Engine struct {
	Repository Repository

	pRegister pRegister
}

type pRegister map[ParticipantID]regParticipant

type Repository interface {
	ProcessDefinitions() ProcessDefinitionRepository
	Instances() InstanceRepository
}

func (engine *Engine) getPRegister() pRegister {
	return zerokit.Init(&engine.pRegister, func() pRegister {
		return make(pRegister)
	})
}

func (engine *Engine) Exec(ctx context.Context, pdef ProcessDefinition) (InstanceID, error) {

	return "", nil
}

func (engine *Engine) RegisterParticipant(id ParticipantID, fn Participant) error {
	regParticipant, err := makeRegParticipant(fn)
	if err != nil {
		return err
	}
	engine.getPRegister()[id] = regParticipant
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type TaskFunc[VS Variables] func(context.Context, VS) error

func (fn TaskFunc[VS]) Do(ctx context.Context, vs VS) error {
	return fn(ctx, vs)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type UseParticipant struct {
	ID   ParticipantID
	Args []Value
	Out  []VariableKey
}

func (task UseParticipant) Visit(visitor func(Task)) { visitor(task) }
