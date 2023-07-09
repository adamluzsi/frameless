package workflow

import (
	"context"
	"github.com/adamluzsi/frameless/pkg/zerokit"
)

// Vars are the data elements that are used and manipulated during the execution of a workflow.
// They can represent inputs and outputs of tasks, intermediate results,
// or any other data that needs to be tracked throughout the workflow.
//
// The reason why the Vars type is not a generic type is because
type Vars map[VarKey]any
type VarKey string

// Event is the occurrence that can trigger changes in the workflow.
// For example, the completion of a task could be an event that triggers the start of the next task.
type Event any

type ConditionCheck[VS Vars] func(ctx context.Context, vs VS) (bool, error)

// Engine is the software that interprets the process definition and controls the execution of the workflow.
// It manages the state of the workflow, the assignment of tasks to participants, and the evaluation of conditions.
//
// Engine is also the (API) interface through which external applications interact with the workflow engine.
// It allows for starting new workflow instances, querying the state of existing instances,
// and performing other operations.
type Engine struct {
	Repository Repository

	_participantRegister participantRegister
}

type Repository interface {
	ProcessDefinitions() ProcessDefinitionRepository
	Instances() InstanceRepository
}

type participantRegister map[ParticipantID]regParticipant

func (engine *Engine) participantRegister() participantRegister {
	return zerokit.Init(&engine._participantRegister, func() participantRegister {
		return make(participantRegister)
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
	engine.participantRegister()[id] = regParticipant
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type TaskFunc[VS Vars] func(context.Context, VS) error

func (fn TaskFunc[VS]) Do(ctx context.Context, vs VS) error {
	return fn(ctx, vs)
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type UseParticipant struct {
	ID   ParticipantID
	Args []Value
	Out  []VarKey
}

func (task UseParticipant) VisitTask(visitor func(Task)) { visitor(task) }
