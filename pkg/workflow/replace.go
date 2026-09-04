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

// VarMapping is a mapping between a parent and child process variables.
//
// It goes [FROM] parent [TO] child.
type VarMapping map[parentVarKey]childVarKey

type parentVarKey = VarName
type childVarKey = VarName

var _ Event = EventUseDefinition{}

const typeSetDefinitionEvent EventType = "workflow::set-definition-event"

func (s EventUseDefinition) GetEventID() EventID     { return s.EventID }
func (s EventUseDefinition) EventType() EventType    { return typeSetDefinitionEvent }
func (s EventUseDefinition) GetProcessID() ProcessID { return s.ProcessID }
func (s EventUseDefinition) GetTimestamp() time.Time { return s.Timestamp }
