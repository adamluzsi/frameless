package workflow

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"go.llib.dev/frameless/pkg/iterkit"
	"go.llib.dev/frameless/pkg/logger"
	"go.llib.dev/frameless/pkg/logging"
	"go.llib.dev/frameless/pkg/validate"
	"go.llib.dev/frameless/port/comproto"
)

// Spawn request Runtime to launch a sub-workflow.
// Upon returning a Spawn Definition from a participant, it is guaranteed that
// upon persisting the results of the participant call with the Spawn Definition,
// the participant call remains idempotent, and spawn will be eventually executed by the runtime.
type Spawn struct {
	// Name is an user given, human friendly Name
	// which can be used later for parent-child coordination, see Join#Spawn.
	//
	// A Spawn is identified by where it is in the definition, and not by its Name alone,
	// so two Spawns of the same Name spawn a child each,
	// and a Spawn in the body of a loop spawns a child in every round.
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

func iterChildren(ctx context.Context, er EventRepository, parentID ProcessID) iterkit.SeqE[ProcessID] {
	return func(yield func(ProcessID, error) bool) {
		for event, err := range er.FindByProcessID(ctx, parentID) {
			if err != nil {
				if !yield(ProcessID{}, err) {
					return
				}
				continue
			}
			var spawn, ok = event.(EventSpawn)
			if !ok {
				continue
			}
			if !yield(spawn.ChildID, nil) {
				return
			}
		}
	}
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
	ctx = WithName(ctx, "spawn")
	ctx = WithName(ctx, string(spawn.Name))
	return ctx
}

// SpawnName is a human friendly user given identifier name for referencing an eventually spawned sub process.
// It doesn't have to be unique among the children of a process,
// a Join by name joins the child of that name spawned the closest to it, see Join#Spawn.
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
	defer comproto.FinishOnePhaseCommit(&rErr, rt.Events, ctx)
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
// The parent variables are looked up from where the Spawn is,
// while the child variables are declared at the root of the child,
// as the child doesn't run in any of its parent's variable scopes.
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
	return transferVars(spawn.Vars,
		ctx, Vars{ProcessID: parentID, EventsRepository: repo},
		atProcessRoot(ctx), Vars{ProcessID: childID, EventsRepository: repo})
}

// transferVars copies variables between a parent and a child Process,
// for each {fromName: toName} entry of the mapping,
// from what fromName refers to at fromCtx in the from Process,
// to toName at toCtx in the to Process.
//
// It is the common ground of Spawn#Vars and Join#Collect,
// so passing variables to a child and collecting them from it behave alike:
//   - a variable that is not set in the from Process is skipped silently;
//   - toName is assigned like any other variable,
//     which writes through to a toName visible from toCtx,
//     or declares it in the variable scope of toCtx otherwise;
//   - the entries are transferred in the order of their fromName,
//     so even a mapping with two entries of the same toName is deterministic.
func transferVars(mapping VarMapping, fromCtx context.Context, from Vars, toCtx context.Context, to Vars) error {
	for _, fromName := range slices.Sorted(maps.Keys(mapping)) {
		value, ok, err := from.Lookup(fromCtx, fromName)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := to.Set(toCtx, mapping[fromName], value); err != nil {
			return err
		}
	}
	return nil
}

// ensureSpawnEvent finds the EventSpawn the Spawn recorded at the current Path,
// or records one with a new child otherwise.
//
// A Spawn is identified by where it is in the definition, and not by its Name alone,
// so every round of a loop spawns a child of its own,
// while a replayed round finds the child it spawned before.
func (spawn Spawn) ensureSpawnEvent(ctx context.Context, rt Runtime, parentID ProcessID) (EventSpawn, error) {
	var path = CurrentPath(ctx)
	for event, err := range rt.Events.FindByProcessID(ctx, parentID) {
		if err != nil {
			return EventSpawn{}, err
		}
		if spawnEvent, ok := event.(EventSpawn); ok && spawnEvent.recordedBy(spawn.Name, path) {
			return spawnEvent, nil
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
		Timestamp: timeNow(),
		Path:      path,
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
	if isCompleted, err := IsCompleted(ctx, rt.Events, event.ChildID); err != nil {
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
	// Path is where the Spawn is in the definition of the parent,
	// which identifies the Spawn that recorded the event.
	Path Path
}

var _ Event = EventSpawn{}

// recordedBy tells whether the event was recorded by the Spawn of the given name at the given Path.
//
// An EventSpawn recorded before Spawns were identified by their Path has none,
// so it is matched by its Name, as it was when it got recorded.
func (e EventSpawn) recordedBy(name SpawnName, path Path) bool {
	if len(e.Path) == 0 {
		return e.Name == name
	}
	return e.Path.Equal(path)
}

func (e EventSpawn) EventType() EventType    { return "workflow::spawn" }
func (e EventSpawn) GetEventID() EventID     { return e.EventID }
func (e EventSpawn) GetProcessID() ProcessID { return e.ProcessID }
func (e EventSpawn) GetTimestamp() time.Time { return e.Timestamp }

// spawnRank is how close a Spawn is to a Join of its name, see Join#Spawn.
// The Join joins the child of the closest Spawn.
type spawnRank struct {
	// visible tells that the Spawn is in the variable scope of the Join, or in one enclosing it.
	visible bool
	// depth is how close the Spawn is among the ones equally visible:
	// the depth of its variable scope for a visible Spawn, as the innermost one is the closest,
	// and the length of the Path it shares with the Join otherwise.
	depth int
}

// rankSpawn ranks the Spawn which recorded the event, for the Join at the given Path.
//
// A Spawn declares its name in its variable scope, the way a Go variable is declared in its block,
// so it is visible to the Joins in that variable scope, and in the ones nested in it.
func rankSpawn(e EventSpawn, join Path) spawnRank {
	if len(e.Path) == 0 {
		// The event was recorded before Spawns had a Path,
		// so the Spawn is only known by its name, which is the least close.
		return spawnRank{}
	}
	if scope := e.Path.VarScope(); join.MatchPrefix(scope) {
		return spawnRank{visible: true, depth: len(scope)}
	}
	return spawnRank{depth: commonPrefixLen(e.Path, join)}
}

func (r spawnRank) compare(oth spawnRank) int {
	if r.visible != oth.visible {
		if r.visible {
			return 1
		}
		return -1
	}
	return cmp.Compare(r.depth, oth.depth)
}

// ---

type Join struct {
	// Spawn [optional] will configure Join to Join a specific Spawn back by its name.
	//
	// When more children were spawned by that name, it joins the one closest to the Join,
	// the way a Go variable name refers to the declaration closest to where it is used:
	//   - A Spawn declares its name in the variable scope it is in,
	//     so it is visible to the Joins of that variable scope, and of the ones nested in it.
	//     Of the visible Spawns, the one of the innermost variable scope is joined,
	//     so a Join in the body of a loop joins the child of its own round,
	//     rather than one spawned before the loop.
	//   - When none is visible, like from after the loop or the If that holds the Spawn,
	//     the Spawn which shares the longest Path with the Join is joined.
	//   - Of the Spawns equally close, the one spawned the latest is joined,
	//     so a Join after a loop joins the child of the last round.
	Spawn SpawnName
	// Collect [optional] collects the results of the joined child into the parent's variables.
	// It is a VarMapping ([FROM] child [TO] parent), so for each {childKey: parentKey} entry,
	// the value of childKey in the root variable scope of the completed child is assigned to parentKey.
	//
	// The parentKey is assigned like the Output of an Execute at the place of the Join:
	// it writes through to a parentKey variable visible from there,
	// or declares parentKey in the variable scope of the Join otherwise.
	//
	// Only the listed variables are collected, a nil or empty Collect collects nothing.
	// Child variables that are not set are skipped silently,
	// so a value the parent assigned to parentKey before the Join remains as a default.
	//
	// The collection is part of the Join itself, so it happens once:
	// a resumed Process replays the Join, and doesn't collect the values again.
	//
	// Collect requires the Spawn to be named, as it collects from that single child.
	// The transferred values must be encodable by the Runtime#Codec.
	Collect VarMapping
}

var _ Definition = Join{}

func (d Join) Error() string { return "workflow::join" }

func (d Join) Execute(ctx context.Context, processID ProcessID) error {
	ctx = WithName(ctx, "join")
	if err := d.Validate(ctx); err != nil {
		return err
	}
	if len(d.Spawn) == 0 {
		return d.joinAll(ctx, processID) // join
	}
	return d.joinChild(ctx, processID) // join .SpawnName
}

func (d Join) joinChild(ctx context.Context, parentID ProcessID) (rErr error) {
	if len(d.Spawn) == 0 {
		return fmt.Errorf("missing %T#Spawn", d)
	}

	ctx = WithName(ctx, string(d.Spawn))
	ctx = logging.ContextWith(ctx, logging.Field("name", d.Spawn))

	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}

	var currentPath = CurrentPath(ctx)

	var (
		child   EventSpawn
		closest spawnRank
		found   bool
		done    bool
	)
scan:
	for event, err := range repo.FindByProcessID(ctx, parentID) {
		if err != nil {
			return err
		}
		switch event := event.(type) {
		case EventSpawn:
			if event.Name != d.Spawn {
				continue
			}
			// The event log is in the order of the Spawns,
			// so of the Spawns equally close, the latest is kept.
			if rank := rankSpawn(event, currentPath); !found || 0 <= rank.compare(closest) {
				child, closest, found = event, rank, true
			}
		case EventJoin:
			if event.Path.Equal(currentPath) {
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
		return fmt.Errorf("missing spawn child for the given Join.SpawnName %q in process %s", d.Spawn, parentID.String())
	}

	done, err = IsCompleted(ctx, repo, child.ChildID)
	if err != nil {
		return err
	}
	if !done {
		logger.Debug(ctx, "the workflow has been paused because the child task is not yet complete")
		return Suspend{}
	}

	// The collected variables and the EventJoin are recorded together,
	// as the EventJoin is what keeps a replay from collecting the values again.
	ctx, err = repo.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, repo, ctx)

	if err := d.collect(ctx, repo, parentID, child.ChildID); err != nil {
		return err
	}

	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventJoin{
		EventID:   eventID,
		ProcessID: parentID,
		Children:  []ProcessID{child.ChildID},
		Timestamp: timeNow(),
		Path:      currentPath,
	}
	return repo.Create(ctx, &event)
}

// collect assigns the child variables listed in Join#Collect to the parent variables they are mapped to.
//
// The child variables are looked up from the root of the child,
// as only its root variable scope outlives the child's execution,
// while the parent variables are assigned where the Join is.
func (d Join) collect(ctx context.Context, repo EventRepository, parentID, childID ProcessID) error {
	if len(d.Collect) == 0 {
		return nil
	}
	return transferVars(d.Collect,
		atProcessRoot(ctx), Vars{ProcessID: childID, EventsRepository: repo},
		ctx, Vars{ProcessID: parentID, EventsRepository: repo})
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
			if event.Path.Equal(currentPath) {
				done = true
				break scan
			}
		}
	}
	if done {
		logger.Debug(ctx, "join of every child is completed")
		return nil
	}
	for _, child := range children {
		done, err = IsCompleted(ctx, repo, child)
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
		Timestamp: timeNow(),
		Path:      currentPath,
	}
	return repo.Create(ctx, &event)
}

var _ validate.Validatable = Join{}

func (d Join) Validate(ctx context.Context) error {
	if 0 < len(d.Collect) && len(d.Spawn) == 0 {
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

func (e EventJoin) EventType() EventType    { return "workflow::join" }
func (e EventJoin) GetEventID() EventID     { return e.EventID }
func (e EventJoin) GetProcessID() ProcessID { return e.ProcessID }
func (e EventJoin) GetTimestamp() time.Time { return e.Timestamp }
