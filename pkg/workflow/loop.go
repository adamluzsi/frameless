package workflow

import (
	"context"
	"fmt"
	"reflect"

	"go.llib.dev/frameless/pkg/contextkit"
	"go.llib.dev/frameless/pkg/reflectkit"
)

// For is the workflow equivalent of Go's three clause for loop:
//
//	for Init; Cond; Post { Do }
//
// Init, Cond, Post and Do share a single variable scope which belongs to the
// loop, so the loop variable is visible to every part of the loop without
// leaking out of it. A variable declared outside of the loop is still visible
// inside, and an assignment to it writes through, so it keeps whatever the loop
// counted it to.
//
// Every round is a step of its own on the definition path, so the idempotent
// replay of a Process can tell the rounds apart: a participant in the body runs
// again on the next round rather than being skipped as already done.
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
	ctx = WithName(ctx, "for")
	ctx = WithVarScope(ctx, "for")

	if loop.Init != nil {
		if err := loop.Init.Execute(WithName(ctx, "init"), processID); err != nil {
			return loopDone(err)
		}
	}

	for round := 0; ; round++ {
		// Each round derives its own context from the loop scoped ctx, so the
		// rounds don't leak path segments into one another.
		roundCtx := WithName(ctx, fmt.Sprintf("[%d]", round))

		if loop.Cond != nil {
			ok, err := loop.Cond.Evaluate(WithName(roundCtx, "cond"), processID)
			if err != nil {
				return loopDone(err)
			}
			if !ok {
				return nil
			}
		}

		if loop.Do != nil {
			if err := loop.Do.Execute(WithName(roundCtx, "do"), processID); err != nil {
				return loopDone(err)
			}
		}

		if loop.Post != nil {
			if err := loop.Post.Execute(WithName(roundCtx, "post"), processID); err != nil {
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
//			workflow.ExecuteParticipant{ID: "poll-job", Output: []workflow.VarName{"done"}},
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

type ForEach struct {
	// Over is the VarName that ForEach uses to find the iterable collection
	Over VarName
	// Do is the ForEach loop body, which is executed on each iteration, but with the currently relevant iteration scoped variable.
	// The body ends the loop before the collection runs out by raising a Break.
	Do Definition
	// K [optional] will be used to store either the index value of a slice, or the key of a map of the iterated collection variable.
	K VarName
	// V [optional] is where the value of the currently iterated element will be placed.
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
// The iteration gets a scope of its own on the definition path so the
// idempotent replay of a Process can tell the iterations apart and skip the
// ones it already ran.
//
// The iteration's Key and Value are not stored as state on the iteration: the
// context only carries a reference — the name of the collection variable and
// the position within it — and every read of the iteration variable re-evaluates
// the position against the current value of the collection in the event log.
// A Process is a record of state changes, and a ForEach iteration is a
// reference on what to look up from that record, not a change of its own.
// Caching the value at iteration-start would silently disagree with the
// collection once the body reassigns it.
func (foreach ForEach) iterate(ctx context.Context, processID ProcessID, bodyScope string, index *int, key reflect.Value) error {
	ctx = WithName(ctx, bodyScope)
	ctx = WithVarScope(ctx, bodyScope)
	ctx = withIterationVars(ctx, iterationVars{
		KeyName:   foreach.K,
		ValueName: foreach.V,

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
// (exactly one of them), and lookupIterationVar uses this instruction to
// re-resolve the value against the current state of the collection on every
// read, so a body that reassigns the collection variable sees the new value
// without the proxy holding a stale snapshot.
type iterationVars struct {
	KeyName   VarName
	ValueName VarName

	Source VarName
	Index  *int
	Key    reflect.Value
}

var ctxIterationVars = contextkit.ValueHandler[ctxKeyIterationVars, iterationVars]{}

type ctxKeyIterationVars struct{}

func withIterationVars(ctx context.Context, v iterationVars) context.Context {
	return ctxIterationVars.ContextWith(ctx, v)
}

func lookupIterationVar(ctx context.Context, name VarName, events []Event) (any, bool, error) {
	var iv, ok = ctxIterationVars.Lookup(ctx)
	if !ok {
		return nil, false, nil
	}
	switch name {
	case iv.KeyName:
		switch {
		case iv.Index != nil:
			return *iv.Index, true, nil
		case iv.Key.IsValid():
			return iv.Key.Interface(), true, nil
		}
		return nil, false, nil
	case iv.ValueName:
		_, collVal, ok := lookupVariable(ctx, events, iv.Source)
		if !ok {
			return nil, false, nil
		}
		return indexCollection(collVal, iv)
	}
	return nil, false, nil
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
