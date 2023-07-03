package workflow

import "context"

// Participant is the entity that perform tasks in the workflow.
// Participants can be human users, groups of users, or even automated systems.
// The workflow engine needs to manage the assignment of tasks to participants and track their progress.
type Participant[VS Variables] interface {
	Do(context.Context, VS) error
}

type ParticipantID string
