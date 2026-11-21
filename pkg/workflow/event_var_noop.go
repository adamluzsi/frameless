package workflow

import "time"

// EventVarNoop records that a variable step succeeded without changing a binding.
// Path is the step's execution path, not the scope of a target binding (there may be none).
// Mutation is the type of event the step would have written: a set or a delete.
// The variable fold ignores this event; only the step's replay guard uses it.
type EventVarNoop struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Path     Path
	Name     VarName
	Mutation EventType
}

var _ Event = EventVarNoop{}

func (e EventVarNoop) GetEventID() EventID     { return e.EventID }
func (e EventVarNoop) GetProcessID() ProcessID { return e.ProcessID }
func (e EventVarNoop) GetTimestamp() time.Time { return e.Timestamp }
func (e EventVarNoop) EventType() EventType    { return "workflow:var:noop" }
