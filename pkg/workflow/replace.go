package workflow

import (
	"context"
	"fmt"
	"time"
)

// Replace is a RuntimeSignal, which instructs the runtime
// to replace the current workflow's Definition with the newly provided one.
type Replace struct {
	Definition Definition
}

var _ RuntimeSignal = Replace{}

func (sig Replace) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error {
	var eventID, err = MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventUseDefinition{
		EventID:    eventID,
		ProcessID:  id,
		Timestamp:  timeNow(),
		Definition: sig.Definition,
	}
	return rt.Events.Create(ctx, &event)
}

func (sig Replace) Error() string {
	return fmt.Sprintf("%T", sig)
}

type EventUseDefinition struct {
	EventID    EventID `ext:"id"`
	ProcessID  ProcessID
	Timestamp  time.Time
	Definition Definition
}

// VarMapping maps the variables of one process to the variables of another, as {fromName: toName} entries.
//
// Depending on the context of its usage, the direction of data flow is [FROM] -> [TO]:
//   - In case of Spawn#Vars, [FROM] parent -> [TO] child, so the entries are {parentKey: childKey}.
//   - In case of Join#Collect, [FROM] child -> [TO] parent, so the entries are {childKey: parentKey}.
type VarMapping map[VarName]VarName

var _ Event = EventUseDefinition{}

func (s EventUseDefinition) GetEventID() EventID     { return s.EventID }
func (s EventUseDefinition) EventType() EventType    { return "workflow:definition" }
func (s EventUseDefinition) GetProcessID() ProcessID { return s.ProcessID }
func (s EventUseDefinition) GetTimestamp() time.Time { return s.Timestamp }
