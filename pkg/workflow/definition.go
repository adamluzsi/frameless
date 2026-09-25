package workflow

import (
	"context"
	"fmt"
	"strconv"
	"time"
)

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Sequence executes its child definitions in order.
type Sequence []Definition

var _ Definition = (*Sequence)(nil)

func (Sequence) Error() string { return "workflow::sequence" }

func (seq Sequence) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "sequence")
	for i, participant := range seq {
		// Each iteration derives its own context from the
		// sequence-scoped ctx so iterations don't leak path
		// segments into one another.
		if err := participant.Execute(WithName(ctx, fmt.Sprintf("[%d]", i)), pid); err != nil {
			return err
		}
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// If executes Then when Cond is true, Else otherwise.
//
// Keeping the same branch on replay is the responsibility of the Condition.
// Execute (with ConditionID) and wftemplate.Condition record their answer at the `if/<ID>` path,
// so a replay takes the same branch even if the variables they read have changed since.
// A custom Condition can do the same by answering through Execute#EvaluateWith;
// otherwise it is asked again on every replay.
type If struct {
	Cond Condition
	Then Definition
	Else Definition
}

var _ Definition = (*If)(nil)

func (If) Error() string { return "workflow::if" }

func (d If) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "if")
	if d.Cond == nil {
		return ErrFatal.F("missing %s condition", d.Error())
	}
	var ok, err = d.Cond.Evaluate(ctx, pid)
	if err != nil {
		return err
	}
	if ok {
		if d.Then != nil {
			return d.Then.Execute(WithName(ctx, "then"), pid)
		}
	} else {
		if d.Else != nil {
			return d.Else.Execute(WithName(ctx, "else"), pid)
		}
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Sleep pauses execution until its While condition is false or its Until
// condition is true. It signals suspension by returning Suspend.
// A Sleep with neither condition never wakes up.
//
// Every attempt asks the condition anew, under the second the attempt happens in.
// A condition that records its answers, like Execute or wftemplate.Condition,
// records them per attempt second, so an answer that kept the Sleep asleep
// is not replayed to the attempts of a later second.
// Attempts within the same second share those answers, and under a frozen clock the Sleep doesn't wake up.
//
// The wake-up is recorded as an EventSleepCompleted at the position of the Sleep.
// A replay passes a Sleep that already woke up without asking its condition again,
// even if the answer of its condition has changed since.
type Sleep struct {
	While Condition
	Until Condition
}

var _ Definition = Sleep{}

func (d Sleep) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "sleep")
	var (
		cond   Condition
		wakeUp bool // the answer of cond that lets the Sleep wake up
	)
	switch {
	case d.While != nil:
		cond, wakeUp = d.While, false
	case d.Until != nil:
		cond, wakeUp = d.Until, true
	default:
		return Suspend{}
	}
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	path := CurrentPath(ctx)
	for event, err := range repo.FindByProcessID(ctx, pid) {
		if err != nil {
			return err
		}
		if completed, ok := event.(EventSleepCompleted); ok && completed.Path.Equal(path) {
			return nil
		}
	}
	// A suspension commits what the attempt recorded,
	// so an attempt at a fixed path would replay the answer that kept the Sleep asleep for ever.
	attempt := WithName(ctx, strconv.FormatInt(timeNow().Unix(), 10))
	answer, err := cond.Evaluate(attempt, pid)
	if err != nil {
		return err
	}
	if answer != wakeUp {
		return Suspend{}
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventSleepCompleted{
		EventID:   eventID,
		ProcessID: pid,
		Timestamp: timeNow(),
		Path:      path,
	}
	return repo.Create(ctx, &event)
}

func (d Sleep) Error() string { return "workflow::sleep" }

// EventSleepCompleted records that the Sleep at Path woke up.
// A Sleep with a recorded completion is passed on replay, without asking its condition again.
type EventSleepCompleted struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Path Path
}

var _ Event = EventSleepCompleted{}

func (e EventSleepCompleted) EventType() EventType    { return "workflow::sleep::completed" }
func (e EventSleepCompleted) GetEventID() EventID     { return e.EventID }
func (e EventSleepCompleted) GetProcessID() ProcessID { return e.ProcessID }
func (e EventSleepCompleted) GetTimestamp() time.Time { return e.Timestamp }

type Suspend struct{}

var _ RuntimeSignal = Suspend{}

func (sig Suspend) Error() string { return "workflow::suspend" }

func (sig Suspend) RuntimeSignalExecute(ctx context.Context, rt Runtime, id ProcessID) error {
	return sig
}
