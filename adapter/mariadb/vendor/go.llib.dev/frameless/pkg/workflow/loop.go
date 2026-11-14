package workflow

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/reflectkit"
)

// For is the workflow equivalent of Go's three clause for loop:
//
//	for Init; Cond; Post { Do }
//
// The variables of a For are scoped the way Go scopes them.
// The loop is a variable scope of its own, which Init declares the loop variables in,
// so they are visible to every part of the loop without leaking out of it.
// Every round is a variable scope nested in the loop's, where Cond and Post run,
// and Do, the body, is a variable scope nested in the round's,
// so what a round declares in the body is gone by Post, and by the next round.
// A variable declared outside of the loop is still visible inside,
// and an assignment to it writes through, so it keeps whatever the loop counted it to.
//
// Every round is a step of its own on the definition path, so the idempotent
// replay of a Process can tell the rounds apart: a participant in the body runs
// again on the next round rather than being skipped as already done.
// Within a round, Do is a step of its own as well,
// so the same step in Do and in Post doesn't count as done for the other.
//
// A For without a Cond is `for { ... }`: nothing in the loop clause will ever
// end it, so it runs until something inside it raises a Break.
type For struct {
	// Init [optional] runs once, before the first evaluation of Cond.
	Init Definition
	// Cond [optional] is evaluated before every round, and the loop ends when it
	// is false. A loop with no Cond only ends on a Break.
	Cond Condition
	// Post [optional] runs after Do, at the end of every round.
	Post Definition
	// Do [optional] is the loop body, executed on every round Cond allows.
	Do Definition
}

var _ Definition = For{}

func (loop For) Execute(ctx context.Context, processID ProcessID) error {
	ctx = WithVarScope(ctx, "for")

	if loop.Init != nil {
		if err := loop.Init.Execute(WithName(ctx, "init"), processID); err != nil {
			return loopDone(err)
		}
	}

	for round := 0; ; round++ {
		var ctx = WithVarScope(ctx, fmt.Sprintf("[%d]", round))

		if loop.Cond != nil {
			ok, err := loop.Cond.Evaluate(ctx, processID)
			if err != nil {
				return loopDone(err)
			}
			if !ok {
				return nil
			}
		}

		if loop.Do != nil {
			// The body is a block of its own on every round, just like in Go.
			if err := loop.Do.Execute(WithVarScope(ctx, "body"), processID); err != nil {
				return loopDone(err)
			}
		}

		if loop.Post != nil {
			if err := loop.Post.Execute(ctx, processID); err != nil {
				return loopDone(err)
			}
		}
	}
}

// loopDone tells a Break apart from a failure.
//
// A Break is not something a loop reports upwards; it is how a loop ends from
// the inside, so the loop finishes on it just as it finishes on a condition
// that turned false, or on a collection that ran out. Every part of a loop is
// treated alike here, so "a Break ends the loop" holds wherever it was raised,
// with no per-clause exception to remember.
//
// The check is a plain type assertion rather than errors.As, matching how the
// runtime dispatches its own control flow in isRuntimeSignal: control flow that
// has been wrapped into another error is no longer control flow.
func loopDone(err error) error {
	if _, ok := err.(Break); ok {
		return nil
	}
	return err
}

func (loop For) Error() string { return "workflow:for" }

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// Break ends the loop it is raised in, the way Go's break statement ends the
// for loop it is written in.
//
//	workflow.For{ // for { ... }
//		Do: workflow.Sequence{
//			workflow.Execute{Participant: "poll-job", Output: []workflow.VarName{"done"}},
//			workflow.If{
//				Cond: wftemplate.Condition(".done"),
//				Then: workflow.Break{},
//			},
//		},
//	}
//
// Break is a Definition and nothing more. It is deliberately not a
// RuntimeSignal: a signal is dynamic control flow which the runtime re-asks on
// every execution and never records, whereas breaking out of a loop is decided
// by the Process's own variables, so a replay re-derives the very same break at
// the very same round. There is nothing here that has to be asked again, and a
// Break never reaches the runtime to be asked in the first place — the loop
// around it consumes it.
//
// A Break raised outside of any loop has nothing to end, and fails the Process.
type Break struct{}

var _ Definition = Break{}

func (Break) Error() string { return "workflow:break" }

// Execute raises the Break for the enclosing loop to catch.
func (Break) Execute(context.Context, ProcessID) error { return Break{} }

//---

// ForEach is the workflow equivalent of Go's for range loop:
//
//	for K, V := range Over { Do }
//
// K and V are the loop variables. Just like in Go, every iteration has its own:
// they belong to the variable scope of the iteration, so they shadow the
// variables of the same name from outside of the loop, stay visible to the
// loops nested in the body, and an assignment to them stays within the
// iteration instead of writing through to an outer variable.
//
// They are visible to every step positioned within the iteration,
// wherever and whenever it runs, like the hook of a Defer in the body,
// as the iteration is on record (see EventForEachIteration).
type ForEach struct {
	// Over is the VarName that ForEach uses to find the iterable collection
	Over VarName
	// Do is the ForEach loop body, which is executed on each iteration, but with the currently relevant iteration scoped variable.
	// The body ends the loop before the collection runs out by raising a Break.
	Do Definition
	// K [optional] is the loop variable that holds either the index of the element in a slice, or the key of the entry in a map.
	K VarName
	// V [optional] is the loop variable that holds the currently iterated element of the collection.
	V VarName
}

var _ Definition = ForEach{}

func (foreach ForEach) Execute(ctx context.Context, processID ProcessID) error {
	vars, err := getVarsFor(ctx, processID)
	if err != nil {
		return err
	}

	collVal, ok, err := vars.Lookup(ctx, foreach.Over)
	if err != nil {
		return err
	}
	if !ok {
		return ErrFatal.F("workflow.ForEach#Execute: missing collection variable %q", foreach.Over)
	}

	var collection = reflect.ValueOf(collVal)
	collection = reflectkit.BaseValue(collection)

	switch collection.Kind() {
	case reflect.Slice:
		for i := range reflectkit.IterSlice(collection) {
			if err := foreach.iterate(ctx, processID, sliceIterationName(i)); err != nil {
				return loopDone(err)
			}
		}

	case reflect.Map:
		for key := range reflectkit.IterMap(collection) {
			if err := foreach.iterate(ctx, processID, mapIterationName(key)); err != nil {
				return loopDone(err)
			}
		}

	default:
		return nil
	}
	return nil
}

// sliceIterationName is the name of the iteration over the i-th element of a slice.
func sliceIterationName(i int) string { return fmt.Sprintf("[%d]", i) }

// mapIterationName is the name of the iteration over the entry of a map under the given key.
func mapIterationName(key reflect.Value) string { return fmt.Sprintf("%v", key.Interface()) }

// iterate runs a single iteration of the loop body.
//
// The iteration is a step of its own on the definition path, so the idempotent
// replay of a Process can tell the iterations apart and skip the ones it
// already ran. It is a block of its own too, with a variable scope of its own.
//
// The iteration's Key and Value are not recorded as variables, as they are not
// a change of the Process state. The iteration itself is recorded instead, at
// its Path (EventForEachIteration), which is a reference on where to look them
// up, and not a copy of them: every read of an iteration variable looks the
// element up in the current value of the collection. Caching the element at
// iteration-start would silently disagree with the collection once the body
// reassigns it.
//
// The iteration variables behave like the loop variables of Go's for range
// loop: each iteration has its own, bound to the iteration's variable scope.
// So they shadow the variables of the same name from outside of the loop, and
// an assignment to them stays within the iteration.
func (foreach ForEach) iterate(ctx context.Context, processID ProcessID, name string) error {
	ctx = WithVarScope(ctx, name)
	if foreach.Do == nil {
		return nil
	}
	if err := foreach.recordIteration(ctx, processID); err != nil {
		return err
	}
	return foreach.Do.Execute(ctx, processID)
}

// recordIteration records the iteration ctx is at, before its loop body runs.
//
// An iteration is identified by its Path, so a replay finds it on record, and
// doesn't record it again. An iteration without iteration variables has
// nothing to tell, so it is not recorded.
func (foreach ForEach) recordIteration(ctx context.Context, processID ProcessID) error {
	if foreach.K == "" && foreach.V == "" {
		return nil
	}
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var path = CurrentPath(ctx)
	for event, err := range repo.FindByProcessID(ctx, processID) {
		if err != nil {
			return err
		}
		if it, ok := event.(EventForEachIteration); ok && it.Path.Equal(path) {
			return nil
		}
	}
	eventID, err := MakeEventID()
	if err != nil {
		return err
	}
	var event Event = EventForEachIteration{
		EventID:   eventID,
		ProcessID: processID,
		Timestamp: timeNow(),
		Path:      path,
		Over:      foreach.Over,
		K:         foreach.K,
		V:         foreach.V,
	}
	return repo.Create(ctx, &event)
}

// EventForEachIteration records an iteration of a ForEach loop body, which binds the loop's iteration variables.
//
// It is a reference, which holds no value, only where the iteration variables are found.
// Its Path is the location of the iteration, which is the variable scope of the iteration too,
// and the name of its last Scope is the position of the element in the collection:
// "[i]" for the i-th element of a slice, and the printed key for the entry of a map.
// From every position within the iteration, K and V are looked up
// in the collection Over refers to from the location of the loop, at that position,
// so they are visible to every step positioned there, no matter when it runs.
// While the collection has no element at that position,
// or more than one key of a map prints the same, neither K nor V is visible.
//
// A variable declared in the iteration's scope, or in one nested in it, shadows the iteration variable of the same name,
// whether it was recorded before or after the iteration.
type EventForEachIteration struct {
	EventID   EventID `ext:"id"`
	ProcessID ProcessID
	Timestamp time.Time

	Path Path
	// Over is the name of the collection variable, as it is visible from the loop.
	Over VarName
	// K is the name of the variable which holds the index or the key of the element, if any.
	K VarName
	// V is the name of the variable which holds the element, if any.
	V VarName
}

var _ Event = EventForEachIteration{}

func (e EventForEachIteration) GetEventID() EventID     { return e.EventID }
func (e EventForEachIteration) GetProcessID() ProcessID { return e.ProcessID }
func (e EventForEachIteration) GetTimestamp() time.Time { return e.Timestamp }
func (e EventForEachIteration) EventType() EventType    { return "workflow:foreach:iteration" }

// binds tells whether the iteration has a variable by the given name.
func (e EventForEachIteration) binds(name VarName) bool {
	return name != "" && (name == e.K || name == e.V)
}

// names are the variable names the iteration binds.
func (e EventForEachIteration) names() []VarName {
	var names []VarName
	for _, name := range []VarName{e.K, e.V} {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// loopPath is the Path of the loop itself, which the iteration is nested in.
func (e EventForEachIteration) loopPath() Path {
	if len(e.Path) == 0 {
		return nil
	}
	return slices.Clip(e.Path[:len(e.Path)-1])
}

// locateIteration finds the element of an iteration in a collection.
//
// The name of the last Scope of the iteration's Path is the position of its
// element, the way ForEach#Execute names its iterations: "[i]" in a slice, and
// the printed key in a map. The element is not found when the collection has
// no element at that position, or when more than one key of a map prints the
// same, as those keys are the same iteration on the Path.
func locateIteration(path Path, coll any) (key, value any, ok bool) {
	if len(path) == 0 {
		return nil, nil, false
	}
	var name = path[len(path)-1].Name
	cv := reflectkit.BaseValue(reflect.ValueOf(coll))
	switch cv.Kind() {
	case reflect.Slice:
		i, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "["), "]"))
		if err != nil || i < 0 || cv.Len() <= i || sliceIterationName(i) != name {
			return nil, nil, false
		}
		return i, cv.Index(i).Interface(), true
	case reflect.Map:
		for k, v := range reflectkit.IterMap(cv) {
			if mapIterationName(k) != name {
				continue
			}
			if ok {
				return nil, nil, false
			}
			key, value, ok = k.Interface(), v.Interface(), true
		}
		return key, value, ok
	default:
		return nil, nil, false
	}
}

func (foreach ForEach) Error() string { return "workflow:foreach" }
