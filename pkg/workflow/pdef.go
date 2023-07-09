package workflow

import "github.com/adamluzsi/frameless/ports/crud"

// ProcessDefinition is the blueprint of your workflow.
// It defines the sequence of tasks that need to be executed,
// the conditions under which they should be executed, and the order in which they should occur.
// This is typically represented in a graphical form using a flowchart or a similar diagram.
type ProcessDefinition struct {
	ID         ProcessDefinitionID `ext:"id"`
	Task       Task
	EntryPoint EntryPoint
}

type ProcessDefinitionID string

type EntryPoint interface{}

type ProcessDefinitionRepository interface {
	crud.Creator[ProcessDefinition]
	crud.Finder[ProcessDefinition, ProcessDefinitionID]
	crud.Deleter[ProcessDefinitionID]
}

// Instance is each run of a workflow according to a particular process definition.
// The workflow engine needs to manage multiple instances, each with its own state and variables.
type Instance struct {
	ID                  InstanceID `ext:"id"`
	ProcessDefinitionID ProcessDefinitionID
	Variables           Vars
}

type InstanceID string

type InstanceRepository interface {
	crud.Creator[Instance]
	crud.Finder[Instance, InstanceID]
	crud.Updater[Instance]
	crud.Deleter[InstanceID]
}
