package workflow

import "fmt"

type ErrParticipantNotFound struct{ PID ParticipantID }

func (e ErrParticipantNotFound) Error() string {
	return fmt.Sprintf("[%T] %s", e, e.PID)
}
