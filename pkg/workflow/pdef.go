package workflow

// ProcessDefinition is the blueprint of your workflow.
// It defines the sequence of tasks that need to be executed,
// the conditions under which they should be executed, and the order in which they should occur.
// This is typically represented in a graphical form using a flowchart or a similar diagram.
type ProcessDefinition struct {
	ID   ProcessDefinitionID `ext:"id"`
	Task Task
}

type ProcessDefinitionID string

