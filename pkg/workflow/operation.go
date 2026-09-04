package workflow

import (
	"context"
	"reflect"
)

// Increment is the "++" increment operator as definition.
// Increment will increment a numeric variable value by one.
//
// It accepts the same value types as Go's ++ operator: signed integers,
// unsigned integers and floats. The incremented value keeps the variable's
// original type, and overflow wraps the same way ++ does.
//
// A variable which is declared but never assigned counts from a zero int, so
// that a DeclareVar is all it takes to start using a variable as a counter.
type Increment struct{ Name VarName }

var _ Definition = Increment{}

func (Increment) Error() string { return "workflow::op::increment" }

func (d Increment) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "inc-op")
	repo, err := LookupEventsRepository(ctx)
	if err != nil {
		return err
	}
	var vars = Vars{ProcessID: pid, EventsRepository: repo}
	v, ok, err := vars.Lookup(ctx, d.Name)
	if err != nil {
		return err
	}
	if !ok {
		return ErrFatal.F("%s var missing", d.Name)
	}
	v, err = d.increment(v)
	if err != nil {
		return err
	}
	return vars.setOnce(ctx, d.Name, v)
}

func (d Increment) increment(v any) (any, error) {
	// A declaration brings a binding into existence without giving it a value,
	// so an unassigned variable arrives here as nil. Counting starts at the
	// zero value of an int, which makes the first increment yield 1.
	if v == nil {
		return 1, nil
	}
	var rv = reflect.ValueOf(v)
	// Convert back to the variable's own type so a named type such as
	// `type Attempt int` survives the increment, then Interface() so what gets
	// recorded is the value itself rather than a reflection handle to it.
	switch {
	case rv.CanInt():
		return reflect.ValueOf(rv.Int() + 1).Convert(rv.Type()).Interface(), nil
	case rv.CanUint():
		return reflect.ValueOf(rv.Uint() + 1).Convert(rv.Type()).Interface(), nil
	case rv.CanFloat():
		return reflect.ValueOf(rv.Float() + 1).Convert(rv.Type()).Interface(), nil
	default:
		return nil, ErrFatal.F("unknown type to increment: %T", v)
	}
}
