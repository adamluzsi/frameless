package workflow

import (
	"time"
)

type EventParticipant struct {
	EventID       EventID `ext:"id"`
	ProcessID     ProcessID
	Timestamp     time.Time
	ParticipantID ParticipantID
	Path          Path
	Input         []any
	Output        []any
	Definition    Definition
}

var _ Event = (*EventParticipant)(nil)

func (EventParticipant) EventType() EventType      { return "workflow::participant" }
func (e EventParticipant) GetEventID() EventID     { return e.EventID }
func (e EventParticipant) GetProcessID() ProcessID { return e.ProcessID }
func (e EventParticipant) GetTimestamp() time.Time { return e.Timestamp }
