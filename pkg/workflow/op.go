package workflow

// Checkpoint can be used in your own workflow definition to set a checkpoint in your workflow.
// A checkpoint serves as a place of return for errors
type Checkpoint struct {
	State *State

	Definition Definition
	Input      string
}

func (Checkpoint) Error() string {
	return "workflow Checkpoint reached"
}
