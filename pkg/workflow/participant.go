package workflow

import (
	"context"
	"fmt"
	"reflect"
)

// Participant is the entity that perform tasks in the workflow.
// Participants can be human users, groups of users, or even automated systems.
// The workflow engine needs to manage the assignment of tasks to participants and track their progress.
//
// A Participant must be a function type, with arguments and must return an error as result.
type Participant any
type ParticipantID string

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var ctxType = reflect.TypeOf((*context.Context)(nil)).Elem()

func makeRegParticipant(fn Participant) (regParticipant, error) {
	var p regParticipant
	p.Func = reflect.ValueOf(fn)
	if p.Func.Kind() != reflect.Func {
		return regParticipant{}, fmt.Errorf("workflow.Participant must be a function")
	}
	return p, nil
}

type regParticipant struct{ Func reflect.Value }

func (rp regParticipant) Exec(ctx context.Context, inargs ...any) ([]any, error) {
	args, err := rp.toArgs(ctx, inargs)
	if err != nil {
		return nil, err
	}
	var (
		out []any
		res = rp.Func.Call(args)
	)
	if len(res) == 0 {
		return []any{}, err
	}
	if NumOut := rp.Func.Type().NumOut(); 0 < NumOut &&
		rp.Func.Type().Out(NumOut-1).Implements(errorType) {
		errVal := res[len(res)-1]
		res = res[0 : NumOut-1]
		err = errVal.Interface().(error)
	}
	for _, v := range res {
		out = append(out, v.Interface())
	}
	return out, err
}

func (rp regParticipant) toArgs(ctx context.Context, inargs []any) ([]reflect.Value, error) {
	var (
		args      []reflect.Value
		expInArgs = rp.getArgTypes()
	)
	if 0 < len(expInArgs) && expInArgs[0].Implements(ctxType) {
		expInArgs = expInArgs[1:]
		args = append(args, reflect.ValueOf(ctx))
	}
	if len(expInArgs) != len(inargs) {
		return nil, fmt.Errorf("input argument count mismatch for participant")
	}
	for i, arg := range inargs {
		expType := expInArgs[i]
		v := reflect.ValueOf(arg)
		switch {
		case v.Type() == expType:
			args = append(args, v)
		case v.CanConvert(expType):
			args = append(args, v.Convert(expType))
		default:
			return nil, fmt.Errorf("invalid type, expected %s, but got %s",
				expType.String(), v.Type().String())
		}
	}
	return args, nil
}

func (rp regParticipant) getArgTypes() []reflect.Type {
	if rp.Func.Kind() != reflect.Func {
		return nil
	}
	var (
		args  []reflect.Type
		pType = rp.Func.Type()
	)
	for i, n := 0, pType.NumIn(); i < n; i++ {
		args = append(args, pType.In(i))
	}
	return args
}

