package workflow

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
)

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

func (EventCondition) EventType() EventType      { return "workflow::condition" }
func (e EventCondition) GetEventID() EventID     { return e.EventID }
func (e EventCondition) GetProcessID() ProcessID { return e.ProcessID }
func (e EventCondition) GetTimestamp() time.Time { return e.Timestamp }

// /////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
//
// conditionWrapper is the internal wrapper for condition functions.
// It implements the Condition interface and provides validation capabilities.
type conditionWrapper struct {
	ID   ConditionID
	Func any // func(context.Context, ...) (bool, error)
}

var _ Condition = (*conditionWrapper)(nil)

func (c *conditionWrapper) Evaluate(ctx context.Context, pid ProcessID) (bool, error) {
	return c.evaluate(ctx, nil)
}

func (cw *conditionWrapper) evaluate(ctx context.Context, input []any) (bool, error) {
	fn, err := cw.rfn(ctx)
	if err != nil {
		return false, err
	}

	args, err := invocationArgs(fn.Type(), ctx, input)
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrFatal, ErrConditionFuncMappingMismatch.F(
			"%v\nsignature: %s", err, cw.funcSignature(ctx)))
	}

	out := invoke(fn, args)
	err, _ = out[1].Interface().(error)
	return out[0].Bool(), err
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
		if fnType.IsVariadic() && i == fnType.NumIn()-1 {
			val = "..." + in.Elem().String()
		}
		input = append(input, val)
	}
	for i := range fnType.NumOut() {
		output = append(output, fnType.Out(i).String())
	}
	return fmt.Sprintf("func(%s) (%s)", strings.Join(input, ", "), strings.Join(output, ", "))
}

func (c *conditionWrapper) rfn(ctx context.Context) (reflect.Value, error) {
	rfunc := reflect.ValueOf(c.Func)
	if rfunc.Kind() != reflect.Func || rfunc.IsNil() {
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
	if funcNumOut != 2 {
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
