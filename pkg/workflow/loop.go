package workflow

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"go.llib.dev/frameless/pkg/contextkit"
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

func (loop For) Error() string { return "workflow::for" }

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

func (Break) Error() string { return "workflow::break" }

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
			var name = fmt.Sprintf("[%d]", i)
			i := i
			if err := foreach.iterate(ctx, processID, name, &i, reflect.Value{}); err != nil {
				return loopDone(err)
			}
		}

	case reflect.Map:
		for key := range reflectkit.IterMap(collection) {
			var name = fmt.Sprintf("%v", key.Interface())
			if err := foreach.iterate(ctx, processID, name, nil, key); err != nil {
				return loopDone(err)
			}
		}

	default:
		return nil
	}
	return nil
}

// iterate runs a single iteration of the loop body.
//
// The iteration is a step of its own on the definition path, so the idempotent
// replay of a Process can tell the iterations apart and skip the ones it
// already ran. It is a block of its own too, with a variable scope of its own.
//
// The iteration's Key and Value are not stored as state on the iteration: the
// context only carries a reference — the name of the collection variable and
// the position within it — and every read of the iteration variable re-evaluates
// the position against the current value of the collection in the event log.
// A Process is a record of state changes, and a ForEach iteration is a
// reference on what to look up from that record, not a change of its own.
// Caching the value at iteration-start would silently disagree with the
// collection once the body reassigns it.
//
// The iteration variables behave like the loop variables of Go's for range
// loop: each iteration has its own, declared in the iteration's variable
// scope. So they shadow the variables of the same name from outside of the
// loop, and an assignment to them stays within the iteration.
func (foreach ForEach) iterate(ctx context.Context, processID ProcessID, name string, index *int, key reflect.Value) error {
	ctx = WithVarScope(ctx, name)
	ctx = withIterationVars(ctx, iterationVars{
		KeyName:   foreach.K,
		ValueName: foreach.V,
		Scope:     CurrentPath(ctx),

		Source: foreach.Over,
		Index:  index,
		Key:    key,
	})
	if foreach.Do != nil {
		return foreach.Do.Execute(ctx, processID)
	}
	return nil
}

// iterationVars is the in-memory reference an iteration has on the Key/Value
// it should expose to the body. It stores the names under which the body
// expects to find the index/key and the element, the name of the source
// collection variable, and the position within that collection.
//
// ForEach#Execute sets Index for slice iterations and Key for map iterations
// (exactly one of them), and visibleVars#iterationValue uses this instruction
// to re-resolve the value against the current state of the collection on every
// read, so a body that reassigns the collection variable sees the new value
// without the proxy holding a stale snapshot.
type iterationVars struct {
	KeyName   VarName
	ValueName VarName
	// Scope is the variable scope of the loop body iteration, which the
	// iteration variables are bound to. As it is the Path of the iteration,
	// no other execution of a loop body shares it, not even the same iteration
	// on another round of an enclosing For.
	Scope Path

	Source VarName
	Index  *int
	Key    reflect.Value

	// parent is the iteration of the enclosing loop, when the loop is nested
	// in the body of another one, whose iteration variables stay visible.
	parent *iterationVars
}

var ctxIterationVars = contextkit.ValueHandler[ctxKeyIterationVars, iterationVars]{}

type ctxKeyIterationVars struct{}

func withIterationVars(ctx context.Context, iv iterationVars) context.Context {
	if parent, ok := ctxIterationVars.Lookup(ctx); ok {
		iv.parent = &parent
	}
	return ctxIterationVars.ContextWith(ctx, iv)
}

// names are the variable names the iteration exposes to the loop body.
func (iv iterationVars) names() []VarName {
	var names []VarName
	for _, name := range []VarName{iv.KeyName, iv.ValueName} {
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// exposes tells whether the iteration has a variable by the given name.
func (iv iterationVars) exposes(name VarName) bool {
	return name != "" && (name == iv.KeyName || name == iv.ValueName)
}

// key is the value of the Key iteration variable: the index of the element in
// a slice, or the key of the entry in a map.
func (iv iterationVars) key() (any, bool, error) {
	switch {
	case iv.Index != nil:
		return *iv.Index, true, nil
	case iv.Key.IsValid():
		return iv.Key.Interface(), true, nil
	default:
		return nil, false, nil
	}
}

// loopPath is the Path of the loop itself, which the iteration is nested in.
func (iv iterationVars) loopPath() Path {
	if len(iv.Scope) == 0 {
		return nil
	}
	return slices.Clip(iv.Scope[:len(iv.Scope)-1])
}

func indexCollection(coll any, iv iterationVars) (any, bool, error) {
	cv := reflectkit.BaseValue(reflect.ValueOf(coll))
	switch {
	case iv.Index != nil:
		if cv.Kind() != reflect.Slice {
			return nil, false, fmt.Errorf("workflow.ForEach: cannot index %s by index", cv.Kind())
		}
		i := *iv.Index
		if i < 0 || i >= cv.Len() {
			return nil, false, fmt.Errorf("workflow.ForEach: slice index %d out of range", i)
		}
		return cv.Index(i).Interface(), true, nil
	case iv.Key.IsValid():
		if cv.Kind() != reflect.Map {
			return nil, false, ErrFatal.F("workflow.ForEach: cannot index %s by key", cv.Kind())
		}
		if !iv.Key.Type().AssignableTo(cv.Type().Key()) {
			return nil, false, ErrFatal.F("workflow.ForEach: cannot index map by %s", iv.Key.Type())
		}
		rv := cv.MapIndex(iv.Key)
		if !rv.IsValid() {
			return nil, false, nil
		}
		return rv.Interface(), true, nil
	default:
		return nil, false, ErrFatal.F("workflow.ForEach: iteration has no index or key")
	}
}

func (foreach ForEach) Error() string { return "workflow::foreach" }
