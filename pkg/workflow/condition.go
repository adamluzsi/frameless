package workflow

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"go.llib.dev/frameless/pkg/jsonkit"
)

type ExecuteCondition struct {
	ID    ConditionID
	Input []VarName
}

var _ Condition = ExecuteCondition{}
var _ Definition = ExecuteCondition{}

const executeConditionJSONType jsonkit.TypeID = "workflow::condition"

func (d ExecuteCondition) Error() string { return executeConditionJSONType.String() }

func (d ExecuteCondition) Evaluate(ctx context.Context, pid ProcessID) (bool, error) {
	return d.cachedExecute(ctx, pid)
}

func (d ExecuteCondition) Execute(ctx context.Context, pid ProcessID) error {
	ctx = WithName(ctx, "condition")
	result, err := d.cachedExecute(ctx, pid)
	if err != nil {
		return err
	}
	// Store the result in a temporary variable for potential use by control flow definitions
	_ = result
	return nil
}

func (d ExecuteCondition) evaluate(ctx context.Context, input []any) (_result bool, _ error) {
	pr, ok := ctxConditionsH.Lookup(ctx)
	if !ok {
		return false, ErrFatal.F("missing condition mapping from workflow runtime")
	}
	condition, found, err := pr.FindByID(ctx, d.ID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, ErrConditionNotFound{ID: d.ID}
	}

	fn, err := condition.(*conditionWrapper).rfn(ctx)
	if err != nil {
		return false, err
	}

	var args []reflect.Value
	args = append(args, reflect.ValueOf(ctx))
	for _, value := range input {
		rval := reflect.ValueOf(value)
		args = append(args, rval)
	}

	out := fn.Call(args)

	var result bool
	if len(out) >= 1 {
		result = out[0].Bool()
	}
	if len(out) >= 2 {
		if errVal, ok := out[len(out)-1].Interface().(error); ok && errVal != nil {
			return false, errVal
		}
	}

	return result, nil
}

func (d ExecuteCondition) cachedExecute(ctx context.Context, pid ProcessID) (result bool, rerr error) {
	exec := idempotentExecutor[EventCondition, ConditionID]{
		ID: d.ID,
		Do: func(ctx context.Context, input []any) (output []any, _ error) {
			ok, err := d.evaluate(ctx, input)
			return []any{ok}, err
		},
		Input: d.Input,
		CastEvent: func(e EventCondition) (executionEvent[ConditionID], bool) {
			return executionEvent[ConditionID]{
				ID:     e.ConditionID,
				Path:   e.Path,
				Input:  e.Input,
				Result: []any{e.Answer},
			}, true
		},
		MakeEvent: func(id ConditionID, path Path, input, output []any) (EventCondition, error) {
			eventID, err := MakeEventID()
			if err != nil {
				return EventCondition{}, err
			}
			return EventCondition{
				EventID:     eventID,
				ProcessID:   pid,
				ConditionID: id,
				Path:        path,
				Input:       input,
				Answer:      output[0].(bool),
				Timestamp:   timeNow(),
			}, nil
		},
	}

	outs, err := exec.executeWR(ctx, pid)
	if err != nil {
		return false, err
	}
	if len(outs) != 1 {
		return false, fmt.Errorf("incorrect condition caching implementation, expected 1 boolean result, but got %d", len(outs))
	}

	if evaluation, ok := outs[0].(bool); ok {
		return evaluation, nil
	}

	return false, fmt.Errorf("incorrect condition caching implementation, expected 1 boolean result, but got type %T", outs[0])
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// conditionWrapper is the internal wrapper for condition functions.
// It implements the Condition interface and provides validation capabilities.
type conditionWrapper struct {
	ID   ConditionID
	Func any // func(context.Context, ...) (bool, error)
}

var _ Condition = (*conditionWrapper)(nil)

func (c *conditionWrapper) Evaluate(ctx context.Context, pid ProcessID) (bool, error) {
	rfunc, err := c.rfn(ctx)
	if err != nil {
		return false, err
	}

	var args []reflect.Value
	args = append(args, reflect.ValueOf(ctx))
	// TODO: add input argument handling similar to ExecuteParticipant
	// for _, value := range input {
	// 	rval := reflect.ValueOf(value)
	// 	args = append(args, rval)
	// }

	out := rfunc.Call(args)

	var result bool
	var errResult error

	if len(out) >= 1 {
		result = out[0].Bool()
	}
	if len(out) >= 2 {
		if errVal, ok := out[len(out)-1].Interface().(error); ok {
			errResult = errVal
		}
	}

	return result, errResult
}

func (c *conditionWrapper) funcSignature(ctx context.Context) string {
	rfunc, err := c.rfn(ctx)
	if err != nil {
		return ""
	}
	var (
		fnType = rfunc.Type()
		input  []string
		output []string
	)
	for i := range fnType.NumIn() {
		in := fnType.In(i)
		val := in.String()
		if in.IsVariadic() {
			val = "..." + val
		}
		input = append(input, in.String())
	}
	for i := range fnType.NumOut() {
		output = append(output, fnType.Out(i).String())
	}
	return fmt.Sprintf("func(%s) (%s)", strings.Join(input, ", "), strings.Join(output, ", "))
}

func (c *conditionWrapper) rfn(ctx context.Context) (reflect.Value, error) {
	rfunc := reflect.ValueOf(c.Func)
	if rfunc.Kind() != reflect.Func {
		return rfunc, ErrInvalidConditionFunc.F("invalid value for condition func")
	}
	var (
		funcType   = rfunc.Type()
		funcNumIn  = funcType.NumIn()
		funcNumOut = funcType.NumOut()
	)
	if funcNumIn < 1 {
		return rfunc, ErrInvalidConditionFunc
	}
	if funcType.In(0) != reflectContextType {
		return rfunc, ErrInvalidConditionFunc
	}
	if funcNumOut < 2 {
		return rfunc, ErrInvalidConditionFunc
	}
	if firstOut := funcType.Out(0); firstOut.Kind() != reflect.Bool {
		return rfunc, ErrInvalidConditionFunc.F("first return value must be bool")
	}
	if lastOut := funcType.Out(funcNumOut - 1); lastOut != reflectErrorType || !lastOut.Implements(reflectErrorType) {
		return rfunc, ErrInvalidConditionFunc
	}
	return rfunc, nil
}

func (c *conditionWrapper) Validate(ctx context.Context) error {
	_, err := c.rfn(ctx)
	return err
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type Conditions map[ConditionID]any

var _ ConditionRepository = (Conditions)(nil)

func (cs Conditions) FindByID(ctx context.Context, id ConditionID) (Condition, bool, error) {
	if len(cs) == 0 {
		var zero Condition
		return zero, false, nil
	}
	fn, ok := cs[id]
	if !ok {
		var zero Condition
		return zero, false, nil
	}
	return &conditionWrapper{ID: id, Func: fn}, true, nil
}

func (cs Conditions) Validate(ctx context.Context) error {
	for id, fn := range cs {
		c := &conditionWrapper{
			ID:   id,
			Func: fn,
		}
		if err := c.Validate(ctx); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
	}
	return nil
}

///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

type EventCondition struct {
	EventID     EventID `ext:"id"`
	ProcessID   ProcessID
	ConditionID ConditionID
	Path        Path
	Input       []any
	Answer      bool
	Timestamp   time.Time
}

var _ Event = EventCondition{}

const eIDExecuteConditionEvent = "execute-condition"

func (EventCondition) EventType() EventType      { return eIDExecuteConditionEvent }
func (e EventCondition) GetEventID() EventID     { return e.EventID }
func (e EventCondition) GetProcessID() ProcessID { return e.ProcessID }
func (e EventCondition) GetTimestamp() time.Time { return e.Timestamp }
