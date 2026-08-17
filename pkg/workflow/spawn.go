package workflow

import (
	"context"
	"fmt"
	"time"

	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/testcase/clock"
)

// Spawn request Runtime to launch a sub-workflow.
// Upon returning a Spawn Definition from a participant, it is guaranteed that
// upon persisting the results of the participant call with the Spawn Definition,
// the participant call remains idempotent, and spawn will be eventually executed by the runtime.
type Spawn struct {
	// Name is an user given, human friendly Name
	// which can be used later for parent-child coordination.
	Name SpawnName
	// Definition is the Process Definition it meant to serve
	Definition Definition
	// Vars forwards selected variables from the parent process to the
	// spawned child. It is a VarMapping ([FROM] parent [TO] child), so for
	// each {parentKey: childKey} entry the current value of parentKey on
	// the parent process is copied into childKey on the child process.
	//
	// Parent variables that are not yet set are skipped silently — pair the
	// Spawn with workflow.SetVar in the parent definition to initialise the
	// forwarded value before the spawn point.
	//
	// The transferred values must be encodable by the Runtime#Codec.
	Vars VarMapping
}

func (spawn Spawn) Validate(ctx context.Context) error {
	if len(spawn.Name) == 0 {
		return fmt.Errorf("missing %T#Name", spawn)
	}
	if spawn.Definition == nil {
		return fmt.Errorf("missing %T#Definition", spawn)
	}
	if v, ok := spawn.Definition.(validate.Validatable); ok {
		if err := v.Validate(spawn.withName(ctx)); err != nil {
			return err
		}
	}
	return nil
}

func (spawn Spawn) withName(ctx context.Context) context.Context {
	return WithName(WithName(ctx, "spawn"), string(spawn.Name))
}

// SpawnName is a human friendly user given identifier name for referencing an eventually spawned sub process.
// If not zero, then it must be uniquely named between the spawned sub processes of a given process.
//
// Its main purpose is to enable parent-child life-cycle coordination.
type SpawnName string

var _ Definition = Spawn{}

func (spawn Spawn) Error() string { return "workflow::signal::spawn" }

// Execute persists a Spawn request and arranges for the sub-workflow to run.
//
// The persistence phase (SpawnEvent + child's UseDefinitionEvent + initial
// variables) is wrapped in a single transaction so the parent and child events
// become visible atomically. The sub-process is only enqueued for execution
// AFTER that transaction commits — otherwise a worker that picked up the
// schedule entry would observe an empty child history, treat the child as a
// never-bound process, and immediately record an EventCompleted, silently
// dropping the spawn.
func (spawn Spawn) Execute(ctx context.Context, processID ProcessID) (rErr error) {

	if err := spawn.Validate(ctx); err != nil {
		return err
	}
	ctx = spawn.withName(ctx)
	rt, ok := RuntimeFromContext(ctx)
	if !ok {
		return ErrNoContextRuntime
	}
	if err := rt.Validate(ctx); err != nil {
		return err
	}
	spawnEvent, err := spawn.ensureEvents(ctx, rt, processID)
	if err != nil {
		return err
	}
	return spawn.scheduleChild(ctx, rt, spawnEvent)
}

func (spawn Spawn) ensureEvents(ctx context.Context, rt Runtime, parentID ProcessID) (_ EventSpawn, rErr error) {
	ctx, err := rt.Events.BeginTx(ctx)
	if err != nil {
		return EventSpawn{}, err
	}
	defer comproto.FinishOnePhaseCommit(&err, rt.Events, ctx)
	event, err := spawn.ensureSpawnEvent(ctx, rt, parentID)
	if err != nil {
		return EventSpawn{}, err
	}
	// since we operate in a single transaction
	// we don't need to use the child process lock
	// because child entries either exists,
	// or we about to commit them.
	for _, err := range rt.Events.FindByProcessID(ctx, event.ChildID) {
		if err != nil {
			return EventSpawn{}, err
		}
		return event, nil // already scheduled
	}
	if err := rt.Bind(ctx, event.ChildID, spawn.Definition); err != nil {
		return EventSpawn{}, err
	}
	if err := spawn.forwardVars(ctx, parentID, event.ChildID); err != nil {
		return EventSpawn{}, err
	}
	return event, nil
}

// forwardVars copies the parent variables listed in spawn.Vars into the
// spawned child under their mapped child keys. spawn.Vars is a VarMapping
// ([FROM] parent [TO] child), so for each {parentKey: childKey} entry the
// current value of parentKey on the parent process is read and stored under
// childKey on the child process.
//
// Parent variables that are not set are skipped silently rather than raising
// an error: the caller can compose a missing parent value by chaining a
// workflow.SetVar before the Spawn, and a half-populated child is preferable
// to failing the spawn outright.
func (spawn Spawn) forwardVars(ctx context.Context, parentID, childID ProcessID) error {
	if len(spawn.Vars) == 0 {
		return nil
	}
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var (
		parentVars = Vars{ProcessID: parentID, EventsRepository: repo}
		childVars  = Vars{ProcessID: childID, EventsRepository: repo}
	)
	for parentKey, childKey := range spawn.Vars {
		value, ok, err := parentVars.Lookup(ctx, parentKey)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := childVars.Set(ctx, childKey, value); err != nil {
			return err
		}
	}
	return nil
}

func (spawn Spawn) ensureSpawnEvent(ctx context.Context, rt Runtime, parentID ProcessID) (EventSpawn, error) {
	for event, err := range rt.Events.FindByProcessID(ctx, parentID) {
		if err != nil {
			return EventSpawn{}, err
		}
		if spawnEvent, ok := event.(EventSpawn); ok {
			if spawn.Name == spawnEvent.Name {
				return spawnEvent, nil
			}
		}
	}
	eventID, err := MakeEventID() // maybe not the right place to issue the event ID
	if err != nil {
		return EventSpawn{}, err
	}
	childID, err := MakeProcessID()
	if err != nil {
		return EventSpawn{}, err
	}
	var e = EventSpawn{
		EventID:   eventID,
		ProcessID: parentID,
		ChildID:   childID,
		Name:      spawn.Name,
		Timestamp: clock.Now(),
	}
	var event Event = e
	return e, rt.Events.Create(ctx, &event)
}

func (spawn Spawn) scheduleChild(ctx context.Context, rt Runtime, event EventSpawn) (rErr error) {
	_, acquired, unlock, err := rt.tryLock(ctx, event.ChildID)
	if err != nil {
		return err
	}
	if !acquired {
		return nil // someone is already working on this, we are good, it is being executed
	}
	if err := unlock(); err != nil {
		return err
	}
	var complete = Complete{ProcessID: event.ChildID}
	if isCompleted, err := complete.IsCompleted(ctx, rt.Events); err != nil {
		return err
	} else if isCompleted {
		return nil
	}
	return rt.Schedule(ctx, event.ChildID)
}

type EventSpawn struct {
	EventID EventID `ext:"id"`
	// ProcessID is the parent ProcessID
	ProcessID ProcessID
	// ChildID id the ProcessID of the child
	ChildID ProcessID
	// Name is the human friendly spawn identifier.
	Name SpawnName
	// Timestamp is when the spawn request was registered
	Timestamp time.Time
}

var _ Event = EventSpawn{}

const typeSpawnEvent = "workflow::event::spawn"

func (e EventSpawn) EventType() EventType    { return typeSpawnEvent }
func (e EventSpawn) GetEventID() EventID     { return e.EventID }
func (e EventSpawn) GetProcessID() ProcessID { return e.ProcessID }
func (e EventSpawn) GetTimestamp() time.Time { return e.Timestamp }

// ---

type Join struct {
	// SpawnName [optional]
	SpawnName SpawnName
	// Collect [optional] will collect the results from the child process variables
	// Mapping happens from child VariableKey -> parent VariableKey (From -> To)
	//
	// - If it is nil, it means collect all
	// - If it is empty, it means dispose result from the child workflow
	// - If it contains mappings, then it will collect values and assign them in the parent
	Collect VarMapping
}

var _ Definition = Join{}

func (d Join) Error() string { return "workflow::join" }

func (d Join) Execute(ctx context.Context, processID ProcessID) error {
	ctx = WithName(ctx, "join")
	if err := d.Validate(ctx); err != nil {
		return err
	}
	if len(d.SpawnName) == 0 {
		return d.joinAll(ctx, processID) // join
	}
	return d.joinChild(ctx, processID) // join .SpawnName
}

func (d Join) joinChild(ctx context.Context, parentID ProcessID) error {
	if len(d.SpawnName) == 0 {
		return fmt.Errorf("missing %T#SpawnName", d)
	}

	ctx = WithName(ctx, string(d.SpawnName))
	ctx = logging.ContextWith(ctx, logging.Field("name", d.SpawnName))

	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}

	var currentPath = CurrentPath(ctx)

	var (
		child EventSpawn
		found bool
		done  bool
	)
scan:
	for event, err := range repo.FindByProcessID(ctx, parentID) {
		if err != nil {
			return err
		}
		switch event := event.(type) {
		case EventSpawn:
			if event.Name == d.SpawnName {
				child = event
				found = true
			}
		case EventJoin:
			if !event.Path.Equal(currentPath) {
				done = true
				break scan
			}
		}
	}
	if done {
		logger.Debug(ctx, "join by spawn name is completed")
		return nil
	}
	if !found {
		return fmt.Errorf("missing spawn child for the given Join.SpawnName %q in process %s", d.SpawnName, parentID.String())
	}

	done, err = Complete{ProcessID: child.ChildID}.IsCompleted(ctx, repo)
	if err != nil {
		return err
	}
	if !done {
		logger.Debug(ctx, "the workflow has been paused because the child task is not yet complete")
		return Suspend{}
	}

	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventJoin{
		EventID:   eventID,
		ProcessID: parentID,
		Children:  []ProcessID{child.ChildID},
		Timestamp: clock.Now(),
		Path:      currentPath,
	}
	return repo.Create(ctx, &event)
}

func (d Join) joinAll(ctx context.Context, parentID ProcessID) (rerr error) {
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var currentPath = CurrentPath(ctx) // "join" path
	var (
		children []ProcessID
		done     bool
	)
scan:
	for event, err := range repo.FindByProcessID(ctx, parentID) {
		if err != nil {
			return err
		}
		switch event := event.(type) {
		case EventSpawn:
			children = append(children, event.ChildID)
		case EventJoin:
			if !event.Path.Equal(currentPath) {
				done = true
				break scan
			}
		}
	}
	for _, child := range children {
		done, err = Complete{ProcessID: child}.IsCompleted(ctx, repo)
		if err != nil {
			return err
		}
		if !done {
			logger.Debug(ctx, "the workflow has been paused because the children processes are not completed")
			return Suspend{}
		}
	}

	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventJoin{
		EventID:   eventID,
		ProcessID: parentID,
		Children:  children,
		Timestamp: clock.Now(),
		Path:      currentPath,
	}
	return repo.Create(ctx, &event)
}

var _ validate.Validatable = Join{}

func (d Join) Validate(ctx context.Context) error {
	if 0 < len(d.Collect) && len(d.SpawnName) == 0 {
		return fmt.Errorf("workflow.Join#Collect only works if the SpawnName is provided")
	}
	return nil
}

type EventJoin struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Children []ProcessID
	Path     Path
}

var _ Event = EventSpawn{}

const typeJoinEvent = "workflow::event::join"

func (e EventJoin) EventType() EventType    { return typeJoinEvent }
func (e EventJoin) GetEventID() EventID     { return e.EventID }
func (e EventJoin) GetProcessID() ProcessID { return e.ProcessID }
func (e EventJoin) GetTimestamp() time.Time { return e.Timestamp }
