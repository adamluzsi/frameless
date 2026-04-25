package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"go.llib.dev/frameless/pkg/jsonkit"
)

type ExecuteCondition struct {
	ID    ConditionID   `json:"id"`
	Input []VariableKey `json:"input,omitempty"`
}

var _ Condition = (*ExecuteCondition)(nil)
var _ Definition = (*ExecuteCondition)(nil)

const executeConditionJSONType jsonkit.TypeID = "workflow::execute-condition"

var _ = jsonkit.RegisterTypeID[ExecuteCondition](executeConditionJSONType)

type dtoJSONExecuteCondition struct {
	Type  string        `json:"@type"`
	ID    ConditionID   `json:"id"`
	Input []VariableKey `json:"input,omitempty"`
}

func (d ExecuteCondition) MarshalJSON() ([]byte, error) {
	return json.Marshal(dtoJSONExecuteCondition{
		Type:  executeConditionJSONType.String(),
		ID:    d.ID,
		Input: d.Input,
	})
}

func (d *ExecuteCondition) UnmarshalJSON(data []byte) error {
	var dto dtoJSONExecuteCondition
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}
	*d = ExecuteCondition{
		ID:    dto.ID,
		Input: dto.Input,
	}
	return nil
}

func (d ExecuteCondition) Evaluate(ctx context.Context, p *Process) (bool, error) {
	return d.cachedExecute(ctx, p)
}

func (d ExecuteCondition) Execute(ctx context.Context, p *Process) error {
	result, err := d.cachedExecute(ctx, p)
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

func (d ExecuteCondition) cachedExecute(ctx context.Context, p *Process) (result bool, rerr error) {
	exec := idempotentExecutor[ExecuteConditionEvent, ConditionID]{
		ID: d.ID,
		Func: func(ctx context.Context, input []any) (output []any, _ error) {
			ok, err := d.evaluate(ctx, input)
			return []any{ok}, err
		},
		Input: d.Input,
		CastEvent: func(e ExecuteConditionEvent) (executionEvent[ConditionID], bool) {
			return executionEvent[ConditionID]{
				ID:     e.ConditionID,
				Input:  e.Input,
				Result: []any{e.Answer},
			}, true
		},
		NewEvent: func(id ConditionID, input, output []any) ExecuteConditionEvent {
			return ExecuteConditionEvent{
				ConditionID: id,
				Input:       input,
				Answer:      output[0].(bool),
			}
		},
	}
	_ = exec

	outs, err := exec.executeWR(ctx, p)
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

func (c *conditionWrapper) Evaluate(ctx context.Context, p *Process) (bool, error) {
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

type ExecuteConditionEvent struct {
	ConditionID ConditionID `json:"cid,omitempty"`
	Input       []any       `json:"input"`
	Answer      bool        `json:"answer"`
}

var _ = jsonkit.RegisterTypeID[ExecuteConditionEvent]("workflow::execute-condition-event")

const eidExecuteConditionEvent = "execute-condition"

func (ExecuteConditionEvent) Type() EventType {
	return eidExecuteConditionEvent
}
